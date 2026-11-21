<template>
    <div>
        <RouterMenu />
        <LayoutContent :title="$t('vllm.deployment')" v-loading="loading">
            <template #leftToolBar>
                <div class="flex flex-wrap gap-3">
                    <el-button type="primary" @click="operateRef.acceptParams({ external: false })">
                        {{ $t('vllm.createInstance') }}
                    </el-button>
                    <el-button type="primary" plain @click="operateRef.acceptParams({ external: true })">
                        {{ $t('vllm.createExternal') }}
                    </el-button>
                </div>
            </template>
            <template #rightToolBar>
                <div class="flex gap-2">
                    <TableSearch @search="search()" v-model:searchName="info" />
                    <TableRefresh @search="search()" />
                </div>
            </template>
            <template #main>
                <ComplexTable :pagination-config="paginationConfig" :data="items" @search="search()">
                    <el-table-column :label="$t('commons.table.name')" prop="name" min-width="150" fix>
                        <template #default="{ row }">
                            <el-text type="primary" class="cursor-pointer" @click="operateRef.acceptParams(row)">
                                {{ row.name }}
                            </el-text>
                            <el-tag v-if="row.external" size="small" class="ml-1">{{ $t('vllm.external') }}</el-tag>
                        </template>
                    </el-table-column>
                    <el-table-column :label="$t('app.version')" prop="image" min-width="160" show-overflow-tooltip>
                        <template #default="{ row }">{{ row.external ? '-' : imageTag(row.image) }}</template>
                    </el-table-column>
                    <el-table-column :label="$t('commons.table.port')" prop="port" width="90">
                        <template #default="{ row }">{{ row.external ? '-' : row.port }}</template>
                    </el-table-column>
                    <el-table-column :label="$t('vllm.kindModel')" min-width="180" show-overflow-tooltip>
                        <template #default="{ row }">
                            {{ row.external ? row.servedName || '-' : row.modelName || row.servedName }}
                        </template>
                    </el-table-column>
                    <el-table-column :label="$t('vllm.endpoint')" min-width="220" show-overflow-tooltip>
                        <template #default="{ row }">
                            {{ row.endpoint }}
                            <CopyButton :content="row.endpoint" />
                        </template>
                    </el-table-column>
                    <el-table-column :label="$t('commons.table.status')" width="110">
                        <template #default="{ row }">
                            <el-popover
                                v-if="row.message"
                                placement="bottom"
                                :width="400"
                                trigger="hover"
                                :content="row.message"
                                popper-class="max-h-[300px] overflow-auto"
                            >
                                <template #reference>
                                    <Status :key="row.status" :status="row.status" />
                                </template>
                            </el-popover>
                            <Status v-else :key="row.status" :status="row.status" />
                        </template>
                    </el-table-column>
                    <el-table-column :label="$t('vllm.serviceStatus')" width="110">
                        <template #default="{ row }">
                            <el-tag v-if="stats[row.id]?.healthy" type="success" size="small">
                                {{ $t('vllm.ready') }}
                            </el-tag>
                            <el-tag v-else-if="row.status === 'Running'" type="warning" size="small">
                                {{ $t('vllm.loading') }}
                            </el-tag>
                            <span v-else>-</span>
                        </template>
                    </el-table-column>
                    <el-table-column :label="$t('commons.button.log')" width="80">
                        <template #default="{ row }">
                            <el-button link type="primary" :disabled="row.external" @click="openLog(row)">
                                {{ $t('website.check') }}
                            </el-button>
                        </template>
                    </el-table-column>
                    <el-table-column :label="$t('vllm.resource')" min-width="150">
                        <template #default="{ row }">
                            <span v-if="!row.external && stats[row.id] && row.status === 'Running'">
                                CPU {{ (stats[row.id].cpuPercent || 0).toFixed(1) }}% /
                                {{ computeSizeFromMB(stats[row.id].memoryMB || 0) }}
                            </span>
                            <span v-else>-</span>
                        </template>
                    </el-table-column>
                    <fu-table-operations
                        :ellipsis="isMobile ? 0 : 2"
                        :min-width="isMobile ? 'auto' : 200"
                        :buttons="buttons"
                        :label="$t('commons.table.operate')"
                        fixed="right"
                        fix
                    />
                </ComplexTable>
            </template>
        </LayoutContent>
        <OperateDrawer ref="operateRef" @search="searchLater" @task="openTaskLog" />
        <ComposeLogs ref="composeLogRef" />
        <TaskLog ref="taskLogRef" width="70%" @close="search" />
        <OpDialog ref="opRef" @search="search" />
    </div>
</template>

<script lang="ts" setup>
import { ElMessageBox } from 'element-plus';
import { onMounted, onUnmounted, reactive, ref } from 'vue';
import RouterMenu from '@/views/ai/vllm/index.vue';
import OperateDrawer from './operate/index.vue';
import ComposeLogs from '@/components/log/compose/index.vue';
import TaskLog from '@/components/log/task/index.vue';
import { Vllm } from '@/api/interface/vllm';
import {
    checkVllmDeployment,
    deleteVllmDeployment,
    loadVllmDeploymentStats,
    operateVllmDeployment,
    pageVllmDeployments,
} from '@/api/modules/vllm';
import { computeSizeFromMB } from '@/utils/size';
import { MsgError, MsgSuccess } from '@/utils/message';
import i18n from '@/lang';
import { useGlobalStore } from '@/composables/useGlobalStore';

const { isMobile } = useGlobalStore();
const loading = ref(false);
const items = ref<Vllm.Deployment[]>([]);
const stats = ref<Record<number, Vllm.DeploymentStats>>({});
const info = ref('');
const operateRef = ref();
const composeLogRef = ref();
const taskLogRef = ref();
const opRef = ref();
let timer: ReturnType<typeof setInterval> | undefined;
const paginationConfig = reactive({ currentPage: 1, pageSize: 20, total: 0 });

const imageTag = (image: string) => {
    const i = image?.lastIndexOf(':') ?? -1;
    return i > 0 ? image.substring(i + 1) : image || '-';
};

const op = (row: Vllm.Deployment, operate: string) => {
    ElMessageBox.confirm(
        i18n.global.t('vllm.operateHelper', [row.name, i18n.global.t('commons.button.' + operate)]),
        i18n.global.t('commons.button.' + operate),
        { type: 'info' },
    ).then(async () => {
        loading.value = true;
        try {
            await operateVllmDeployment(row.id, operate);
            MsgSuccess(i18n.global.t('commons.msg.operationSuccess'));
        } finally {
            loading.value = false;
            search();
        }
    });
};

const buttons = [
    {
        label: i18n.global.t('commons.button.edit'),
        click: (row: Vllm.Deployment) => operateRef.value.acceptParams(row),
    },
    {
        label: i18n.global.t('commons.button.start'),
        click: (row: Vllm.Deployment) => op(row, 'start'),
        disabled: (row: Vllm.Deployment) => row.external || row.status === 'Running',
    },
    {
        label: i18n.global.t('commons.button.stop'),
        click: (row: Vllm.Deployment) => op(row, 'stop'),
        disabled: (row: Vllm.Deployment) => row.external || row.status === 'Stopped',
    },
    {
        label: i18n.global.t('commons.button.restart'),
        click: (row: Vllm.Deployment) => op(row, 'restart'),
        disabled: (row: Vllm.Deployment) => row.external,
    },
    {
        label: i18n.global.t('vllm.testConnection'),
        click: async (row: Vllm.Deployment) => {
            loading.value = true;
            try {
                const res = await checkVllmDeployment(row.id);
                if (res.data.healthy) {
                    MsgSuccess(i18n.global.t('vllm.connectOk', [(res.data.models || []).join(', ') || '-']));
                } else {
                    MsgError(res.data.message || 'unhealthy');
                }
            } finally {
                loading.value = false;
            }
        },
    },
    {
        label: i18n.global.t('commons.button.delete'),
        click: (row: Vllm.Deployment) => {
            opRef.value.acceptParams({
                title: i18n.global.t('commons.button.delete'),
                names: [row.name],
                msg: i18n.global.t('vllm.deleteDeploymentHelper'),
                api: deleteVllmDeployment,
                params: { id: row.id },
            });
        },
    },
];

const search = async () => {
    loading.value = true;
    try {
        const res = await pageVllmDeployments({
            page: paginationConfig.currentPage,
            pageSize: paginationConfig.pageSize,
            info: info.value,
        });
        items.value = res.data.items || [];
        paginationConfig.total = res.data.total;
    } finally {
        loading.value = false;
    }
    loadStats();
};

const loadStats = async () => {
    const ids = items.value.map((i) => i.id);
    if (ids.length === 0) return;
    try {
        const res = await loadVllmDeploymentStats(ids);
        const map: Record<number, Vllm.DeploymentStats> = {};
        for (const s of res.data || []) {
            map[s.id] = s;
            const row = items.value.find((i) => i.id === s.id);
            if (row) {
                row.status = s.status;
                row.message = s.message;
            }
        }
        stats.value = map;
    } catch (e) {}
};

const searchLater = () => {
    search();
    setTimeout(search, 1500);
};

const openLog = (row: Vllm.Deployment) => {
    composeLogRef.value.acceptParams({
        compose: row.dir + '/docker-compose.yml',
        resource: row.name,
        container: row.containerName,
    });
};

const openTaskLog = (taskID: string) => {
    taskLogRef.value.openWithTaskID(taskID);
};

onMounted(() => {
    search();
    timer = setInterval(loadStats, 10000);
});
onUnmounted(() => {
    if (timer) clearInterval(timer);
});
</script>

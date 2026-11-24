<template>
    <div>
        <RouterMenu />
        <LayoutContent :title="$t('vllm.modelPrepare')" v-loading="loading">
            <template #leftToolBar>
                <div class="flex flex-wrap gap-3">
                    <el-button type="primary" @click="downloadRef.acceptParams()">
                        {{ $t('vllm.downloadModel') }}
                    </el-button>
                    <el-button type="primary" plain @click="importRef.acceptParams()">
                        {{ $t('vllm.importLocal') }}
                    </el-button>
                    <el-button type="primary" plain @click="recipeRef.acceptParams()">vLLM Recipes</el-button>
                    <el-button plain @click="settingRef.acceptParams()">
                        {{ $t('commons.button.set') }}
                    </el-button>
                </div>
            </template>
            <template #rightToolBar>
                <div class="flex gap-2">
                    <el-select v-model="kind" class="!w-32" @change="search()">
                        <el-option :label="$t('commons.table.all')" value="" />
                        <el-option :label="$t('vllm.kindModel')" value="model" />
                        <el-option :label="$t('vllm.kindTokenizer')" value="tokenizer" />
                    </el-select>
                    <TableSearch @search="search()" v-model:searchName="info" />
                    <TableRefresh @search="search()" />
                </div>
            </template>
            <template #main>
                <ComplexTable :pagination-config="paginationConfig" :data="items" @search="search()">
                    <el-table-column :label="$t('vllm.repoId')" prop="repoId" min-width="220" show-overflow-tooltip />
                    <el-table-column :label="$t('commons.table.type')" prop="kind" width="100">
                        <template #default="{ row }">
                            {{ row.kind === 'tokenizer' ? $t('vllm.kindTokenizer') : $t('vllm.kindModel') }}
                        </template>
                    </el-table-column>
                    <el-table-column :label="$t('vllm.source')" prop="source" width="120">
                        <template #default="{ row }">
                            {{ sourceLabel(row.source) }}
                        </template>
                    </el-table-column>
                    <el-table-column :label="$t('commons.table.status')" prop="status" width="120">
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
                    <el-table-column :label="$t('file.size')" prop="size" width="110">
                        <template #default="{ row }">
                            {{ row.size ? computeSize(row.size) : '-' }}
                        </template>
                    </el-table-column>
                    <el-table-column :label="$t('vllm.dir')" prop="dir" min-width="240" show-overflow-tooltip>
                        <template #default="{ row }">
                            {{ row.dir }}
                            <CopyButton :content="row.dir" />
                        </template>
                    </el-table-column>
                    <el-table-column
                        prop="createdAt"
                        :label="$t('commons.table.date')"
                        :formatter="dateFormat"
                        show-overflow-tooltip
                        width="170"
                    />
                    <fu-table-operations
                        :ellipsis="isMobile ? 0 : 3"
                        :min-width="isMobile ? 'auto' : 220"
                        :buttons="buttons"
                        :label="$t('commons.table.operate')"
                        fixed="right"
                        fix
                    />
                </ComplexTable>
            </template>
        </LayoutContent>

        <DownloadDialog ref="downloadRef" @search="searchLater" @task="openTaskLog" />
        <ImportDialog ref="importRef" @search="search" />
        <SettingDrawer ref="settingRef" />
        <RecipeDrawer ref="recipeRef" />
        <TaskLog ref="taskLogRef" width="70%" @close="search" />
        <SendDialog ref="sendRef" />

        <DialogPro v-model="delOpen" :title="$t('commons.button.delete')" size="small">
            <div>{{ $t('vllm.deleteModelHelper', [delRow?.repoId]) }}</div>
            <el-checkbox v-if="delRow?.source !== 'local'" class="mt-3" v-model="delFile">
                {{ $t('vllm.deleteFiles') }}
            </el-checkbox>
            <template #footer>
                <el-button @click="delOpen = false">{{ $t('commons.button.cancel') }}</el-button>
                <el-button type="danger" @click="onDelete">{{ $t('commons.button.confirm') }}</el-button>
            </template>
        </DialogPro>
    </div>
</template>

<script lang="ts" setup>
import { onMounted, onUnmounted, reactive, ref } from 'vue';
import RouterMenu from '@/views/ai/vllm/index.vue';
import DownloadDialog from './download/index.vue';
import ImportDialog from './import/index.vue';
import SettingDrawer from './setting/index.vue';
import RecipeDrawer from './recipe/index.vue';
import SendDialog from './send/index.vue';
import TaskLog from '@/components/log/task/index.vue';
import { Vllm } from '@/api/interface/vllm';
import { cancelVllmModel, deleteVllmModel, pageVllmModels, retryVllmModel } from '@/api/modules/vllm';
import { dateFormat } from '@/utils/date';
import { computeSize } from '@/utils/size';
import { MsgSuccess } from '@/utils/message';
import i18n from '@/lang';
import { useGlobalStore } from '@/composables/useGlobalStore';

const { isMobile } = useGlobalStore();
const loading = ref(false);
const items = ref<Vllm.Model[]>([]);
const info = ref('');
const kind = ref('');
const downloadRef = ref();
const importRef = ref();
const settingRef = ref();
const recipeRef = ref();
const taskLogRef = ref();
const sendRef = ref();
const delOpen = ref(false);
const delFile = ref(true);
const delRow = ref<Vllm.Model>();
let timer: ReturnType<typeof setInterval> | undefined;

const paginationConfig = reactive({ currentPage: 1, pageSize: 20, total: 0 });

const sourceLabel = (s: string) => {
    switch (s) {
        case 'modelscope':
            return 'ModelScope';
        case 'huggingface':
            return 'Hugging Face';
        default:
            return i18n.global.t('vllm.sourceLocal');
    }
};

const buttons = [
    {
        label: i18n.global.t('commons.button.log'),
        click: (row: Vllm.Model) => openTaskLog(row.taskID),
        disabled: (row: Vllm.Model) => !row.taskID || row.source === 'local',
    },
    {
        label: i18n.global.t('commons.button.cancel'),
        click: async (row: Vllm.Model) => {
            await cancelVllmModel(row.id);
            MsgSuccess(i18n.global.t('commons.msg.operationSuccess'));
            searchLater();
        },
        disabled: (row: Vllm.Model) => row.status !== 'Downloading',
    },
    {
        label: i18n.global.t('vllm.resume'),
        click: async (row: Vllm.Model) => {
            await retryVllmModel(row.id);
            MsgSuccess(i18n.global.t('commons.msg.operationSuccess'));
            searchLater();
        },
        disabled: (row: Vllm.Model) => row.source === 'local' || row.status === 'Downloading',
    },
    {
        label: i18n.global.t('vllm.sendToNode'),
        click: (row: Vllm.Model) => sendRef.value.acceptParams(row),
        // only complete models are a consistent set of files
        disabled: (row: Vllm.Model) => row.status !== 'Success' || !row.dir,
    },
    {
        label: i18n.global.t('commons.button.delete'),
        click: (row: Vllm.Model) => {
            delRow.value = row;
            delFile.value = true;
            delOpen.value = true;
        },
        disabled: (row: Vllm.Model) => row.status === 'Downloading',
    },
];

const onDelete = async () => {
    if (!delRow.value) return;
    await deleteVllmModel({ id: delRow.value.id, deleteFile: delFile.value });
    delOpen.value = false;
    MsgSuccess(i18n.global.t('commons.msg.deleteSuccess'));
    search();
};

const search = async () => {
    loading.value = true;
    try {
        const res = await pageVllmModels({
            page: paginationConfig.currentPage,
            pageSize: paginationConfig.pageSize,
            info: info.value,
            kind: kind.value,
        });
        items.value = res.data.items || [];
        paginationConfig.total = res.data.total;
    } finally {
        loading.value = false;
    }
};

const searchLater = () => {
    search();
    setTimeout(search, 1500);
};

const openTaskLog = (taskID: string) => {
    if (taskID) taskLogRef.value.openWithTaskID(taskID);
};

onMounted(() => {
    search();
    // refresh while downloads are running
    timer = setInterval(() => {
        if (items.value.some((i) => i.status === 'Downloading')) search();
    }, 5000);
});
onUnmounted(() => {
    if (timer) clearInterval(timer);
});
</script>

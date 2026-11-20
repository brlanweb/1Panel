<template>
    <div>
        <LayoutContent :title="$t('menu.clusterPanels')" v-loading="loading">
            <template #leftToolBar>
                <el-button type="primary" @click="openCreate">
                    {{ $t('commons.button.create') }}
                </el-button>
                <el-button type="primary" plain @click="checkAll" :loading="checking">
                    {{ $t('cluster.checkAll') }}
                </el-button>
            </template>
            <template #rightToolBar>
                <el-select class="p-w-200" v-model="req.status" @change="search()" clearable>
                    <template #prefix>{{ $t('cluster.status') }}</template>
                    <el-option :label="$t('commons.table.all')" :value="''"></el-option>
                    <el-option :label="$t('cluster.online')" value="online"></el-option>
                    <el-option :label="$t('cluster.offline')" value="offline"></el-option>
                    <el-option :label="$t('cluster.unknown')" value="unknown"></el-option>
                </el-select>
                <TableSearch @search="search()" v-model:searchName="req.info" />
                <TableRefresh @search="search()" />
                <fu-table-column-select
                    :columns="columns"
                    trigger="hover"
                    :title="$t('commons.table.selectColumn')"
                    popper-class="popper-class"
                    :only-icon="true"
                />
            </template>
            <template #main>
                <ComplexTable
                    :pagination-config="paginationConfig"
                    v-model:selects="selects"
                    :data="data"
                    @search="search()"
                    :columns="columns"
                    localKey="clusterPanelColumn"
                    row-key="id"
                >
                    <el-table-column
                        :label="$t('commons.table.name')"
                        prop="name"
                        min-width="160px"
                        show-overflow-tooltip
                        fix
                    />
                    <el-table-column :label="$t('cluster.status')" prop="status" width="100px" align="center">
                        <template #default="{ row }">
                            <el-tag :type="statusTagType(row.status)" size="small">
                                {{ statusLabel(row.status) }}
                            </el-tag>
                        </template>
                    </el-table-column>
                    <el-table-column :label="$t('cluster.version')" prop="version" width="110px" align="center">
                        <template #default="{ row }">
                            {{ row.version || '—' }}
                        </template>
                    </el-table-column>
                    <el-table-column :label="$t('cluster.cpu')" prop="cpuUsedPercent" width="100px" align="center">
                        <template #default="{ row }">
                            <span v-if="row.status === 'online'">{{ row.cpuUsedPercent.toFixed(1) }}%</span>
                            <span v-else>—</span>
                        </template>
                    </el-table-column>
                    <el-table-column :label="$t('cluster.memory')" prop="memUsedPercent" width="100px" align="center">
                        <template #default="{ row }">
                            <span v-if="row.status === 'online'">{{ row.memUsedPercent.toFixed(1) }}%</span>
                            <span v-else>—</span>
                        </template>
                    </el-table-column>
                    <el-table-column :label="$t('cluster.appCount')" prop="appCount" width="90px" align="center" />
                    <el-table-column
                        :label="$t('cluster.websiteCount')"
                        prop="websiteCount"
                        width="90px"
                        align="center"
                    />
                    <el-table-column :label="$t('cluster.lastSyncStatus')" prop="lastSyncStatus" width="120px">
                        <template #default="{ row }">
                            {{ row.lastSyncStatus || '—' }}
                        </template>
                    </el-table-column>
                    <el-table-column :label="$t('cluster.lastCheck')" prop="lastCheckAt" width="170px">
                        <template #default="{ row }">
                            {{ row.lastCheckAt ? dateFormatSimple(row.lastCheckAt) : '—' }}
                        </template>
                    </el-table-column>
                    <fu-table-operations
                        :ellipsis="1"
                        width="200px"
                        :buttons="buttons"
                        :label="$t('commons.table.operate')"
                        fix
                    />
                </ComplexTable>
            </template>
        </LayoutContent>
        <Operate ref="operateRef" @search="search" />
        <OpDialog ref="deleteRef" @search="search" />
    </div>
</template>

<script lang="ts" setup>
import { onMounted, ref } from 'vue';
import i18n from '@/lang';
import Operate from '@/views/cluster/nodes/components/operate.vue';
import {
    checkAllClusterNodes,
    deleteClusterNode,
    searchClusterNodes,
    testClusterNode,
} from '@/api/modules/cluster';
import { Cluster } from '@/api/interface/cluster';
import { MsgError, MsgSuccess } from '@/utils/message';
import { dateFormatSimple } from '@/utils/date';
import { usePageState } from '@/composables/usePageState';

const loading = ref(false);
const checking = ref(false);
const operateRef = ref();
const deleteRef = ref();
const data = ref<Cluster.NodeInfo[]>([]);
const selects = ref([]);
const columns = ref([]);

const pageState = usePageState(() => ({
    paginationConfig: {
        currentPage: 1,
        pageSize: 20,
        total: 0,
    },
    req: {
        page: 1,
        pageSize: 20,
        info: '',
        status: '',
        groupId: 0,
    } as Cluster.NodeSearch,
}));
const paginationConfig = pageState.paginationConfig;
const req = pageState.req;

const statusTagType = (status: string) => {
    if (status === 'online') return 'success';
    if (status === 'offline') return 'danger';
    return 'info';
};

const statusLabel = (status: string) => {
    if (status === 'online') return i18n.global.t('cluster.online');
    if (status === 'offline') return i18n.global.t('cluster.offline');
    return i18n.global.t('cluster.unknown');
};

const openPanel = (row: Cluster.NodeInfo) => {
    window.open(row.entryUrl, '_blank');
};

const search = async () => {
    req.page = paginationConfig.currentPage;
    req.pageSize = paginationConfig.pageSize;
    loading.value = true;
    await searchClusterNodes(req)
        .then((res) => {
            data.value = res.data.items;
            paginationConfig.total = res.data.total;
        })
        .finally(() => {
            loading.value = false;
        });
};

const checkAll = async () => {
    checking.value = true;
    await checkAllClusterNodes()
        .then(() => {
            MsgSuccess(i18n.global.t('commons.msg.operationSuccess'));
            search();
        })
        .finally(() => {
            checking.value = false;
        });
};

const testNode = async (row: Cluster.NodeInfo) => {
    await testClusterNode(row.id)
        .then(() => {
            MsgSuccess(i18n.global.t('cluster.testSuccess'));
            search();
        })
        .catch(() => {
            MsgError(i18n.global.t('cluster.testFailed'));
        });
};

const openCreate = () => {
    operateRef.value.acceptParams({});
};

const openEdit = (row: Cluster.NodeInfo) => {
    operateRef.value.acceptParams({ row });
};

const openDelete = (row: Cluster.NodeInfo) => {
    deleteRef.value.acceptParams({
        title: i18n.global.t('cluster.deleteNode'),
        msg: i18n.global.t('cluster.deleteNodeHelper', [row.name]),
        names: [row.name],
        api: deleteClusterNode,
        params: row.id,
    });
};

const buttons = [
    {
        label: i18n.global.t('commons.button.edit'),
        click: (row: Cluster.NodeInfo) => {
            openEdit(row);
        },
    },
    {
        label: i18n.global.t('commons.button.delete'),
        click: (row: Cluster.NodeInfo) => {
            openDelete(row);
        },
    },
    {
        label: i18n.global.t('cluster.testConnection'),
        click: (row: Cluster.NodeInfo) => {
            testNode(row);
        },
    },
    {
        label: i18n.global.t('cluster.openPanel'),
        click: (row: Cluster.NodeInfo) => {
            openPanel(row);
        },
    },
];

onMounted(() => {
    search();
});
</script>

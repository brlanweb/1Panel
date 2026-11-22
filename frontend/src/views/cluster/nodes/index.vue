<template>
    <div>
        <LayoutContent :title="$t('menu.clusterNodes')" v-loading="loading">
            <template #leftToolBar>
                <el-button type="primary" @click="openCreate">
                    {{ $t('commons.button.create') }}
                </el-button>
                <el-button type="primary" plain @click="checkAll" :loading="checking">
                    {{ $t('cluster.checkAll') }}
                </el-button>
                <el-button type="primary" plain @click="openAddSelf">
                    {{ $t('cluster.addThisPanel') }}
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
                <el-select class="p-w-200" v-model="req.groupId" @change="search()" clearable>
                    <template #prefix>{{ $t('cluster.group') }}</template>
                    <el-option :label="$t('commons.table.all')" :value="0"></el-option>
                    <el-option v-for="item in groups" :key="item.id" :label="item.name" :value="item.id" />
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
                <el-row :gutter="12" class="mb-4">
                    <el-col :span="6">
                        <el-card shadow="hover">
                            <el-statistic :title="$t('cluster.totalNodes')" :value="overview.total" />
                        </el-card>
                    </el-col>
                    <el-col :span="6">
                        <el-card shadow="hover">
                            <el-statistic :title="$t('cluster.onlineNodes')" :value="overview.online" />
                        </el-card>
                    </el-col>
                    <el-col :span="6">
                        <el-card shadow="hover">
                            <el-statistic :title="$t('cluster.offlineNodes')" :value="overview.offline" />
                        </el-card>
                    </el-col>
                    <el-col :span="6">
                        <el-card shadow="hover">
                            <el-statistic :title="$t('cluster.unknownNodes')" :value="overview.unknown" />
                        </el-card>
                    </el-col>
                </el-row>
                <ComplexTable
                    :pagination-config="paginationConfig"
                    v-model:selects="selects"
                    :data="data"
                    @search="search()"
                    :columns="columns"
                    localKey="clusterNodeColumn"
                    row-key="id"
                >
                    <el-table-column
                        :label="$t('commons.table.name')"
                        prop="name"
                        min-width="160px"
                        show-overflow-tooltip
                        fix
                    />
                    <el-table-column :label="$t('cluster.entryUrl')" prop="entryUrl" min-width="220px">
                        <template #default="{ row }">
                            <el-link type="primary" :underline="false" @click="openPanel(row)">
                                {{ row.entryUrl }}
                            </el-link>
                        </template>
                    </el-table-column>
                    <el-table-column :label="$t('cluster.status')" prop="status" width="100px" align="center">
                        <template #default="{ row }">
                            <el-tag :type="statusTagType(row.status)" size="small">
                                {{ statusLabel(row.status) }}
                            </el-tag>
                        </template>
                    </el-table-column>
                    <el-table-column :label="$t('cluster.latency')" prop="latencyMs" width="100px" align="center">
                        <template #default="{ row }">
                            <span v-if="row.latencyMs > 0">{{ row.latencyMs }} ms</span>
                            <span v-else>—</span>
                        </template>
                    </el-table-column>
                    <el-table-column :label="$t('cluster.version')" prop="version" width="110px" align="center">
                        <template #default="{ row }">
                            {{ row.version || '—' }}
                        </template>
                    </el-table-column>
                    <el-table-column :label="$t('cluster.loginLock')" prop="loginLocked" width="150px" align="center">
                        <template #default="{ row }">
                            <el-tag :type="loginLockTag(row)" size="small">{{ loginLockLabel(row) }}</el-tag>
                        </template>
                    </el-table-column>
                    <el-table-column :label="$t('cluster.lastCheck')" prop="lastCheckAt" width="170px">
                        <template #default="{ row }">
                            {{ row.lastCheckAt ? dateFormatSimple(row.lastCheckAt) : '—' }}
                        </template>
                    </el-table-column>
                    <el-table-column :label="$t('cluster.group')" prop="groupId" width="120px">
                        <template #default="{ row }">
                            {{ groupName(row.groupId) }}
                        </template>
                    </el-table-column>
                    <fu-table-operations
                        :ellipsis="2"
                        width="240px"
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
import { onMounted, reactive, ref } from 'vue';
import i18n from '@/lang';
import Operate from './components/operate.vue';
import {
    checkAllClusterNodes,
    deleteClusterNode,
    getClusterNodeOverview,
    searchClusterNodes,
    setClusterNodeLoginLock,
    testClusterNode,
} from '@/api/modules/cluster';
import { ElMessageBox } from 'element-plus';
import { getGroupList } from '@/api/modules/group';
import { Group } from '@/api/interface/group';
import { Cluster } from '@/api/interface/cluster';
import { MsgError, MsgSuccess, MsgWarning } from '@/utils/message';
import { dateFormatSimple } from '@/utils/date';
import { usePageState } from '@/composables/usePageState';

const loading = ref(false);
const checking = ref(false);
const operateRef = ref();
const deleteRef = ref();
const data = ref<Cluster.NodeInfo[]>([]);
const selects = ref([]);
const columns = ref([]);
const groups = ref<Group.GroupInfo[]>([]);
const overview = reactive<Cluster.NodeOverview>({
    total: 0,
    online: 0,
    offline: 0,
    unknown: 0,
});

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

const groupName = (groupId: number) => {
    if (!groupId) return '—';
    const group = groups.value.find((item) => item.id === groupId);
    return group ? group.name : '—';
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

const loadOverview = async () => {
    await getClusterNodeOverview()
        .then((res) => {
            overview.total = res.data.total;
            overview.online = res.data.online;
            overview.offline = res.data.offline;
            overview.unknown = res.data.unknown;
        })
        .catch(() => {
            // The request interceptor already reports the error.
        });
};

const listGroups = async () => {
    try {
        const res = await getGroupList('cluster_node');
        groups.value = res.data;
    } catch {
        // The request interceptor already reports the error.
    }
};

const checkAll = async () => {
    checking.value = true;
    await checkAllClusterNodes()
        .then(() => {
            MsgSuccess(i18n.global.t('commons.msg.operationSuccess'));
            search();
            loadOverview();
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

const openAddSelf = () => {
    operateRef.value.acceptParams({ isSelf: true });
};

const openEdit = (row: Cluster.NodeInfo) => {
    operateRef.value.acceptParams({ row });
};

const nowSeconds = () => Math.floor(Date.now() / 1000);

const loginLockLabel = (row: Cluster.NodeInfo) => {
    if (row.loginLocked === null || row.loginLocked === undefined) {
        return row.version ? i18n.global.t('cluster.loginUnknown') : i18n.global.t('cluster.loginUnsupported');
    }
    if (row.loginLocked) return i18n.global.t('cluster.loginLocked');
    if (row.loginUnlockUntil > nowSeconds()) {
        return i18n.global.t('cluster.loginOpenUntil', [dateFormatSimple(new Date(row.loginUnlockUntil * 1000))]);
    }
    return i18n.global.t('cluster.loginOpen');
};

const loginLockTag = (row: Cluster.NodeInfo) => {
    if (row.loginLocked === null || row.loginLocked === undefined) return 'info';
    if (row.loginLocked) return 'success';
    return row.loginUnlockUntil > nowSeconds() ? 'warning' : 'danger';
};

const applyLoginLock = async (row: Cluster.NodeInfo, locked: boolean, unlockMinutes: number) => {
    const msg = locked
        ? i18n.global.t('cluster.lockLoginHelper')
        : i18n.global.t('cluster.openLoginHelper', [row.name]);
    try {
        await ElMessageBox.confirm(
            msg,
            locked ? i18n.global.t('cluster.lockLogin') : i18n.global.t('cluster.loginLock'),
            {
                confirmButtonText: i18n.global.t('commons.button.confirm'),
                cancelButtonText: i18n.global.t('commons.button.cancel'),
                type: 'warning',
            },
        );
    } catch {
        return;
    }
    loading.value = true;
    try {
        const res = await setClusterNodeLoginLock({ nodeIds: [row.id], locked, unlockMinutes });
        const items = res.data || [];
        const failed = items.filter((item) => !item.success);
        if (failed.length) {
            MsgError(failed.map((item) => `${item.name || item.nodeId}: ${item.message}`).join('; '));
        } else {
            MsgSuccess(i18n.global.t('commons.msg.operationSuccess'));
        }
        await search();
    } finally {
        loading.value = false;
    }
};

const openDelete = (row: Cluster.NodeInfo) => {
    if (row.loginLocked) {
        MsgWarning(i18n.global.t('cluster.deleteLockedNodeHelper'));
    }
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
        label: i18n.global.t('cluster.openPanel'),
        click: (row: Cluster.NodeInfo) => {
            openPanel(row);
        },
    },
    {
        label: i18n.global.t('cluster.testConnection'),
        click: (row: Cluster.NodeInfo) => {
            testNode(row);
        },
    },
    {
        label: i18n.global.t('commons.button.edit'),
        click: (row: Cluster.NodeInfo) => {
            openEdit(row);
        },
    },
    {
        label: i18n.global.t('cluster.lockLogin'),
        disabled: (row: Cluster.NodeInfo) => row.status !== 'online' || row.loginLocked === true,
        click: (row: Cluster.NodeInfo) => {
            applyLoginLock(row, true, 0);
        },
    },
    {
        label: i18n.global.t('cluster.openLogin30'),
        disabled: (row: Cluster.NodeInfo) => row.status !== 'online',
        click: (row: Cluster.NodeInfo) => {
            applyLoginLock(row, false, 30);
        },
    },
    {
        label: i18n.global.t('cluster.openLogin120'),
        disabled: (row: Cluster.NodeInfo) => row.status !== 'online',
        click: (row: Cluster.NodeInfo) => {
            applyLoginLock(row, false, 120);
        },
    },
    {
        label: i18n.global.t('cluster.openLoginForever'),
        disabled: (row: Cluster.NodeInfo) => row.status !== 'online',
        click: (row: Cluster.NodeInfo) => {
            applyLoginLock(row, false, 0);
        },
    },
    {
        label: i18n.global.t('commons.button.delete'),
        click: (row: Cluster.NodeInfo) => {
            openDelete(row);
        },
    },
];

onMounted(async () => {
    listGroups();
    loadOverview();
    search();
});
</script>

<style scoped>
.mb-4 {
    margin-bottom: 16px;
}
</style>

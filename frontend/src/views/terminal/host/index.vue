<template>
    <div>
        <LayoutContent v-loading="loading" :title="$t('terminal.host', 2)">
            <template #leftToolBar>
                <el-button type="primary" @click="onOpenDialog('create')">
                    {{ $t('terminal.addHost') }}
                </el-button>
                <el-button type="primary" plain @click="onOpenGroupDialog()">
                    {{ $t('commons.table.group') }}
                </el-button>
                <el-button type="primary" plain :disabled="selects.length === 0" @click="onBatchDelete(null)">
                    {{ $t('commons.button.delete') }}
                </el-button>
            </template>
            <template #rightToolBar>
                <el-select v-model="group" @change="search()" clearable class="p-w-200">
                    <template #prefix>{{ $t('commons.table.group') }}</template>
                    <el-option :label="$t('commons.table.all')" value=""></el-option>
                    <el-option v-for="item in groupList" :key="item.id" :label="item.name" :value="item.id" />
                </el-select>
                <TableSearch @search="search()" v-model:searchName="info" />
                <TableRefresh @search="search()" />
            </template>
            <template #main>
                <el-alert
                    :title="$t('cluster.terminalStaysLocal')"
                    type="info"
                    :closable="false"
                    show-icon
                    class="p-mb-2"
                />
                <ComplexTable
                    :cache-pagination="false"
                    :pagination-config="paginationConfig"
                    v-model:selects="selects"
                    :data="data"
                    @search="search"
                >
                    <el-table-column type="selection" fix />
                    <el-table-column :label="$t('terminal.ip')" prop="addr" fix />
                    <el-table-column :label="$t('commons.login.username')" show-overflow-tooltip prop="user" />
                    <el-table-column :label="$t('commons.table.port')" prop="port" />
                    <el-table-column
                        :label="$t('commons.table.group')"
                        prop="group"
                        min-width="80"
                        show-overflow-tooltip
                    >
                        <template #default="{ row }">
                            <fu-select-rw-switch v-model="row.groupID" @change="updateGroup(row)">
                                <template #read>
                                    {{ row.groupBelong }}
                                </template>
                                <el-option
                                    v-for="item in groupList"
                                    :key="item.id"
                                    :label="item.name"
                                    :value="item.id"
                                />
                            </fu-select-rw-switch>
                        </template>
                    </el-table-column>
                    <el-table-column :label="$t('commons.table.title')" show-overflow-tooltip prop="name" />
                    <el-table-column
                        :label="$t('commons.table.description')"
                        show-overflow-tooltip
                        prop="description"
                    />
                    <el-table-column :label="$t('terminal.authMode')" min-width="100">
                        <template #default="{ row }">
                            {{
                                $t(
                                    row.authMode === 'passwordless'
                                        ? 'terminal.passwordless'
                                        : row.authMode === 'key'
                                          ? 'terminal.keyMode'
                                          : 'terminal.passwordMode',
                                )
                            }}
                        </template>
                    </el-table-column>
                    <el-table-column :label="$t('cluster.clusterStatus')" min-width="150">
                        <template #default="{ row }">
                            <template v-if="findHostLink(row)">
                                <el-tag
                                    size="small"
                                    :type="findHostLink(row)!.status === 'online' ? 'success' : 'danger'"
                                >
                                    {{
                                        findHostLink(row)!.status === 'online'
                                            ? $t('cluster.enrolledOnline')
                                            : $t('cluster.enrolledOffline')
                                    }}
                                </el-tag>
                                <span class="ml-1.5 text-xs text-gray-500">{{ findHostLink(row)!.nodeName }}</span>
                            </template>
                            <el-tag v-else size="small" type="info" effect="plain">
                                {{ $t('cluster.notEnrolled') }}
                            </el-tag>
                        </template>
                    </el-table-column>
                    <fu-table-operations width="240px" :buttons="buttons" :label="$t('commons.table.operate')" fix />
                </ComplexTable>
            </template>
        </LayoutContent>

        <OpDialog ref="opRef" @search="search" />
        <OperateDialog @search="search" ref="dialogRef" />
        <GroupDialog @search="search" ref="dialogGroupRef" />
        <ClusterNodeOperate @search="search" ref="clusterOperateRef" />
    </div>
</template>

<script setup lang="ts">
import { useSearchState } from '@/utils/search-reset';
import GroupDialog from '@/components/agent-group/index.vue';
import OperateDialog from '@/views/terminal/host/operate/index.vue';
import ClusterNodeOperate from '@/views/cluster/nodes/components/operate.vue';
import { deleteHost, editHostGroup, searchHosts } from '@/api/modules/terminal';
import { getAgentGroupList } from '@/api/modules/group';
import { listClusterHostLinks, testClusterNode } from '@/api/modules/cluster';
import { getAgentSettingInfo } from '@/api/modules/setting';
import { onMounted, toRef, reactive, ref } from 'vue';
import i18n from '@/lang';
import { Host } from '@/api/interface/host';
import { Cluster } from '@/api/interface/cluster';
import { MsgError, MsgSuccess } from '@/utils/message';
import { useGlobalStore } from '@/composables/useGlobalStore';
import { loadProductProFromDB } from '@/utils/xpack';
import { routerToNameWithQuery } from '@/utils/router';

const { currentNode, currentNodeAddr, defaultNetwork, isAdmin } = useGlobalStore();
const loading = ref();
const data = ref();
const groupList = ref();
const hostLinks = ref<Cluster.HostLink[]>([]);
const selects = ref<any>([]);
const paginationConfig = reactive({
    currentPage: 1,
    pageSize: 20,
    total: 0,
});
const info = ref();
useSearchState('terminal/host/index', {
    search: info,
    page: toRef(paginationConfig, 'currentPage'),
    pageSize: toRef(paginationConfig, 'pageSize'),
});
const group = ref<string>('');

const opRef = ref();
const clusterOperateRef = ref();

const acceptParams = () => {
    search();
};

const findHostLink = (row: Host.Host): Cluster.HostLink | undefined => {
    return hostLinks.value.find((item) => (item.hostId && item.hostId === row.id) || (item.addr && item.addr === row.addr));
};

const loadHostLinks = async () => {
    try {
        const res = await listClusterHostLinks();
        hostLinks.value = res.data || [];
    } catch {
        hostLinks.value = [];
    }
};

const onEnrollHost = (row: Host.Host) => {
    clusterOperateRef.value?.acceptParams({
        prefill: {
            name: row.name || row.addr,
            entryUrl: `https://${row.addr}:`,
            hostId: row.id,
            remark: row.description || '',
        },
    });
};

const onSwitchHostNode = async (row: Host.Host) => {
    const link = findHostLink(row);
    if (!link) return;
    if (!isAdmin.value) {
        MsgError(i18n.global.t('cluster.switchAdminOnly'));
        return;
    }
    loading.value = true;
    try {
        if (link.status !== 'online') {
            const res = await testClusterNode(link.nodeId);
            if (res?.data?.status !== 'online') {
                MsgError(res?.data?.lastError || i18n.global.t('cluster.switchTestFailed'));
                return;
            }
        }
        const settingRes = await getAgentSettingInfo(link.nodeName, { skipAuthRedirect: true });
        defaultNetwork.value = settingRes.data.defaultNetwork;
        localStorage.removeItem('dashboardCache');
        localStorage.removeItem('upgradeChecked');
        currentNode.value = link.nodeName;
        currentNodeAddr.value = link.addr || row.addr;
        loadProductProFromDB();
        routerToNameWithQuery('home', { t: Date.now() });
    } catch (e: any) {
        MsgError(e?.response?.data?.message || e?.message || i18n.global.t('cluster.switchTestFailed'));
    } finally {
        loading.value = false;
    }
};

const dialogRef = ref();
const onOpenDialog = async (
    title: string,
    rowData: Partial<Host.Host> = {
        port: 22,
        user: 'root',
        authMode: 'password',
    },
) => {
    let params = {
        title,
        rowData: { ...rowData },
    };
    dialogRef.value!.acceptParams(params);
};

const dialogGroupRef = ref();
const onOpenGroupDialog = () => {
    dialogGroupRef.value!.acceptParams({ type: 'host' });
};

const onBatchDelete = async (row: Host.Host | null) => {
    let names = [];
    let ids = [];
    if (row) {
        names = [row.name + '[' + row.addr + ']'];
        ids = [row.id];
    } else {
        selects.value.forEach((item: Host.Host) => {
            names.push(item.name + '[' + item.addr + ']');
            ids.push(item.id);
        });
    }
    opRef.value.acceptParams({
        title: i18n.global.t('commons.button.delete'),
        names: names,
        msg: i18n.global.t('commons.msg.operatorHelper', [
            i18n.global.t('terminal.host'),
            i18n.global.t('commons.button.delete'),
        ]),
        api: deleteHost,
        params: { ids: ids },
    });
};

const loadGroups = async () => {
    const res = await getAgentGroupList('host');
    groupList.value = res.data;
};

const updateGroup = async (row: any) => {
    await editHostGroup({ id: row.id, groupID: row.groupID });
    search();
    MsgSuccess(i18n.global.t('commons.msg.operationSuccess'));
};

const buttons = [
    {
        label: i18n.global.t('cluster.enrollToCluster'),
        show: (row: Host.Host) => !findHostLink(row),
        click: (row: Host.Host) => {
            onEnrollHost(row);
        },
    },
    {
        label: i18n.global.t('cluster.switchNode'),
        show: (row: Host.Host) => Boolean(findHostLink(row)),
        click: (row: Host.Host) => {
            onSwitchHostNode(row);
        },
    },
    {
        label: i18n.global.t('commons.button.edit'),
        click: (row: any) => {
            onOpenDialog('edit', row);
        },
    },
    {
        label: i18n.global.t('commons.button.delete'),
        click: (row: Host.Host) => {
            onBatchDelete(row);
        },
    },
];

const search = async () => {
    let params = {
        page: paginationConfig.currentPage,
        pageSize: paginationConfig.pageSize,
        groupID: Number(group.value),
        info: info.value,
    };
    loadGroups();
    loadHostLinks();
    loading.value = true;
    await searchHosts(params)
        .then((res) => {
            loading.value = false;
            data.value = res.data.items || [];
            paginationConfig.total = res.data.total;
        })
        .catch(() => {
            loading.value = false;
        });
};

onMounted(() => {
    search();
});

defineExpose({
    acceptParams,
});
</script>

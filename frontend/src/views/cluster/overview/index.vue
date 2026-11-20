<template>
    <div>
        <LayoutContent :title="$t('menu.clusterOverview')" v-loading="loading">
            <template #rightToolBar>
                <TableRefresh @search="loadAll()" />
            </template>
            <template #main>
                <el-row :gutter="12">
                    <!-- Left: node list, demo /xpack/node/dashboard style -->
                    <el-col :span="6">
                        <el-card shadow="never" class="node-list-card">
                            <template #header>
                                <div class="node-list-header">
                                    <el-select
                                        v-model="groupFilter"
                                        class="p-w-150"
                                        :placeholder="$t('cluster.group')"
                                        clearable
                                        @change="loadAll"
                                    >
                                        <el-option
                                            v-for="item in groups"
                                            :key="item.id"
                                            :label="item.name"
                                            :value="item.id"
                                        />
                                    </el-select>
                                    <el-button :icon="Setting" circle @click="goNodeManage" />
                                </div>
                                <el-input
                                    v-model="searchKeyword"
                                    :placeholder="$t('commons.button.search')"
                                    clearable
                                    class="mt-2"
                                />
                            </template>
                            <div
                                v-for="node in filteredNodes"
                                :key="node.key"
                                class="node-card"
                                :class="{ 'node-card-selected': selectedKey === node.key }"
                                @click="selectNode(node)"
                            >
                                <div class="node-card-title">
                                    <span class="node-card-name">{{ node.displayName }}</span>
                                    <el-tag :type="statusTagType(node.status)" size="small">
                                        {{ statusLabel(node.status) }}
                                    </el-tag>
                                </div>
                                <div class="node-card-addr">{{ node.addr }}</div>
                                <div class="node-card-metric">
                                    <span>
                                        {{ $t('cluster.cpu') }} {{ node.cpuUsedPercent.toFixed(1) }}% /
                                        {{ node.cpuTotal || '—' }}{{ $t('cluster.cores') }}
                                    </span>
                                    <el-progress
                                        :percentage="node.cpuUsedPercent"
                                        :show-text="false"
                                        :stroke-width="8"
                                    />
                                </div>
                                <div class="node-card-metric">
                                    <span>
                                        {{ $t('cluster.memory') }} {{ node.memUsedPercent.toFixed(1) }}% /
                                        {{ formatMem(node.memTotal) }}
                                    </span>
                                    <el-progress
                                        :percentage="node.memUsedPercent"
                                        :show-text="false"
                                        :stroke-width="8"
                                    />
                                </div>
                                <el-text v-if="node.fetchError" type="danger" size="small">
                                    {{ $t('cluster.fetchError') }}: {{ node.fetchError }}
                                </el-text>
                                <div class="node-card-actions" @click.stop>
                                    <el-button size="small" @click="openTerminal(node)">
                                        {{ $t('cluster.openTerminal') }}
                                    </el-button>
                                    <el-button size="small" @click="openNodeLog(node)">
                                        {{ $t('cluster.openLogs') }}
                                    </el-button>
                                    <el-tooltip :content="$t('cluster.restartUnsupported')" placement="top">
                                        <el-button size="small" disabled>
                                            {{ $t('commons.button.restart') }}
                                        </el-button>
                                    </el-tooltip>
                                </div>
                            </div>
                            <el-empty v-if="!filteredNodes.length" :description="$t('commons.msg.noneData')" />
                        </el-card>
                    </el-col>

                    <!-- Right: summary cards of the selected node -->
                    <el-col :span="18">
                        <div v-if="selectedNode">
                            <el-row :gutter="12" class="mb-4">
                                <el-col :span="8">
                                    <el-card shadow="hover">
                                        <el-statistic :title="$t('cluster.agent')" :value="0" />
                                        <el-text type="info" size="small">
                                            {{ $t('cluster.agentNoData') }}
                                        </el-text>
                                    </el-card>
                                </el-col>
                                <el-col :span="8">
                                    <el-card shadow="hover">
                                        <!-- Header counts come from the same live queries as
                                        the tables below (res.data.total), never from the
                                        aggregate snapshot, which can be stale or 0 for
                                        the master node. null = this batch failed to
                                        refresh: show —, not a stale-looking number. -->
                                        <el-statistic
                                            :title="$t('cluster.appCount')"
                                            :value="selectedNode.appCount ?? '—'"
                                        />
                                        <el-text v-if="selectedNode.appCount === null" type="warning" size="small">
                                            {{ $t('cluster.countsNotRefreshed') }}
                                        </el-text>
                                    </el-card>
                                </el-col>
                                <el-col :span="8">
                                    <el-card shadow="hover">
                                        <el-statistic
                                            :title="$t('cluster.websiteCount')"
                                            :value="selectedNode.websiteCount ?? '—'"
                                        />
                                        <el-text
                                            v-if="selectedNode.websiteCount === null"
                                            type="warning"
                                            size="small"
                                        >
                                            {{ $t('cluster.countsNotRefreshed') }}
                                        </el-text>
                                    </el-card>
                                </el-col>
                            </el-row>

                            <el-card shadow="never" class="mb-4">
                                <template #header>{{ $t('menu.app') }}</template>
                                <el-table :data="apps" style="width: 100%" v-loading="appsLoading">
                                    <el-table-column :label="$t('commons.table.name')" prop="name" min-width="160px" />
                                    <el-table-column :label="$t('commons.table.status')" prop="status" width="120px" />
                                    <el-table-column :label="$t('app.version')" prop="version" width="120px" />
                                    <el-table-column :label="$t('commons.table.operate')" width="120px" align="center">
                                        <template #default="{ row }">
                                            <el-button link type="primary" @click="openAppLog(row)">
                                                {{ $t('commons.button.log') }}
                                            </el-button>
                                        </template>
                                    </el-table-column>
                                </el-table>
                                <el-empty v-if="!apps.length && !appsLoading" :description="$t('commons.msg.noneData')" />
                            </el-card>

                            <el-card shadow="never" class="mb-4">
                                <template #header>{{ $t('menu.website') }}</template>
                                <el-table :data="websites" style="width: 100%" v-loading="websitesLoading">
                                    <el-table-column
                                        :label="$t('commons.table.name')"
                                        prop="primaryDomain"
                                        min-width="180px"
                                    />
                                    <el-table-column :label="$t('cluster.websiteType')" prop="type" width="110px" />
                                    <el-table-column :label="$t('commons.table.status')" prop="status" width="110px" />
                                    <el-table-column :label="$t('website.expireDate')" width="130px">
                                        <template #default="{ row }">
                                            {{ row.expireDate || '—' }}
                                        </template>
                                    </el-table-column>
                                    <el-table-column :label="$t('commons.table.operate')" width="120px" align="center">
                                        <template #default="{ row }">
                                            <el-button link type="primary" @click="openWebsiteBackup(row)">
                                                {{ $t('database.backupList') }}
                                            </el-button>
                                        </template>
                                    </el-table-column>
                                </el-table>
                                <el-empty
                                    v-if="!websites.length && !websitesLoading"
                                    :description="$t('commons.msg.noneData')"
                                />
                            </el-card>

                            <el-card shadow="never" class="mb-4">
                                <template #header>{{ $t('menu.ssl') }}</template>
                                <el-table :data="certs" style="width: 100%" v-loading="certsLoading">
                                    <el-table-column
                                        :label="$t('commons.table.name')"
                                        prop="primaryDomain"
                                        min-width="180px"
                                    />
                                    <el-table-column :label="$t('cluster.sslProvider')" prop="provider" width="130px" />
                                    <el-table-column :label="$t('website.expireDate')" width="150px">
                                        <template #default="{ row }">
                                            {{ row.expireDate || '—' }}
                                        </template>
                                    </el-table-column>
                                </el-table>
                                <el-empty v-if="!certs.length && !certsLoading" :description="$t('commons.msg.noneData')" />
                            </el-card>

                            <el-card shadow="never" class="mb-4">
                                <template #header>
                                    <div class="card-header-with-select">
                                        <span>{{ $t('menu.database') }}</span>
                                        <el-select v-model="dbType" class="p-w-150" @change="loadDatabases">
                                            <el-option label="MySQL" value="mysql" />
                                            <el-option label="PostgreSQL" value="postgresql" />
                                        </el-select>
                                        <el-select
                                            v-model="dbInstance"
                                            class="p-w-200"
                                            :disabled="!dbOptions.length"
                                            :placeholder="$t('cluster.dbNoInstance')"
                                            @change="loadDatabases"
                                        >
                                            <el-option
                                                v-for="item in dbOptions"
                                                :key="item.database"
                                                :label="item.database"
                                                :value="item.database"
                                            />
                                        </el-select>
                                    </div>
                                </template>
                                <el-table :data="databases" style="width: 100%" v-loading="databasesLoading">
                                    <el-table-column :label="$t('commons.table.name')" prop="name" min-width="160px" />
                                    <el-table-column
                                        :label="$t('cluster.dbUsername')"
                                        prop="username"
                                        min-width="140px"
                                    />
                                    <el-table-column :label="$t('commons.table.description')" min-width="160px">
                                        <template #default="{ row }">
                                            {{ row.description || row.format || '—' }}
                                        </template>
                                    </el-table-column>
                                    <el-table-column :label="$t('commons.table.operate')" width="120px" align="center">
                                        <template #default="{ row }">
                                            <el-button link type="primary" @click="openDbBackup(row)">
                                                {{ $t('database.backupList') }}
                                            </el-button>
                                        </template>
                                    </el-table-column>
                                </el-table>
                                <el-empty
                                    v-if="!databases.length && !databasesLoading"
                                    :description="$t('commons.msg.noneData')"
                                />
                            </el-card>

                            <el-card shadow="never">
                                <template #header>{{ $t('menu.cronjob') }}</template>
                                <el-table :data="cronjobs" style="width: 100%" v-loading="cronjobsLoading">
                                    <el-table-column :label="$t('commons.table.name')" prop="name" min-width="180px" />
                                    <el-table-column :label="$t('commons.table.status')" prop="status" width="120px" />
                                    <el-table-column :label="$t('cluster.cronSpec')" prop="spec" min-width="140px" />
                                    <el-table-column :label="$t('commons.table.operate')" width="120px" align="center">
                                        <template #default="{ row }">
                                            <el-button link type="primary" @click="runCronjob(row)">
                                                {{ $t('cluster.runOnce') }}
                                            </el-button>
                                        </template>
                                    </el-table-column>
                                </el-table>
                                <el-empty
                                    v-if="!cronjobs.length && !cronjobsLoading"
                                    :description="$t('commons.msg.noneData')"
                                />
                            </el-card>
                        </div>
                        <el-empty v-else :description="$t('commons.msg.noneData')" />
                    </el-col>
                </el-row>
            </template>
        </LayoutContent>

        <!-- Terminal drawer: same page, no jump to the remote panel URL -->
        <DrawerPro
            v-model="terminalVisible"
            :header="terminalTitle"
            :resource="terminalNodeName"
            size="70%"
            @close="terminalVisible = false"
        >
            <template #extra>
                <el-button @click="terminalVisible = false">{{ $t('commons.button.back') }}</el-button>
            </template>
            <template #content>
                <div v-if="terminalNodeOffline" class="drawer-note">
                    <el-alert type="warning" :closable="false" :title="$t('cluster.terminalNodeOffline')" />
                </div>
                <div v-if="terminalExpired" class="drawer-note">
                    <el-alert type="error" :closable="false" :title="$t('cluster.terminalDisconnected')">
                        <template #default>
                            <el-button type="primary" size="small" @click="reconnectTerminal">
                                {{ $t('cluster.terminalReconnect') }}
                            </el-button>
                            <span class="ml-2">{{ $t('cluster.terminalReconnectHelper') }}</span>
                        </template>
                    </el-alert>
                </div>
                <div class="terminal-drawer-body">
                    <Terminal ref="terminalRef" @session="terminalExpired = false" @expired="terminalExpired = true" />
                </div>
            </template>
        </DrawerPro>

        <!-- Node (system) log drawer -->
        <DrawerPro
            v-model="nodeLogVisible"
            :header="nodeLogTitle"
            :resource="nodeLogNodeName"
            size="70%"
            @close="nodeLogVisible = false"
        >
            <template #content>
                <el-form :inline="true" class="mb-4">
                    <el-form-item :label="$t('cluster.logService')">
                        <el-select v-model="logService" class="p-w-200" clearable @change="loadNodeLogs">
                            <el-option v-for="item in logServices" :key="item" :label="item" :value="item" />
                        </el-select>
                    </el-form-item>
                    <el-form-item :label="$t('commons.table.date')">
                        <el-date-picker v-model="logDate" type="date" @change="loadNodeLogs" />
                    </el-form-item>
                </el-form>
                <div class="log-black" v-loading="nodeLogLoading">
                    <div v-for="(item, idx) in nodeLogs" :key="idx" class="log-line">
                        {{ item.time }} {{ item.service }} {{ item.message }}
                    </div>
                    <div v-if="!nodeLogs.length && !nodeLogLoading">{{ $t('commons.msg.noneData') }}</div>
                </div>
            </template>
        </DrawerPro>

        <!-- Container / app log drawer -->
        <DrawerPro
            v-model="containerLogVisible"
            :header="containerLogTitle"
            :resource="containerLogName"
            size="70%"
            @close="containerLogVisible = false"
        >
            <template #content>
                <ContainerLog
                    :container="containerLogName"
                    :compose="containerLogCompose"
                    :node="containerLogNode"
                    :highlightDiff="420"
                />
            </template>
        </DrawerPro>

        <Backups ref="backupRef" />
    </div>
</template>

<script lang="ts" setup>
import { computed, nextTick, onMounted, ref } from 'vue';
import { useRouter } from 'vue-router';
import { Setting } from '@element-plus/icons-vue';
import i18n from '@/lang';
import { getClusterAggregateOverview, searchClusterNodes } from '@/api/modules/cluster';
import { getGroupList } from '@/api/modules/group';
import { searchAppInstalled } from '@/api/modules/app';
import { searchWebsites } from '@/api/modules/website';
import { searchSSL } from '@/api/modules/website';
import { searchMysqlDBs, searchPostgresqlDBs, listDatabases } from '@/api/modules/database';
import { searchCronjobPage, handleOnce } from '@/api/modules/cronjob';
import { listRunningServices, readSystemLogs } from '@/api/modules/log';
import { loadCurrentInfo } from '@/api/modules/dashboard';
import { App } from '@/api/interface/app';
import { Website } from '@/api/interface/website';
import { Database } from '@/api/interface/database';
import { Cronjob } from '@/api/interface/cronjob';
import { Log } from '@/api/interface/log';
import { Cluster } from '@/api/interface/cluster';
import { Group } from '@/api/interface/group';
import { MsgError, MsgSuccess } from '@/utils/message';
import { useGlobalStore } from '@/composables/useGlobalStore';
import Terminal from '@/components/terminal/index.vue';
import ContainerLog from '@/components/log/container/index.vue';
import Backups from '@/components/backup/index.vue';

const router = useRouter();
const { globalStore } = useGlobalStore();

interface OverviewNode {
    key: string;
    name: string;
    displayName: string;
    addr: string;
    status: string;
    cpuUsedPercent: number;
    cpuTotal: number;
    memUsedPercent: number;
    memTotal: number;
    appCount: number | null;
    websiteCount: number | null;
    groupId: number;
    fetchError: string;
}

const loading = ref(false);
const groups = ref<Group.GroupInfo[]>([]);
const groupFilter = ref<number | undefined>(undefined);
const searchKeyword = ref('');
const nodes = ref<OverviewNode[]>([]);
const selectedKey = ref('');
const apps = ref<App.AppInstallDto[]>([]);
const appsLoading = ref(false);
const websites = ref<Website.WebsiteRes[]>([]);
const websitesLoading = ref(false);
const certs = ref<Website.SSLDTO[]>([]);
const certsLoading = ref(false);
const dbType = ref<'mysql' | 'postgresql'>('mysql');
const databases = ref<Array<Database.MysqlDBInfo | Database.PostgresqlDBInfo>>([]);
const databasesLoading = ref(false);
// Database server instances of the selected node for the current dbType.
// The search APIs require a non-empty `database` (instance name), so the
// options are loaded first and the first available instance is used; when
// the node has no instance at all the card shows an empty state instead
// of calling search with an empty database (backend `required` error).
const dbOptions = ref<Array<Database.DatabaseOption>>([]);
const dbInstance = ref('');
const dbOptionsKey = ref('');
const cronjobs = ref<Cronjob.CronjobInfo[]>([]);
const cronjobsLoading = ref(false);

const terminalVisible = ref(false);
const terminalRef = ref();
const terminalNodeName = ref('');
const terminalNodeOffline = ref(false);
const terminalNode = ref<OverviewNode | undefined>(undefined);
const terminalExpired = ref(false);
const nodeLogVisible = ref(false);
const nodeLogNodeName = ref('');
const nodeLogNodeKey = ref('');
const logServices = ref<string[]>([]);
const logService = ref('');
const logDate = ref<Date>(new Date());
const nodeLogs = ref<Log.SystemLogItem[]>([]);
const nodeLogLoading = ref(false);
const containerLogVisible = ref(false);
const containerLogName = ref('');
const containerLogCompose = ref('');
const containerLogNode = ref('');
const backupRef = ref();

const selectedNode = computed(() => nodes.value.find((item) => item.key === selectedKey.value));
const filteredNodes = computed(() => {
    return nodes.value.filter((item) => {
        if (groupFilter.value && item.groupId !== groupFilter.value) return false;
        if (searchKeyword.value && !item.displayName.includes(searchKeyword.value)) return false;
        return true;
    });
});
const terminalTitle = computed(() => `${i18n.global.t('cluster.openTerminal')} - ${terminalNodeName.value}`);
const nodeLogTitle = computed(() => `${i18n.global.t('cluster.openLogs')} - ${nodeLogNodeName.value}`);
const containerLogTitle = computed(() => `${i18n.global.t('commons.button.log')} - ${containerLogName.value}`);

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
const formatMem = (bytes: number) => {
    if (!bytes) return '—';
    return `${(bytes / 1024 / 1024 / 1024).toFixed(2)} GB`;
};
// The summary APIs take a node name for the operateNode selector. Always
// pass it explicitly — even 'local' — so a globally switched node cannot
// leak into this page's per-node reads.
const nodeParam = (node: OverviewNode) => node.name;

const loadAll = async () => {
    loading.value = true;
    try {
        const [aggRes, nodesRes, groupsRes] = await Promise.allSettled([
            getClusterAggregateOverview(),
            searchClusterNodes({ page: 1, pageSize: 500 }),
            getGroupList('cluster_node'),
        ]);
        const entryUrls: Record<number, string> = {};
        const groupIds: Record<number, number> = {};
        if (nodesRes.status === 'fulfilled') {
            for (const item of nodesRes.value.data.items) {
                entryUrls[item.id] = item.entryUrl;
                groupIds[item.id] = item.groupId;
            }
        }
        if (groupsRes.status === 'fulfilled') {
            groups.value = groupsRes.value.data || [];
        }
        const list: OverviewNode[] = [];
        // Master node first, from the local dashboard (not part of the
        // aggregate, which only covers registered remote nodes).
        try {
            const local = await loadCurrentInfo('all', 'all', 'local');
            list.push({
                key: 'local',
                name: 'local',
                displayName: globalStore.getMasterAlias(),
                addr: '127.0.0.1',
                status: 'online',
                cpuUsedPercent: local.data.cpuUsedPercent || 0,
                cpuTotal: local.data.cpuTotal || 0,
                memUsedPercent: local.data.memoryUsedPercent || 0,
                memTotal: local.data.memoryTotal || 0,
                // Header counts are NOT taken from the aggregate snapshot
                // (the master node is not in it, so it used to show 0,
                // and a failed refresh kept a stale number). They are
                // filled by loadSelectedNodeData from the same live
                // queries as the tables below; null means not refreshed.
                appCount: null,
                websiteCount: null,
                groupId: 0,
                fetchError: '',
            });
        } catch {
            list.push({
                key: 'local',
                name: 'local',
                displayName: globalStore.getMasterAlias(),
                addr: '127.0.0.1',
                status: 'unknown',
                cpuUsedPercent: 0,
                cpuTotal: 0,
                memUsedPercent: 0,
                memTotal: 0,
                appCount: null,
                websiteCount: null,
                groupId: 0,
                fetchError: '',
            });
        }
        if (aggRes.status === 'fulfilled') {
            for (const snap of aggRes.value.data.nodes || []) {
                list.push({
                    key: `node-${snap.nodeId}`,
                    name: snap.name,
                    displayName: snap.name,
                    addr: (entryUrls[snap.nodeId] || '').replace(/\/$/, ''),
                    status: snap.status,
                    cpuUsedPercent: snap.cpuUsedPercent || 0,
                    cpuTotal: snap.cpuTotal || 0,
                    memUsedPercent: snap.memUsedPercent || 0,
                    memTotal: snap.memTotal || 0,
                    // Snapshot counts are not displayed: they come from a
                    // different query path (all:true / persisted row) than
                    // the tables below, so they can disagree with them.
                    // loadSelectedNodeData fills them from the live totals.
                    appCount: null,
                    websiteCount: null,
                    groupId: groupIds[snap.nodeId] || 0,
                    fetchError: snap.fetchError || '',
                });
            }
        }
        nodes.value = list;
        if (!selectedKey.value || !list.some((item) => item.key === selectedKey.value)) {
            selectedKey.value = list.length ? list[0].key : '';
        }
        if (selectedNode.value) {
            await loadSelectedNodeData();
        }
    } finally {
        loading.value = false;
    }
};

const selectNode = async (node: OverviewNode) => {
    if (selectedKey.value === node.key) return;
    selectedKey.value = node.key;
    await loadSelectedNodeData();
};

const loadSelectedNodeData = async () => {
    const node = selectedNode.value;
    if (!node) return;
    const param = nodeParam(node);
    appsLoading.value = true;
    websitesLoading.value = true;
    certsLoading.value = true;
    databasesLoading.value = true;
    cronjobsLoading.value = true;
    try {
        const [appsRes, sitesRes, certsRes, cronRes] = await Promise.allSettled([
            searchAppInstalled({ page: 1, pageSize: 8 }, param),
            searchWebsites(
                { page: 1, pageSize: 8, name: '', orderBy: 'created_at', order: 'descending', websiteGroupId: 0 },
                param,
            ),
            searchSSL({ page: 1, pageSize: 8 }, param),
            searchCronjobPage(
                { page: 1, pageSize: 8, info: '', groupIDs: [], orderBy: 'createdAt', order: 'null' },
                param,
            ),
        ]);
        apps.value = appsRes.status === 'fulfilled' ? appsRes.value.data.items || [] : [];
        websites.value = sitesRes.status === 'fulfilled' ? sitesRes.value.data.items || [] : [];
        certs.value = certsRes.status === 'fulfilled' ? certsRes.value.data.items || [] : [];
        cronjobs.value = cronRes.status === 'fulfilled' ? cronRes.value.data.items || [] : [];
        // Header counts use the totals of this same batch of live queries,
        // so the number always matches the tables below. A failed query
        // leaves null ("—" + 未刷新) instead of a stale snapshot value.
        node.appCount = appsRes.status === 'fulfilled' ? appsRes.value.data.total : null;
        node.websiteCount = sitesRes.status === 'fulfilled' ? sitesRes.value.data.total : null;
        await loadDatabases();
    } finally {
        appsLoading.value = false;
        websitesLoading.value = false;
        certsLoading.value = false;
        cronjobsLoading.value = false;
    }
};

const loadDatabases = async () => {
    const node = selectedNode.value;
    if (!node) return;
    databasesLoading.value = true;
    try {
        const param = nodeParam(node);
        // Instance options are per node + dbType: reload when either changes.
        const optionsKey = `${node.key}:${dbType.value}`;
        if (dbOptionsKey.value !== optionsKey) {
            dbOptions.value = [];
            dbInstance.value = '';
            try {
                const type =
                    dbType.value === 'mysql' ? 'mysql,mariadb,mysql-cluster' : 'postgresql,postgresql-cluster';
                const res = await listDatabases(type, param);
                dbOptions.value = res.data || [];
            } catch {
                dbOptions.value = [];
            }
            dbOptionsKey.value = optionsKey;
            if (dbOptions.value.length) {
                dbInstance.value = dbOptions.value[0].database;
            }
        }
        // No database server instance on this node: empty state, no search
        // call (searching with an empty `database` is a backend 400).
        if (!dbInstance.value) {
            databases.value = [];
            return;
        }
        const search = {
            info: '',
            database: dbInstance.value,
            page: 1,
            pageSize: 8,
            orderBy: 'createdAt',
            order: 'null',
        };
        if (dbType.value === 'mysql') {
            const res = await searchMysqlDBs(search, param);
            databases.value = res.data.items || [];
        } else {
            const res = await searchPostgresqlDBs(search, param);
            databases.value = res.data.items || [];
        }
    } catch {
        databases.value = [];
    } finally {
        databasesLoading.value = false;
    }
};

const goNodeManage = () => {
    router.push({ name: 'ClusterNodes' });
};

const openTerminal = (node: OverviewNode) => {
    terminalNodeName.value = node.displayName;
    terminalNodeOffline.value = node.status !== 'online';
    terminalNode.value = node;
    terminalExpired.value = false;
    terminalVisible.value = true;
    nextTick(() => {
        terminalRef.value?.acceptParams({
            endpoint: '/api/v2/hosts/terminal/local',
            args: '',
            error: '',
            initCmd: '',
            node: node.name,
        });
    });
};

// The Terminal component emits `expired` when the agent drops the session
// (lease revalidation failed, session not found, shell exited). Without
// this the drawer only shows one red "disconnected" line inside xterm;
// the alert below gives the user a reconnect button instead.
const reconnectTerminal = () => {
    const node = terminalNode.value;
    if (!node) return;
    terminalExpired.value = false;
    nextTick(() => {
        terminalRef.value?.acceptParams({
            endpoint: '/api/v2/hosts/terminal/local',
            args: '',
            error: '',
            initCmd: '',
            node: node.name,
        });
    });
};

const openNodeLog = async (node: OverviewNode) => {
    nodeLogNodeName.value = node.displayName;
    nodeLogNodeKey.value = node.key;
    nodeLogVisible.value = true;
    nodeLogs.value = [];
    logServices.value = [];
    logService.value = '';
    const param = nodeParam(node);
    try {
        const res = await listRunningServices(param);
        logServices.value = res.data || [];
    } catch {
        // Services list is best-effort; the log read still works without it.
    }
    await loadNodeLogs();
};

const loadNodeLogs = async () => {
    const node = nodes.value.find((item) => item.key === nodeLogNodeKey.value);
    if (!node) return;
    nodeLogLoading.value = true;
    try {
        const day = new Date(logDate.value);
        day.setHours(0, 0, 0, 0);
        const end = new Date(day);
        end.setDate(end.getDate() + 1);
        const res = await readSystemLogs(
            { pageSize: 500, service: logService.value || undefined, startTime: day, endTime: end },
            nodeParam(node),
        );
        nodeLogs.value = res.data?.items || [];
    } catch {
        nodeLogs.value = [];
    } finally {
        nodeLogLoading.value = false;
    }
};

const openAppLog = (row: App.AppInstallDto) => {
    containerLogName.value = row.name;
    containerLogCompose.value = row.path ? `${row.path}/docker-compose.yml` : '';
    containerLogNode.value = selectedNode.value?.name || 'local';
    containerLogVisible.value = true;
};

const openWebsiteBackup = (row: Website.WebsiteRes) => {
    backupRef.value?.acceptParams({
        type: 'website',
        name: row.alias,
        detailName: row.alias,
        status: row.status,
        node: selectedNode.value?.name,
    });
};

const openDbBackup = (row: Database.MysqlDBInfo | Database.PostgresqlDBInfo) => {
    // Backups takes the database server/instance as `name` and the database
    // itself as `detailName`, matching the database list pages.
    const instanceName =
        dbType.value === 'mysql'
            ? (row as Database.MysqlDBInfo).mysqlName || row.name
            : (row as Database.PostgresqlDBInfo).postgresqlName || row.name;
    backupRef.value?.acceptParams({
        type: dbType.value === 'mysql' ? 'mysql' : 'postgresql',
        name: instanceName,
        detailName: row.name,
        status: '',
        node: selectedNode.value?.name,
    });
};

const runCronjob = async (row: Cronjob.CronjobInfo) => {
    try {
        await handleOnce(row.id, nodeParam(selectedNode.value!));
        MsgSuccess(i18n.global.t('commons.msg.operationSuccess'));
    } catch {
        MsgError(i18n.global.t('commons.msg.operationFailed'));
    }
};

onMounted(() => {
    loadAll();
});
</script>

<style scoped>
.mb-4 {
    margin-bottom: 16px;
}

.mt-2 {
    margin-top: 8px;
}

.node-list-card {
    position: sticky;
    top: 12px;
}

.node-list-header {
    display: flex;
    gap: 8px;
    align-items: center;
}

.node-card {
    border: 1px solid var(--el-border-color);
    border-radius: 8px;
    padding: 12px;
    margin-top: 12px;
    cursor: pointer;
}

.node-card-selected {
    border-color: var(--el-color-primary);
    box-shadow: 0 0 0 1px var(--el-color-primary);
}

.node-card-title {
    display: flex;
    justify-content: space-between;
    align-items: center;
    gap: 8px;
}

.node-card-name {
    font-weight: 600;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
}

.node-card-addr {
    color: var(--el-text-color-secondary);
    font-size: 12px;
    margin: 6px 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
}

.node-card-metric {
    display: flex;
    flex-direction: column;
    gap: 4px;
    margin-top: 8px;
    font-size: 12px;
}

.node-card-actions {
    display: flex;
    gap: 8px;
    margin-top: 12px;
}

.card-header-with-select {
    display: flex;
    justify-content: space-between;
    align-items: center;
}

.log-black {
    background: #000;
    color: #eee;
    min-height: 420px;
    max-height: 60vh;
    overflow-y: auto;
    padding: 12px;
    font-family: monospace;
    font-size: 12px;
    line-height: 1.6;
    border-radius: 4px;
}

.log-line {
    white-space: pre-wrap;
    word-break: break-all;
}

.drawer-note {
    margin-bottom: 12px;
}

.terminal-drawer-body {
    height: 600px;
}
</style>

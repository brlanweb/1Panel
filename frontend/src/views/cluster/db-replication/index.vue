<template>
    <div>
        <LayoutContent :title="$t('menu.clusterDbReplication')" v-loading="loading">
            <template #leftToolBar>
                <el-button type="primary" @click="openCreate">
                    {{ $t('cluster.dbReplCreate') }}
                </el-button>
            </template>
            <template #rightToolBar>
                <TableRefresh @search="loadTopologies" />
            </template>
            <template #main>
                <el-alert :title="$t('cluster.dbReplAsyncNote')" type="info" :closable="false" class="mb-4" />
                <el-alert
                    v-if="tunnelHint"
                    :title="$t('cluster.dbReplSshTunnel')"
                    type="warning"
                    :closable="false"
                    class="mb-4"
                />
                <el-table :data="topologies" style="width: 100%" v-loading="loading">
                    <el-table-column :label="$t('cluster.dbReplName')" prop="name" min-width="140px" />
                    <el-table-column :label="$t('cluster.dbReplDbType')" prop="dbType" width="110px" />
                    <el-table-column :label="$t('cluster.dbReplMaster')" min-width="220px">
                        <template #default="{ row }">
                            <div>{{ row.masterName }} / {{ row.masterInstance }}</div>
                            <div class="text-sm text-gray-500">{{ row.database }} · {{ row.masterHost }}:{{ row.masterPort }}</div>
                        </template>
                    </el-table-column>
                    <el-table-column :label="$t('cluster.dbReplReplica')" min-width="220px">
                        <template #default="{ row }">
                            <div>{{ row.replicaName }} / {{ row.replicaInstance }}</div>
                            <div class="text-sm text-gray-500">{{ row.replicaDatabase }}</div>
                        </template>
                    </el-table-column>
                    <el-table-column :label="$t('cluster.dbReplStatus')" width="130px">
                        <template #default="{ row }">
                            <el-tag :type="statusTagType(row.status)" size="small">
                                {{ statusLabel(row.status) }}
                            </el-tag>
                        </template>
                    </el-table-column>
                    <el-table-column :label="$t('cluster.dbReplSnapshot')" width="130px">
                        <template #default="{ row }">
                            <span>{{ row.snapshotStatus || 'idle' }}</span>
                        </template>
                    </el-table-column>
                    <el-table-column :label="$t('cluster.dbReplBinlog')" width="170px">
                        <template #default="{ row }">
                            <span v-if="row.masterBinlogFile && row.masterBinlogFile !== 'OFF'">
                                {{ row.masterBinlogFile }} @ {{ row.masterBinlogPosition }}
                            </span>
                            <span v-else>—</span>
                        </template>
                    </el-table-column>
                    <el-table-column :label="$t('commons.table.operate')" width="150px" fixed="right">
                        <template #default="{ row }">
                            <el-button link type="primary" @click="openDetail(row)">
                                {{ $t('cluster.dbReplDetail') }}
                            </el-button>
                            <el-button link type="danger" @click="onDelete(row)">
                                {{ $t('commons.button.delete') }}
                            </el-button>
                        </template>
                    </el-table-column>
                </el-table>
                <el-empty v-if="!topologies.length && !loading" :description="$t('cluster.dbReplNoData')" />

                <!-- Detail drawer: setup steps, manual SQL, status, failover -->
                <el-drawer v-model="detailVisible" :title="detail?.name" size="62%" @close="stopPolling">
                    <div v-if="detail">
                        <el-alert
                            v-if="detail.failoverSuggested"
                            :title="$t('cluster.dbReplFailoverSuggested')"
                            type="warning"
                            :closable="false"
                            class="mb-4"
                        />
                        <el-descriptions :column="2" border class="mb-4">
                            <el-descriptions-item :label="$t('cluster.dbReplStatus')">
                                <el-tag :type="statusTagType(detail.status)" size="small">
                                    {{ statusLabel(detail.status) }}
                                </el-tag>
                            </el-descriptions-item>
                            <el-descriptions-item :label="$t('cluster.dbReplDelay')">
                                {{ detail.lastDelaySeconds === null || detail.lastDelaySeconds === undefined ? '—' : detail.lastDelaySeconds + 's' }}
                            </el-descriptions-item>
                            <el-descriptions-item :label="$t('cluster.dbReplMaster')">
                                {{ detail.masterName }} / {{ detail.masterInstance }} / {{ detail.database }}
                            </el-descriptions-item>
                            <el-descriptions-item :label="$t('cluster.dbReplReplica')">
                                {{ detail.replicaName }} / {{ detail.replicaInstance }} / {{ detail.replicaDatabase }}
                            </el-descriptions-item>
                            <el-descriptions-item :label="$t('cluster.dbReplBinlog')">
                                <span v-if="detail.masterBinlogFile && detail.masterBinlogFile !== 'OFF'">
                                    {{ detail.masterBinlogFile }} @ {{ detail.masterBinlogPosition }}
                                </span>
                                <span v-else>—</span>
                            </el-descriptions-item>
                            <el-descriptions-item :label="$t('cluster.dbReplLastChecked')">
                                {{ detail.lastCheckedAt || '—' }}
                            </el-descriptions-item>
                            <el-descriptions-item v-if="detail.lastError" :label="$t('cluster.dbReplLastError')" :span="2">
                                <span class="text-red-500">{{ detail.lastError }}</span>
                            </el-descriptions-item>
                            <el-descriptions-item v-if="detail.lastConfirmedAt" :label="$t('cluster.dbReplLastConfirmed')" :span="2">
                                {{ detail.lastConfirmedAt }}
                            </el-descriptions-item>
                        </el-descriptions>

                        <el-card shadow="never" class="mb-4">
                            <template #header>
                                <div class="flex justify-between items-center">
                                    <span>1 · {{ $t('cluster.dbReplPreflight') }}</span>
                                    <el-button type="primary" size="small" :loading="preflighting" @click="runPreflight">
                                        {{ $t('cluster.dbReplRunPreflight') }}
                                    </el-button>
                                </div>
                            </template>
                            <el-table v-if="preflight.length" :data="preflight" style="width: 100%">
                                <el-table-column :label="$t('commons.table.name')" prop="name" min-width="160px" />
                                <el-table-column :label="$t('cluster.dbReplStatus')" width="120px">
                                    <template #default="{ row }">
                                        <el-tag :type="preflightTagType(row.status)" size="small">
                                            {{ preflightLabel(row.status) }}
                                        </el-tag>
                                    </template>
                                </el-table-column>
                                <el-table-column :label="$t('commons.table.description')" prop="detail" min-width="220px" />
                            </el-table>
                            <el-empty v-else :description="$t('cluster.dbReplRunPreflight')" />
                        </el-card>

                        <el-card shadow="never" class="mb-4">
                            <template #header>
                                <span>2 · {{ $t('cluster.dbReplManualSteps') }}</span>
                            </template>
                            <p class="text-sm text-gray-500 mb-3">{{ $t('cluster.dbReplManualStepsHelper') }}</p>
                            <div v-for="step in manualSteps" :key="step.key" class="mb-4">
                                <div class="flex justify-between items-center mb-1">
                                    <el-checkbox v-model="step.done" @change="onManualStepChange(step)">
                                        {{ step.title }}
                                    </el-checkbox>
                                    <el-button link type="primary" size="small" @click="copyText(step.sql)">
                                        {{ $t('cluster.dbReplCopySql') }}
                                    </el-button>
                                </div>
                                <pre class="sql-block">{{ step.sql }}</pre>
                            </div>
                            <el-empty v-if="!manualSteps.length" :description="$t('commons.msg.noneData')" />
                        </el-card>

                        <el-card shadow="never" class="mb-4">
                            <template #header>
                                <div class="flex justify-between items-center">
                                    <span>3 · {{ $t('cluster.dbReplConfigure') }}</span>
                                    <el-button
                                        type="primary"
                                        size="small"
                                        :loading="configuring"
                                        :disabled="detail.status === 'running'"
                                        @click="onConfigure"
                                    >
                                        {{ $t('cluster.dbReplConfigure') }}
                                    </el-button>
                                </div>
                            </template>
                            <p class="text-sm text-gray-500">{{ $t('cluster.dbReplConfigureHelper') }}</p>
                        </el-card>

                        <el-card shadow="never" class="mb-4">
                            <template #header>
                                <div class="flex justify-between items-center">
                                    <span>4 · {{ $t('cluster.dbReplSnapshot') }}</span>
                                    <el-button
                                        type="primary"
                                        size="small"
                                        :loading="snapshotStarting"
                                        :disabled="detail.snapshotStatus === 'running'"
                                        @click="onStartSnapshot"
                                    >
                                        {{ $t('cluster.dbReplSnapshotStart') }}
                                    </el-button>
                                </div>
                            </template>
                            <p class="text-sm text-gray-500 mb-2">{{ $t('cluster.dbReplSnapshotHelper') }}</p>
                            <p v-if="detail.snapshotDetail" class="text-sm">{{ detail.snapshotDetail }}</p>
                        </el-card>

                        <el-card shadow="never" class="mb-4">
                            <template #header>
                                <div class="flex justify-between items-center">
                                    <span>5 · {{ $t('cluster.dbReplStartReplication') }}</span>
                                    <el-button
                                        type="success"
                                        size="small"
                                        :loading="confirming"
                                        :disabled="detail.status !== 'awaiting_replication'"
                                        @click="onConfirmStarted"
                                    >
                                        {{ $t('cluster.dbReplConfirmStarted') }}
                                    </el-button>
                                </div>
                            </template>
                            <p class="text-sm text-gray-500">{{ $t('cluster.dbReplConfirmStartedHelper') }}</p>
                        </el-card>

                        <el-card shadow="never" class="mb-4">
                            <template #header>
                                <div class="flex justify-between items-center">
                                    <span>{{ $t('cluster.dbReplStatus') }}</span>
                                    <el-button type="primary" plain size="small" :loading="refreshing" @click="onRefresh">
                                        {{ $t('cluster.dbReplRefresh') }}
                                    </el-button>
                                </div>
                            </template>
                            <p class="text-sm text-gray-500">
                                {{ $t('cluster.dbReplDelay') }}：—（{{ $t('cluster.dbReplDelayUnknown') }}）
                            </p>
                        </el-card>

                        <el-card shadow="never" class="mb-4">
                            <template #header>
                                <div class="flex justify-between items-center">
                                    <span>{{ $t('cluster.dbReplFailover') }}</span>
                                    <el-button
                                        type="danger"
                                        size="small"
                                        :disabled="detail.status !== 'running' && detail.status !== 'error'"
                                        @click="openFailover"
                                    >
                                        {{ $t('cluster.dbReplFailover') }}
                                    </el-button>
                                </div>
                            </template>
                            <p class="text-sm text-gray-500">{{ $t('cluster.dbReplFailoverHelper') }}</p>
                            <el-table v-if="failovers.length" :data="failovers" style="width: 100%" class="mt-2">
                                <el-table-column :label="$t('cluster.dbReplFailover')" min-width="200px">
                                    <template #default="{ row }">{{ row.fromName }} → {{ row.toName }}</template>
                                </el-table-column>
                                <el-table-column :label="$t('cluster.dbReplFailoverReason')" prop="reason" min-width="140px" />
                                <el-table-column :label="$t('commons.table.createTime')" prop="createdAt" width="170px" />
                            </el-table>
                        </el-card>
                    </div>
                </el-drawer>

                <!-- Create dialog -->
                <el-dialog v-model="createVisible" :title="$t('cluster.dbReplCreate')" width="640px">
                    <el-form :model="form" label-width="130px" :rules="rules" ref="formRef">
                        <el-form-item :label="$t('cluster.dbReplName')" prop="name">
                            <el-input v-model="form.name" />
                        </el-form-item>
                        <el-form-item :label="$t('cluster.dbReplDbType')" prop="dbType">
                            <el-select v-model="form.dbType" @change="onTypeChange">
                                <el-option label="MySQL" value="mysql" />
                                <el-option label="MariaDB" value="mariadb" />
                                <el-option label="PostgreSQL" value="postgresql" />
                            </el-select>
                        </el-form-item>
                        <el-form-item :label="$t('cluster.dbReplMasterNode')" prop="masterNodeId">
                            <el-select v-model="form.masterNodeId" @change="onMasterNodeChange">
                                <el-option
                                    v-for="item in nodeOptions"
                                    :key="item.id"
                                    :label="item.name"
                                    :value="item.id"
                                />
                            </el-select>
                        </el-form-item>
                        <el-form-item :label="$t('cluster.dbReplMasterInstance')" prop="masterInstance">
                            <el-select v-model="form.masterInstance" @change="onMasterInstanceChange" :disabled="!masterInstances.length">
                                <el-option
                                    v-for="item in masterInstances"
                                    :key="item.name"
                                    :label="`${item.name}（${item.version}）`"
                                    :value="item.name"
                                />
                            </el-select>
                        </el-form-item>
                        <el-form-item :label="$t('cluster.dbReplDatabase')" prop="database">
                            <el-select v-model="form.database" filterable allow-create :disabled="!masterSchemas.length">
                                <el-option v-for="item in masterSchemas" :key="item.name" :label="item.name" :value="item.name" />
                            </el-select>
                        </el-form-item>
                        <el-form-item :label="$t('cluster.dbReplMasterHost')" prop="masterHost">
                            <el-input v-model="form.masterHost" />
                            <div class="text-sm text-gray-500">{{ $t('cluster.dbReplMasterHostHelper') }}</div>
                        </el-form-item>
                        <el-form-item :label="$t('cluster.dbReplMasterPort')" prop="masterPort">
                            <el-input-number v-model="form.masterPort" :min="1" :max="65535" />
                        </el-form-item>
                        <el-form-item :label="$t('cluster.dbReplReplicaNode')" prop="replicaNodeId">
                            <el-select v-model="form.replicaNodeId" @change="onReplicaNodeChange">
                                <el-option
                                    v-for="item in replicaNodeOptions"
                                    :key="item.id"
                                    :label="item.name"
                                    :value="item.id"
                                />
                            </el-select>
                        </el-form-item>
                        <el-form-item :label="$t('cluster.dbReplReplicaInstance')" prop="replicaInstance">
                            <el-select v-model="form.replicaInstance" :disabled="!replicaInstances.length">
                                <el-option
                                    v-for="item in replicaInstances"
                                    :key="item.name"
                                    :label="`${item.name}（${item.version}）`"
                                    :value="item.name"
                                />
                            </el-select>
                        </el-form-item>
                        <el-form-item :label="$t('cluster.dbReplReplicaDatabase')" prop="replicaDatabase">
                            <el-input v-model="form.replicaDatabase" />
                        </el-form-item>
                        <el-form-item :label="$t('cluster.dbReplUsername')" prop="replUsername">
                            <el-input v-model="form.replUsername" />
                        </el-form-item>
                        <el-form-item :label="$t('cluster.dbReplPassword')" prop="replPassword">
                            <el-input v-model="form.replPassword" show-password />
                            <div class="text-sm text-gray-500">{{ $t('cluster.dbReplPasswordHelper') }}</div>
                        </el-form-item>
                    </el-form>
                    <template #footer>
                        <el-button @click="createVisible = false">{{ $t('commons.button.cancel') }}</el-button>
                        <el-button type="primary" :loading="creating" @click="onCreate">
                            {{ $t('commons.button.confirm') }}
                        </el-button>
                    </template>
                </el-dialog>

                <!-- Failover dialog -->
                <el-dialog v-model="failoverVisible" :title="$t('cluster.dbReplFailover')" width="640px">
                    <p class="text-sm text-gray-500 mb-3">{{ $t('cluster.dbReplFailoverHelper') }}</p>
                    <div v-for="step in promoteSteps" :key="step.key" class="mb-4">
                        <div class="flex justify-between items-center mb-1">
                            <el-checkbox v-model="step.done">{{ step.title }}</el-checkbox>
                            <el-button link type="primary" size="small" @click="copyText(step.sql)">
                                {{ $t('cluster.dbReplCopySql') }}
                            </el-button>
                        </div>
                        <pre class="sql-block">{{ step.sql }}</pre>
                    </div>
                    <el-form label-width="110px">
                        <el-form-item :label="$t('cluster.dbReplFailoverReason')">
                            <el-input v-model="failoverReason" />
                        </el-form-item>
                    </el-form>
                    <template #footer>
                        <el-button @click="failoverVisible = false">{{ $t('commons.button.cancel') }}</el-button>
                        <el-button type="danger" :loading="promoting" @click="onPromote">
                            {{ $t('commons.button.confirm') }}
                        </el-button>
                    </template>
                </el-dialog>
            </template>
        </LayoutContent>
    </div>
</template>

<script lang="ts" setup>
import { computed, onMounted, onUnmounted, reactive, ref } from 'vue';
import i18n from '@/lang';
import {
    searchClusterNodes,
    searchDbReplications,
    getDbReplication,
    createDbReplication,
    deleteDbReplication,
    searchDbInstances,
    searchDbSchemas,
    preflightDbReplication,
    listDbManualSteps,
    markDbManualStepsDone,
    configureDbReplication,
    startDbSnapshot,
    confirmDbReplicationStarted,
    refreshDbReplication,
    listDbPromoteSteps,
    promoteDbReplication,
    listDbFailovers,
} from '@/api/modules/cluster';
import { Cluster } from '@/api/interface/cluster';
import { MsgError, MsgSuccess } from '@/utils/message';
import { copyText } from '@/utils/clipboard';
import { ElMessageBox } from 'element-plus';

const loading = ref(false);
const topologies = ref<Cluster.DbReplicationInfo[]>([]);
const tunnelHint = ref(true);

const nodeOptions = ref<{ id: number; name: string; entryUrl: string }[]>([]);

const statusLabel = (status: string) => {
    const map: Record<string, string> = {
        draft: i18n.global.t('cluster.dbReplStatusDraft'),
        ready: i18n.global.t('cluster.dbReplStatusReady'),
        configuring: i18n.global.t('cluster.dbReplStatusConfiguring'),
        snapshot: i18n.global.t('cluster.dbReplStatusSnapshot'),
        awaiting_replication: i18n.global.t('cluster.dbReplStatusAwaiting'),
        running: i18n.global.t('cluster.dbReplStatusRunning'),
        error: i18n.global.t('cluster.dbReplStatusError'),
        failed_over: i18n.global.t('cluster.dbReplStatusFailedOver'),
    };
    return map[status] || status;
};

const statusTagType = (status: string) => {
    const map: Record<string, string> = {
        draft: 'info',
        ready: '',
        configuring: 'warning',
        snapshot: 'warning',
        awaiting_replication: 'warning',
        running: 'success',
        error: 'danger',
        failed_over: 'warning',
    };
    return map[status] || 'info';
};

const preflightLabel = (status: string) => {
    const map: Record<string, string> = {
        pass: i18n.global.t('cluster.dbReplPass'),
        fail: i18n.global.t('cluster.dbReplFail'),
        warn: i18n.global.t('cluster.dbReplWarn'),
        manual: i18n.global.t('cluster.dbReplManual'),
    };
    return map[status] || status;
};

const preflightTagType = (status: string) => {
    const map: Record<string, string> = {
        pass: 'success',
        fail: 'danger',
        warn: 'warning',
        manual: 'info',
    };
    return map[status] || 'info';
};

const loadTopologies = async () => {
    loading.value = true;
    try {
        const res = await searchDbReplications();
        topologies.value = res.data || [];
    } finally {
        loading.value = false;
    }
};

const loadNodes = async () => {
    try {
        const res = await searchClusterNodes({ page: 1, pageSize: 500 });
        const items = res.data.items || [];
        nodeOptions.value = [
            { id: 0, name: i18n.global.t('cluster.localNode'), entryUrl: '' },
            ...items.filter((n) => n.status === 'online').map((n) => ({ id: n.id, name: n.name, entryUrl: n.entryUrl })),
        ];
    } catch {
        nodeOptions.value = [{ id: 0, name: i18n.global.t('cluster.localNode'), entryUrl: '' }];
    }
};

const replicaNodeOptions = computed(() =>
    nodeOptions.value.filter((n) => n.id !== form.masterNodeId),
);

// ---------- create ----------
const createVisible = ref(false);
const creating = ref(false);
const formRef = ref();
const form = reactive<Cluster.DbReplicationCreate>({
    name: '',
    dbType: 'mysql',
    masterNodeId: 0,
    masterInstance: '',
    masterHost: '',
    masterPort: 3306,
    database: '',
    replicaNodeId: undefined as unknown as number,
    replicaInstance: '',
    replicaDatabase: '',
    replUsername: 'repl',
    replPassword: '',
});
const rules = {
    name: [{ required: true, message: i18n.global.t('commons.msg.inputOrSelect'), trigger: 'blur' }],
    masterInstance: [{ required: true, message: i18n.global.t('commons.msg.inputOrSelect'), trigger: 'change' }],
    database: [{ required: true, message: i18n.global.t('commons.msg.inputOrSelect'), trigger: 'change' }],
    masterHost: [{ required: true, message: i18n.global.t('commons.msg.inputOrSelect'), trigger: 'blur' }],
    masterPort: [{ required: true, message: i18n.global.t('commons.msg.inputOrSelect'), trigger: 'blur' }],
    replicaInstance: [{ required: true, message: i18n.global.t('commons.msg.inputOrSelect'), trigger: 'change' }],
    replicaDatabase: [{ required: true, message: i18n.global.t('commons.msg.inputOrSelect'), trigger: 'blur' }],
    replUsername: [{ required: true, message: i18n.global.t('commons.msg.inputOrSelect'), trigger: 'blur' }],
    replPassword: [{ required: true, message: i18n.global.t('commons.msg.inputOrSelect'), trigger: 'blur' }],
};
const masterInstances = ref<Cluster.DbInstance[]>([]);
const replicaInstances = ref<Cluster.DbInstance[]>([]);
const masterSchemas = ref<Cluster.DbSchema[]>([]);

const openCreate = () => {
    form.name = '';
    form.dbType = 'mysql';
    form.masterNodeId = 0;
    form.masterInstance = '';
    form.masterHost = '';
    form.masterPort = 3306;
    form.database = '';
    form.replicaNodeId = undefined as unknown as number;
    form.replicaInstance = '';
    form.replicaDatabase = '';
    form.replUsername = 'repl';
    form.replPassword = '';
    masterInstances.value = [];
    replicaInstances.value = [];
    masterSchemas.value = [];
    createVisible.value = true;
    loadMasterInstances();
};

const hostFromEntryUrl = (entryUrl: string) => {
    try {
        return new URL(entryUrl).hostname;
    } catch {
        return '';
    }
};

const onTypeChange = () => {
    form.masterPort = form.dbType === 'postgresql' ? 5432 : 3306;
    form.masterInstance = '';
    form.replicaInstance = '';
    form.database = '';
    masterInstances.value = [];
    replicaInstances.value = [];
    masterSchemas.value = [];
    loadMasterInstances();
    if (form.replicaNodeId !== undefined) {
        loadReplicaInstances();
    }
};

const onMasterNodeChange = () => {
    const node = nodeOptions.value.find((n) => n.id === form.masterNodeId);
    if (node && node.entryUrl) {
        form.masterHost = hostFromEntryUrl(node.entryUrl);
    }
    form.masterInstance = '';
    form.database = '';
    masterInstances.value = [];
    masterSchemas.value = [];
    loadMasterInstances();
};

const loadMasterInstances = async () => {
    try {
        const res = await searchDbInstances(form.masterNodeId, form.dbType);
        masterInstances.value = res.data || [];
    } catch {
        masterInstances.value = [];
    }
};

const onMasterInstanceChange = async () => {
    form.database = '';
    masterSchemas.value = [];
    try {
        const res = await searchDbSchemas(form.masterNodeId, form.dbType, form.masterInstance);
        masterSchemas.value = res.data || [];
    } catch {
        masterSchemas.value = [];
    }
};

const onReplicaNodeChange = () => {
    form.replicaInstance = '';
    replicaInstances.value = [];
    loadReplicaInstances();
};

const loadReplicaInstances = async () => {
    try {
        const res = await searchDbInstances(form.replicaNodeId, form.dbType);
        replicaInstances.value = res.data || [];
    } catch {
        replicaInstances.value = [];
    }
};

const onCreate = async () => {
    await formRef.value.validate();
    if (form.masterNodeId === form.replicaNodeId) {
        MsgError(i18n.global.t('cluster.dbReplSameNode'));
        return;
    }
    creating.value = true;
    try {
        await createDbReplication(form);
        MsgSuccess(i18n.global.t('commons.msg.createSuccess'));
        createVisible.value = false;
        loadTopologies();
    } finally {
        creating.value = false;
    }
};

const onDelete = async (row: Cluster.DbReplicationInfo) => {
    try {
        await ElMessageBox.confirm(
            i18n.global.t('cluster.dbReplDeleteHelper', [row.name]),
            i18n.global.t('commons.button.delete'),
            { type: 'warning' },
        );
    } catch {
        return;
    }
    await deleteDbReplication(row.id);
    MsgSuccess(i18n.global.t('commons.msg.deleteSuccess'));
    loadTopologies();
};

// ---------- detail ----------
const detailVisible = ref(false);
const detail = ref<Cluster.DbReplicationInfo | null>(null);
const preflight = ref<Cluster.DbPreflightCheck[]>([]);
const manualSteps = ref<Cluster.DbManualStep[]>([]);
const failovers = ref<Cluster.DbFailoverInfo[]>([]);
const preflighting = ref(false);
const configuring = ref(false);
const snapshotStarting = ref(false);
const confirming = ref(false);
const refreshing = ref(false);
let pollTimer: number | undefined;

const openDetail = async (row: Cluster.DbReplicationInfo) => {
    detail.value = row;
    detailVisible.value = true;
    await refreshDetail();
};

const refreshDetail = async () => {
    if (!detail.value) return;
    const id = detail.value.id;
    try {
        const res = await getDbReplication(id);
        detail.value = res.data;
    } catch {
        // Keep the last known detail on refresh failure.
    }
    try {
        const steps = await listDbManualSteps(id);
        manualSteps.value = steps.data || [];
    } catch {
        manualSteps.value = [];
    }
    try {
        const res = await listDbFailovers(id);
        failovers.value = res.data || [];
    } catch {
        failovers.value = [];
    }
    if (detail.value?.preflight) {
        try {
            preflight.value = JSON.parse(detail.value.preflight);
        } catch {
            preflight.value = [];
        }
    }
    if (detail.value?.snapshotStatus === 'running') {
        startPolling();
    } else {
        stopPolling();
    }
};

const startPolling = () => {
    stopPolling();
    pollTimer = window.setInterval(async () => {
        if (!detail.value) return;
        await refreshDetail();
        if (detail.value?.snapshotStatus !== 'running') {
            stopPolling();
        }
    }, 5000);
};

const stopPolling = () => {
    if (pollTimer !== undefined) {
        window.clearInterval(pollTimer);
        pollTimer = undefined;
    }
};

const runPreflight = async () => {
    if (!detail.value) return;
    preflighting.value = true;
    try {
        const res = await preflightDbReplication(detail.value.id);
        preflight.value = res.data || [];
        await refreshDetail();
    } finally {
        preflighting.value = false;
    }
};

const onManualStepChange = async (step: Cluster.DbManualStep) => {
    if (!detail.value) return;
    const doneKeys = manualSteps.value.filter((s) => s.done).map((s) => s.key);
    try {
        await markDbManualStepsDone(detail.value.id, doneKeys);
    } catch {
        step.done = !step.done;
    }
};

const onConfigure = async () => {
    if (!detail.value) return;
    try {
        await ElMessageBox.confirm(
            i18n.global.t('cluster.dbReplConfigureConfirm'),
            i18n.global.t('cluster.dbReplConfigure'),
            { type: 'warning' },
        );
    } catch {
        return;
    }
    configuring.value = true;
    try {
        await configureDbReplication(detail.value.id);
        MsgSuccess(i18n.global.t('commons.msg.operationSuccess'));
        await refreshDetail();
    } finally {
        configuring.value = false;
    }
};

const onStartSnapshot = async () => {
    if (!detail.value) return;
    snapshotStarting.value = true;
    try {
        await startDbSnapshot(detail.value.id);
        MsgSuccess(i18n.global.t('commons.msg.operationSuccess'));
        await refreshDetail();
    } finally {
        snapshotStarting.value = false;
    }
};

const onConfirmStarted = async () => {
    if (!detail.value) return;
    confirming.value = true;
    try {
        const doneKeys = manualSteps.value.filter((s) => s.done).map((s) => s.key);
        await confirmDbReplicationStarted(detail.value.id, doneKeys);
        MsgSuccess(i18n.global.t('commons.msg.operationSuccess'));
        await refreshDetail();
        loadTopologies();
    } finally {
        confirming.value = false;
    }
};

const onRefresh = async () => {
    if (!detail.value) return;
    refreshing.value = true;
    try {
        const res = await refreshDbReplication(detail.value.id);
        detail.value = res.data;
        loadTopologies();
    } finally {
        refreshing.value = false;
    }
};

// ---------- failover ----------
const failoverVisible = ref(false);
const promoteSteps = ref<Cluster.DbManualStep[]>([]);
const failoverReason = ref('');
const promoting = ref(false);

const openFailover = async () => {
    if (!detail.value) return;
    failoverReason.value = '';
    try {
        const res = await listDbPromoteSteps(detail.value.id);
        promoteSteps.value = res.data || [];
    } catch {
        promoteSteps.value = [];
    }
    failoverVisible.value = true;
};

const onPromote = async () => {
    if (!detail.value) return;
    try {
        await ElMessageBox.confirm(
            i18n.global.t('cluster.dbReplFailoverConfirm', [detail.value.replicaName]),
            i18n.global.t('cluster.dbReplFailover'),
            { type: 'warning' },
        );
    } catch {
        return;
    }
    promoting.value = true;
    try {
        const doneKeys = promoteSteps.value.filter((s) => s.done).map((s) => s.key);
        await promoteDbReplication(detail.value.id, failoverReason.value, doneKeys);
        MsgSuccess(i18n.global.t('commons.msg.operationSuccess'));
        failoverVisible.value = false;
        detailVisible.value = false;
        loadTopologies();
    } finally {
        promoting.value = false;
    }
};

onMounted(() => {
    loadNodes();
    loadTopologies();
});

onUnmounted(() => {
    stopPolling();
});
</script>

<style scoped lang="scss">
.sql-block {
    background: var(--el-fill-color-light);
    border: 1px solid var(--el-border-color-lighter);
    border-radius: 4px;
    padding: 10px 12px;
    white-space: pre-wrap;
    word-break: break-all;
    font-family: monospace;
    font-size: 12px;
    margin: 0;
}
</style>

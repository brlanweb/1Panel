<template>
    <div>
        <RouterButton :buttons="buttons" />
        <LayoutContent v-loading="loading" :title="$t('compute.pool.title')">
            <template #leftToolBar>
                <el-button type="primary" @click="createRef.open()">{{ $t('compute.pool.create') }}</el-button>
                <el-button @click="clusterVisible = true">{{ $t('compute.pool.cluster') }}</el-button>
                <el-button @click="accessVisible = true">{{ $t('compute.pool.routerSetting') }}</el-button>
            </template>
            <template #rightToolBar>
                <span class="sync-hint">
                    <el-tag v-if="paused" type="info" size="small">{{ $t('compute.paused') }}</el-tag>
                    <span v-else-if="refreshedAt">{{ $t('compute.refreshedAt') }} {{ refreshedAt }}</span>
                </span>
                <span class="sub mr-2">{{ $t('compute.policy.autoSwitch') }}</span>
                <el-switch
                    v-model="autoEnabled"
                    :loading="savingSwitch"
                    :active-text="$t('compute.policy.autoOn')"
                    :inactive-text="$t('compute.policy.autoOff')"
                    @change="onAutoSwitch"
                />
                <el-button class="ml-2" :icon="Refresh" @click="tick(true)" />
            </template>
            <template #prompt>
                <el-alert type="info" :closable="false" :title="$t('compute.pool.helper')" />
                <el-alert
                    v-if="setting && setting.enabled && !setting.running"
                    class="mt-2"
                    type="error"
                    :closable="false"
                    show-icon
                    :title="$t('compute.pool.routerDown', [setting.listen, setting.error || ''])"
                />
            </template>
            <template #main>
                <div v-if="suggestions.length" class="sug-bar mb-3">
                    <div v-for="s in suggestions" :key="s.id" class="sug-item">
                        <el-tag size="small" :type="sevType(s.severity)">{{ $t('compute.suggest.' + s.kind) }}</el-tag>
                        <span class="strong ml-2">{{ poolName(s.servedName) }}</span>
                        <span class="sub ml-2">{{ s.reason }}</span>
                        <span class="spacer" />
                        <el-button v-if="s.actionable" size="small" type="primary" link @click="onApply(s)">
                            {{ $t('compute.suggest.apply') }}
                        </el-button>
                        <el-button size="small" link @click="onDismiss(s)">
                            {{ $t('compute.suggest.dismiss') }}
                        </el-button>
                    </div>
                </div>
                <el-table :data="rows" row-key="id" :empty-text="$t('compute.pool.empty')">
                    <el-table-column :label="$t('commons.table.name')" min-width="200" fixed="left">
                        <template #default="{ row }">
                            <el-button link type="primary" @click="detailRef.open(row)">
                                {{ row.name || row.servedName }}
                            </el-button>
                            <div class="sub mono">{{ row.servedName }}</div>
                        </template>
                    </el-table-column>
                    <el-table-column :label="$t('compute.pool.replicas')" width="140">
                        <template #default="{ row }">
                            <span :class="row.healthy < Math.max(row.minInstances, 1) ? 'stat-danger' : 'stat-success'">
                                {{ row.healthy }}
                            </span>
                            / {{ row.instances }}
                            <div class="sub" v-if="row.mode !== 'off'">
                                {{ $t('compute.pool.range', [row.minInstances, row.maxInstances]) }}
                            </div>
                        </template>
                    </el-table-column>
                    <el-table-column :label="$t('compute.metric.load')" min-width="150">
                        <template #default="{ row }">
                            <template v-if="metric(row)">
                                <div>
                                    {{ $t('compute.metric.running') }} {{ num(metric(row)!.running, 0) }} ·
                                    <span :class="metric(row)!.waiting ? 'stat-warning' : ''">
                                        {{ $t('compute.metric.waiting') }} {{ num(metric(row)!.waiting, 0) }}
                                    </span>
                                </div>
                                <div class="sub">{{ num(metric(row)!.genTps) }} tok/s</div>
                            </template>
                            <span v-else>—</span>
                        </template>
                    </el-table-column>
                    <el-table-column :label="$t('compute.metric.ttft')" width="110">
                        <template #default="{ row }">
                            <span
                                :class="
                                    row.ttftHigh && (metric(row)?.ttftMs || 0) >= row.ttftHigh ? 'stat-warning' : ''
                                "
                            >
                                {{ ms(metric(row)?.ttftMs || 0) }}
                            </span>
                        </template>
                    </el-table-column>
                    <el-table-column :label="$t('compute.pool.plan')" min-width="170">
                        <template #default="{ row }">
                            <span v-if="row.profileName">{{ row.profileName }}</span>
                            <span v-else class="sub">—</span>
                            <div class="sub" v-if="row.nodeCount">
                                {{ $t('compute.pool.hostsPerReplica', [row.nodeCount]) }}
                            </div>
                        </template>
                    </el-table-column>
                    <el-table-column :label="$t('compute.policy.mode')" width="100">
                        <template #default="{ row }">
                            <el-tag size="small" :type="modeType(row.mode)">
                                {{ $t('compute.policy.mode_' + row.mode) }}
                            </el-tag>
                        </template>
                    </el-table-column>
                    <el-table-column :label="$t('compute.pool.service')" min-width="160">
                        <template #default="{ row }">
                            <el-tag
                                v-if="row.serviceMissing || !row.accountId"
                                size="small"
                                type="warning"
                                effect="plain"
                            >
                                {{ $t('compute.pool.noService') }}
                            </el-tag>
                            <el-button v-else link type="primary" @click="gotoService(row)">
                                {{ $t('compute.pool.openService') }}
                            </el-button>
                        </template>
                    </el-table-column>
                    <el-table-column :label="$t('commons.table.operate')" width="200" fixed="right">
                        <template #default="{ row }">
                            <el-button link type="primary" @click="detailRef.open(row)">
                                {{ $t('compute.pool.manage') }}
                            </el-button>
                            <el-button
                                v-if="row.serviceMissing || !row.accountId"
                                link
                                type="primary"
                                @click="onBind(row)"
                            >
                                {{ $t('compute.pool.bind') }}
                            </el-button>
                            <el-button link type="danger" @click="onDelete(row)">
                                {{ $t('commons.button.delete') }}
                            </el-button>
                        </template>
                    </el-table-column>
                </el-table>
            </template>
        </LayoutContent>

        <PoolCreate ref="createRef" @created="onCreated" />
        <PoolDetail ref="detailRef" @changed="refresh(true)" />
        <KeyDialog ref="keyRef" />
        <DrawerPro
            v-model="clusterVisible"
            :header="$t('compute.pool.cluster')"
            size="large"
            @close="clusterVisible = false"
        >
            <OverviewTab v-if="clusterVisible" ref="overviewRef" @goto="onGoto" />
        </DrawerPro>
        <DrawerPro
            v-model="accessVisible"
            :header="$t('compute.pool.routerSetting')"
            size="large"
            @close="onAccessClosed"
        >
            <AccessTab v-if="accessVisible" />
        </DrawerPro>
    </div>
</template>

<script lang="ts" setup>
import { computed, ref } from 'vue';
import { useRouter } from 'vue-router';
import { ElMessageBox } from 'element-plus';
import { Refresh } from '@element-plus/icons-vue';
import i18n from '@/lang';
import { Compute } from '@/api/interface/compute';
import {
    bindComputePool,
    deleteComputePool,
    getComputeOverview,
    getComputeSetting,
    listComputePools,
    listComputeSuggestions,
    operateComputeSuggestion,
    updateComputeSetting,
} from '@/api/modules/compute';
import { MsgSuccess } from '@/utils/message';
import { ms, num, t } from '../helper';
import { useComputePoll } from '../poll';
import PoolCreate from './create.vue';
import PoolDetail from './detail.vue';
import KeyDialog from './key.vue';
import OverviewTab from '../overview/index.vue';
import AccessTab from '../access/index.vue';

const router = useRouter();
const buttons = computed(() => [
    { label: i18n.global.t('aiTools.model.services'), path: '/ai/model-services' },
    { label: i18n.global.t('compute.pool.title'), path: '/ai/model-services/pools' },
    { label: i18n.global.t('compute.ana.title'), path: '/ai/model-services/analytics' },
]);

const loading = ref(false);
const pools = ref<Compute.Pool[]>([]);
const models = ref<Compute.ModelSummary[]>([]);
const suggestions = ref<Compute.Suggestion[]>([]);
const setting = ref<Compute.RouterSetting>();
const autoEnabled = ref(true);
const savingSwitch = ref(false);
const createRef = ref();
const detailRef = ref();
const keyRef = ref();
const overviewRef = ref();
const clusterVisible = ref(false);
const accessVisible = ref(false);
const onAccessClosed = () => {
    accessVisible.value = false;
    refresh(true);
};

// The page itself is the poll target; the open drawer refreshes too.
const self = {
    refresh: async (force = false) => {
        await refresh(force);
        if (detailRef.value?.visible) await detailRef.value.refresh();
        if (clusterVisible.value) await overviewRef.value?.refresh();
    },
};
const target = ref<any>(self);
const { paused, refreshedAt, tick } = useComputePoll(target);

const rows = computed(() => pools.value);
const metric = (row: Compute.Pool) => models.value.find((m) => m.servedName === row.servedName);
const poolName = (served: string) => pools.value.find((p) => p.servedName === served)?.name || served;
const modeType = (m: string) => (m === 'auto' ? 'success' : m === 'suggest' ? 'primary' : 'info');
const sevType = (s: string) => (s === 'critical' ? 'danger' : s === 'warning' ? 'warning' : 'info');

const refresh = async (force = false) => {
    if (force && !pools.value.length) loading.value = true;
    try {
        const [p, o, s, st] = await Promise.all([
            listComputePools(),
            getComputeOverview(),
            listComputeSuggestions(),
            getComputeSetting(),
        ]);
        pools.value = p.data || [];
        models.value = o.data?.models || [];
        suggestions.value = s.data || [];
        setting.value = st.data;
        autoEnabled.value = !st.data.schedulerPaused;
    } finally {
        loading.value = false;
    }
};

const onCreated = (res: Compute.PoolRes, row: Partial<Compute.Pool>) => {
    if (res?.key) keyRef.value.open(res.key, row);
    refresh(true);
};

const onBind = async (row: Compute.Pool) => {
    await ElMessageBox.confirm(t('compute.pool.bindConfirm', [row.name || row.servedName]), t('compute.pool.bind'), {
        type: 'info',
    });
    const res = await bindComputePool(row.id);
    keyRef.value.open(res.data.key, row);
    refresh(true);
};

const onDelete = async (row: Compute.Pool) => {
    let removeService = true;
    await ElMessageBox.confirm(
        t('compute.pool.deleteConfirm', [row.name || row.servedName]),
        t('commons.button.delete'),
        {
            type: 'warning',
            distinguishCancelAndClose: true,
            confirmButtonText: t('compute.pool.deleteWithService'),
            cancelButtonText: t('compute.pool.deleteKeepService'),
        },
    ).catch((action) => {
        if (action === 'cancel') {
            removeService = false;
            return;
        }
        throw action;
    });
    await deleteComputePool(row.id, removeService);
    MsgSuccess(t('commons.msg.deleteSuccess'));
    refresh(true);
};

const gotoService = (row: Compute.Pool) => {
    router.push({ path: '/ai/model-services', query: { name: row.name } });
};

const onApply = async (s: Compute.Suggestion) => {
    await ElMessageBox.confirm(
        `${t('compute.suggest.' + s.kind)} · ${poolName(s.servedName)}\n${s.reason}`,
        t('compute.suggest.applyConfirm'),
        { type: 'warning' },
    );
    const res = await operateComputeSuggestion(s.id, 'apply');
    MsgSuccess(t('compute.job.started', [res.data?.jobId || '']));
    refresh();
};

const onDismiss = async (s: Compute.Suggestion) => {
    await operateComputeSuggestion(s.id, 'dismiss');
    suggestions.value = suggestions.value.filter((x) => x.id !== s.id);
};

const onAutoSwitch = async (v: boolean) => {
    if (!setting.value) return;
    savingSwitch.value = true;
    try {
        await updateComputeSetting({ ...setting.value, schedulerPaused: !v });
        setting.value.schedulerPaused = !v;
        MsgSuccess(t('commons.msg.operationSuccess'));
    } catch {
        autoEnabled.value = !v;
    } finally {
        savingSwitch.value = false;
    }
};

// Links of the cluster overview point to where things live now.
const onGoto = (tab: string) => {
    clusterVisible.value = false;
    if (tab === 'hosts') router.push('/ai/gpu/scheduling');
    else if (tab === 'instances') router.push('/ai/vllm/cluster');
    else if (tab === 'access') accessVisible.value = true;
};
</script>

<style scoped src="../style.css"></style>
<style scoped>
.sync-hint {
    font-size: 12px;
    color: var(--el-text-color-secondary);
    margin-right: 12px;
}
.mr-2 {
    margin-right: 8px;
}
.sug-bar {
    border: 1px solid var(--el-color-warning-light-5);
    background: var(--el-color-warning-light-9);
    border-radius: 4px;
    padding: 4px 12px;
}
.sug-item {
    display: flex;
    align-items: center;
    padding: 4px 0;
}
</style>

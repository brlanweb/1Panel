<template>
    <div>
        <LayoutContent :title="$t('clusterResource.title')" v-loading="firstLoading">
            <template #leftToolBar>
                <el-input
                    v-model="keyword"
                    class="p-w-250"
                    clearable
                    :placeholder="$t('clusterResource.search')"
                    :prefix-icon="Search"
                />
                <el-checkbox v-model="onlyRisk" class="ml-3">{{ $t('clusterResource.onlyRisk') }}</el-checkbox>
            </template>
            <template #rightToolBar>
                <span class="sync-hint">
                    <el-tag v-if="paused" type="info" size="small">{{ $t('clusterResource.paused') }}</el-tag>
                    <span v-else-if="staticAt">{{ $t('clusterResource.lastSync') }} {{ staticAt }}</span>
                </span>
                <el-button :icon="Refresh" :loading="staticLoading" @click="loadStatic(true)">
                    {{ $t('commons.button.refresh') }}
                </el-button>
                <el-button v-if="isMaster" :icon="Bell" @click="openAlertRules">
                    {{ $t('xpack.alert.alertRulesEntry') }}
                </el-button>
            </template>
            <template #main>
                <!-- Summary: same stat-card structure as the cluster overview -->
                <div class="stat-cards mb-4">
                    <el-card v-for="stat in statCards" :key="stat.key" shadow="hover" class="stat-card">
                        <div class="stat-title">{{ stat.title }}</div>
                        <div class="stat-value" :class="stat.cls">{{ stat.value }}</div>
                        <div class="stat-note">{{ stat.note || '\u00a0' }}</div>
                    </el-card>
                </div>

                <el-alert
                    class="mb-4"
                    type="info"
                    :closable="false"
                    show-icon
                    :title="$t('clusterResource.realtimeHelper')"
                />

                <el-row :gutter="12">
                    <el-col :span="risks.length ? 18 : 24">
                        <el-card shadow="never">
                            <el-table :data="rows" row-key="nodeId" style="width: 100%">
                                <el-table-column :label="$t('clusterResource.name')" min-width="170" fixed="left">
                                    <template #default="{ row }">
                                        <div class="node-name">{{ row.displayName }}</div>
                                        <div class="node-sub" v-if="row.hostname || row.os">
                                            {{ row.hostname }}
                                            <span v-if="row.prettyDistro">· {{ row.prettyDistro }}</span>
                                        </div>
                                    </template>
                                </el-table-column>
                                <el-table-column :label="$t('clusterResource.status')" width="110">
                                    <template #default="{ row }">
                                        <el-tooltip
                                            :disabled="!row.error"
                                            :content="row.error"
                                            placement="top"
                                            :show-after="200"
                                        >
                                            <el-tag :type="row.online ? 'success' : 'danger'" size="small">
                                                {{
                                                    row.online
                                                        ? $t('clusterResource.online')
                                                        : $t('clusterResource.offline')
                                                }}
                                            </el-tag>
                                        </el-tooltip>
                                        <div class="node-sub" v-if="row.online && row.uptime">
                                            {{ formatUptime(row.uptime) }}
                                        </div>
                                    </template>
                                </el-table-column>
                                <el-table-column :label="$t('clusterResource.cpu')" min-width="150">
                                    <template #default="{ row }">
                                        <template v-if="row.online">
                                            <el-progress
                                                :percentage="round(row.cpuUsedPercent)"
                                                :color="barColor(row.cpuUsedPercent)"
                                                :stroke-width="10"
                                            />
                                            <div class="node-sub">
                                                {{ row.cpuTotal || '—' }} {{ $t('clusterResource.cores') }}
                                                <span v-if="row.load1">· load {{ row.load1.toFixed(2) }}</span>
                                            </div>
                                        </template>
                                        <span v-else>—</span>
                                    </template>
                                </el-table-column>
                                <el-table-column :label="$t('clusterResource.memory')" min-width="160">
                                    <template #default="{ row }">
                                        <template v-if="row.memoryTotal">
                                            <el-progress
                                                :percentage="round(row.memoryUsedPercent)"
                                                :color="barColor(row.memoryUsedPercent)"
                                                :stroke-width="10"
                                            />
                                            <div class="node-sub">
                                                {{ computeSize(row.memoryUsed) }} / {{ computeSize(row.memoryTotal) }}
                                            </div>
                                        </template>
                                        <span v-else>—</span>
                                    </template>
                                </el-table-column>
                                <el-table-column :label="$t('clusterResource.disk')" min-width="160">
                                    <template #default="{ row }">
                                        <template v-if="row.diskTotal">
                                            <el-popover placement="bottom" :width="360" trigger="hover">
                                                <template #reference>
                                                    <div>
                                                        <el-progress
                                                            :percentage="round(row.diskUsedPercent)"
                                                            :color="barColor(maxDiskPercent(row))"
                                                            :stroke-width="10"
                                                        />
                                                        <div class="node-sub">
                                                            {{ computeSize(row.diskUsed) }} /
                                                            {{ computeSize(row.diskTotal) }}
                                                        </div>
                                                    </div>
                                                </template>
                                                <div v-for="disk in row.disks" :key="disk.path" class="disk-line">
                                                    <div class="disk-line-head">
                                                        <span class="disk-path">{{ disk.path }}</span>
                                                        <span>
                                                            {{ computeSize(disk.used) }} / {{ computeSize(disk.total) }}
                                                        </span>
                                                    </div>
                                                    <el-progress
                                                        :percentage="round(disk.usedPercent)"
                                                        :color="barColor(disk.usedPercent)"
                                                        :stroke-width="6"
                                                    />
                                                </div>
                                            </el-popover>
                                        </template>
                                        <span v-else>—</span>
                                    </template>
                                </el-table-column>
                                <el-table-column :label="$t('clusterResource.netSpeed')" min-width="130">
                                    <template #default="{ row }">
                                        <template v-if="row.online">
                                            <div class="net-line">
                                                <el-icon class="net-up"><Top /></el-icon>
                                                {{ formatSpeed(row.upSpeed) }}
                                            </div>
                                            <div class="net-line">
                                                <el-icon class="net-down"><Bottom /></el-icon>
                                                {{ formatSpeed(row.downSpeed) }}
                                            </div>
                                        </template>
                                        <span v-else>—</span>
                                    </template>
                                </el-table-column>
                                <el-table-column :label="$t('clusterResource.traffic')" min-width="140">
                                    <template #default="{ row }">
                                        <template v-if="row.netBytesSent || row.netBytesRecv">
                                            <div class="node-sub-strong">
                                                {{ $t('clusterResource.sent') }} {{ computeSize(row.netBytesSent) }}
                                            </div>
                                            <div class="node-sub-strong">
                                                {{ $t('clusterResource.received') }}
                                                {{ computeSize(row.netBytesRecv) }}
                                            </div>
                                        </template>
                                        <span v-else>—</span>
                                    </template>
                                </el-table-column>
                                <el-table-column :label="$t('clusterResource.address')" min-width="170">
                                    <template #default="{ row }">
                                        <div v-if="row.publicIP" class="ip-public">
                                            <el-tag size="small" effect="plain" class="ip-tag">
                                                {{ $t('clusterResource.publicIP') }}
                                            </el-tag>
                                            {{ row.publicIP }}
                                        </div>
                                        <div v-for="ip in row.internalIPs" :key="ip" class="ip-internal">
                                            <span class="ip-label">{{ $t('clusterResource.internalIP') }}</span>
                                            {{ ip }}
                                        </div>
                                        <span v-if="!row.publicIP && !row.internalIPs?.length">—</span>
                                    </template>
                                </el-table-column>
                            </el-table>
                        </el-card>
                    </el-col>
                    <el-col :span="6" v-if="risks.length">
                        <el-card shadow="never" class="risk-card">
                            <template #header>
                                <div class="risk-header">
                                    <span>{{ $t('clusterResource.riskList') }}</span>
                                    <el-tag type="danger" size="small" v-if="summary.critical">
                                        {{ $t('clusterResource.levelCritical') }} {{ summary.critical }}
                                    </el-tag>
                                    <el-tag type="warning" size="small" v-if="summary.warning">
                                        {{ $t('clusterResource.levelWarning') }} {{ summary.warning }}
                                    </el-tag>
                                </div>
                            </template>
                            <div v-for="(risk, idx) in risks" :key="idx" class="risk-item">
                                <el-tag :type="risk.level === 'critical' ? 'danger' : 'warning'" size="small">
                                    {{
                                        risk.level === 'critical'
                                            ? $t('clusterResource.levelCritical')
                                            : $t('clusterResource.levelWarning')
                                    }}
                                </el-tag>
                                <div class="risk-body">
                                    <div class="risk-title">{{ nodeDisplayName(risk.nodeId, risk.name) }}</div>
                                    <div class="node-sub">
                                        {{ riskText(risk) }}
                                    </div>
                                </div>
                            </div>
                        </el-card>
                    </el-col>
                </el-row>
            </template>
        </LayoutContent>
    </div>
</template>

<script lang="ts" setup>
import { computed, onActivated, onDeactivated, onMounted, onUnmounted, ref } from 'vue';
import { Bell, Bottom, Refresh, Search, Top } from '@element-plus/icons-vue';
import { useRouter } from 'vue-router';
import i18n from '@/lang';
import { Cluster } from '@/api/interface/cluster';
import { loadClusterMonitorRealtime, loadClusterMonitorStatic } from '@/api/modules/cluster';
import { computeSize } from '@/utils/size';
import { useGlobalStore } from '@/composables/useGlobalStore';

const { globalStore, isMaster } = useGlobalStore();
const router = useRouter();
const openAlertRules = () => {
    // Open the alert notice page on its rule list, filtered to cluster rules.
    localStorage.setItem('alert-notice-tab', '0');
    router.push({ name: 'Alert', query: { type: 'clusterResource' } });
};

// Polling periods. Realtime data (CPU / network speed) is only fetched
// while this page is visible; memory / disks / IPs change slowly and are
// cached on the server for 30s.
const REALTIME_INTERVAL = 1000;
const STATIC_INTERVAL = 30000;

const firstLoading = ref(true);
const staticLoading = ref(false);
const staticData = ref<Cluster.MonitorStaticRes | null>(null);
const realtime = ref<Record<number, Cluster.MonitorRealtime>>({});
const staticAt = ref('');
const keyword = ref('');
const onlyRisk = ref(false);
const paused = ref(false);

let realtimeTimer: ReturnType<typeof setTimeout> | undefined;
let staticTimer: ReturnType<typeof setInterval> | undefined;
let active = false;
let realtimeInFlight = false;

const t = (key: string, args?: unknown[]) => i18n.global.t(key, args as any);

const nodeDisplayName = (nodeId: number, name: string) => {
    if (nodeId === 0) return globalStore.getMasterAlias() || t('clusterResource.local');
    return name;
};

const rows = computed(() => {
    const nodes = staticData.value?.nodes || [];
    const riskIds = new Set((staticData.value?.risks || []).map((r) => r.nodeId));
    const kw = keyword.value.trim().toLowerCase();
    return nodes
        .map((node) => {
            const rt = realtime.value[node.nodeId];
            const online = rt ? rt.online : node.online;
            return {
                ...node,
                displayName: nodeDisplayName(node.nodeId, node.name),
                online,
                error: (rt && !rt.online && rt.error) || node.error,
                cpuUsedPercent: rt?.cpuUsedPercent || 0,
                cpuTotal: rt?.cpuTotal || node.cpuTotal,
                load1: rt?.load1 || 0,
                upSpeed: rt?.upSpeed || 0,
                downSpeed: rt?.downSpeed || 0,
                netBytesSent: rt?.netBytesSent || 0,
                netBytesRecv: rt?.netBytesRecv || 0,
            };
        })
        .filter((row) => {
            if (onlyRisk.value && !riskIds.has(row.nodeId)) return false;
            if (!kw) return true;
            const hay = [row.displayName, row.hostname, row.publicIP, ...(row.internalIPs || [])]
                .join(' ')
                .toLowerCase();
            return hay.includes(kw);
        });
});

// CPU risks are re-evaluated with the live CPU sample so the list does not
// lag 30s behind the bars.
const risks = computed<Cluster.MonitorRisk[]>(() => {
    const data = staticData.value;
    if (!data) return [];
    const warn = data.warnPercent || 80;
    const crit = data.criticalPercent || 90;
    const list = (data.risks || []).filter((r) => r.kind !== 'cpu');
    for (const node of data.nodes) {
        const rt = realtime.value[node.nodeId];
        if (rt?.online && rt.cpuUsedPercent >= warn) {
            list.push({
                nodeId: node.nodeId,
                name: node.name,
                kind: 'cpu',
                level: rt.cpuUsedPercent >= crit ? 'critical' : 'warning',
                value: rt.cpuUsedPercent,
                target: '',
                message: '',
            });
        }
        if (rt && !rt.online && node.online && !list.some((r) => r.nodeId === node.nodeId && r.kind === 'offline')) {
            list.push({
                nodeId: node.nodeId,
                name: node.name,
                kind: 'offline',
                level: 'critical',
                value: 0,
                target: '',
                message: rt.error,
            });
        }
    }
    return list.sort((a, b) => (a.level === b.level ? 0 : a.level === 'critical' ? -1 : 1));
});

const summary = computed(() => {
    const total = staticData.value?.nodes.length || 0;
    let online = 0;
    for (const node of staticData.value?.nodes || []) {
        const rt = realtime.value[node.nodeId];
        if (rt ? rt.online : node.online) online++;
    }
    return {
        total,
        online,
        offline: total - online,
        warning: risks.value.filter((r) => r.level === 'warning').length,
        critical: risks.value.filter((r) => r.level === 'critical').length,
    };
});

const statCards = computed(() => [
    { key: 'total', title: t('clusterResource.totalNodes'), value: summary.value.total, cls: '', note: '' },
    { key: 'online', title: t('clusterResource.online'), value: summary.value.online, cls: 'stat-success', note: '' },
    {
        key: 'offline',
        title: t('clusterResource.offline'),
        value: summary.value.offline,
        cls: summary.value.offline ? 'stat-danger' : '',
        note: '',
    },
    {
        key: 'risk',
        title: t('clusterResource.warning'),
        value: summary.value.warning + summary.value.critical,
        cls: summary.value.critical ? 'stat-danger' : summary.value.warning ? 'stat-warning' : '',
        note:
            summary.value.critical || summary.value.warning
                ? `${t('clusterResource.levelCritical')} ${summary.value.critical} · ${t('clusterResource.levelWarning')} ${summary.value.warning}`
                : t('clusterResource.noRisk'),
    },
]);

const round = (value: number) => Math.min(100, Math.max(0, Number((value || 0).toFixed(1))));
const barColor = (value: number) => {
    const warn = staticData.value?.warnPercent || 80;
    const crit = staticData.value?.criticalPercent || 90;
    if (value >= crit) return 'var(--el-color-danger)';
    if (value >= warn) return 'var(--el-color-warning)';
    return 'var(--el-color-primary)';
};
const maxDiskPercent = (row: Cluster.MonitorStatic) =>
    Math.max(row.diskUsedPercent || 0, ...(row.disks || []).map((d) => d.usedPercent || 0));

const formatSpeed = (bytes: number) => `${computeSize(Math.round(bytes || 0))}/s`;
const formatUptime = (seconds: number) => {
    const days = Math.floor(seconds / 86400);
    const hours = Math.floor((seconds % 86400) / 3600);
    const minutes = Math.floor((seconds % 3600) / 60);
    if (days > 0) return `${t('clusterResource.uptime')} ${days}${t('clusterResource.days')} ${hours}h`;
    return `${t('clusterResource.uptime')} ${hours}h ${minutes}m`;
};

const riskText = (risk: Cluster.MonitorRisk) => {
    switch (risk.kind) {
        case 'offline':
            return risk.message
                ? `${t('clusterResource.kindOffline')}: ${risk.message}`
                : t('clusterResource.kindOffline');
        case 'cpu':
            return `${t('clusterResource.kindCpu')} ${risk.value.toFixed(1)}%`;
        case 'memory':
            return `${t('clusterResource.kindMemory')} ${risk.value.toFixed(1)}%`;
        case 'disk':
            return `${t('clusterResource.kindDisk')} ${risk.target} ${risk.value.toFixed(1)}%`;
    }
    return '';
};

const loadStatic = async (force = false) => {
    staticLoading.value = true;
    try {
        const res = await loadClusterMonitorStatic(force);
        staticData.value = res.data;
        staticAt.value = new Date().toLocaleTimeString();
    } catch {
        // keep the last snapshot on screen
    } finally {
        staticLoading.value = false;
        firstLoading.value = false;
    }
};

// Self-scheduling loop (setTimeout, not setInterval) so a slow round never
// overlaps the next one.
const realtimeTick = async () => {
    if (!active) return;
    if (document.hidden) {
        paused.value = true;
        scheduleRealtime();
        return;
    }
    paused.value = false;
    if (!realtimeInFlight) {
        realtimeInFlight = true;
        try {
            const res = await loadClusterMonitorRealtime();
            const map: Record<number, Cluster.MonitorRealtime> = {};
            for (const item of res.data?.nodes || []) {
                map[item.nodeId] = item;
            }
            realtime.value = map;
        } catch {
            // transient; next tick retries
        } finally {
            realtimeInFlight = false;
        }
    }
    scheduleRealtime();
};

const scheduleRealtime = () => {
    if (!active) return;
    clearTimeout(realtimeTimer);
    realtimeTimer = setTimeout(realtimeTick, REALTIME_INTERVAL);
};

const onVisibilityChange = () => {
    if (!document.hidden && active) {
        paused.value = false;
        clearTimeout(realtimeTimer);
        realtimeTick();
        loadStatic();
    }
};

const start = () => {
    if (active) return;
    active = true;
    loadStatic();
    realtimeTick();
    staticTimer = setInterval(() => {
        if (!document.hidden) loadStatic();
    }, STATIC_INTERVAL);
    document.addEventListener('visibilitychange', onVisibilityChange);
};

const stop = () => {
    active = false;
    clearTimeout(realtimeTimer);
    clearInterval(staticTimer);
    realtimeTimer = undefined;
    staticTimer = undefined;
    document.removeEventListener('visibilitychange', onVisibilityChange);
};

// The page may be kept alive by the tab cache: stop polling when it is
// deactivated, resume when it comes back.
onMounted(start);
onActivated(start);
onDeactivated(stop);
onUnmounted(stop);
</script>

<style scoped>
.mb-4 {
    margin-bottom: 16px;
}

.ml-3 {
    margin-left: 12px;
}

.stat-cards {
    display: grid;
    grid-template-columns: repeat(4, minmax(0, 1fr));
    gap: 12px;
    align-items: stretch;
}

.stat-card {
    height: 100%;
}

.stat-title {
    font-size: 13px;
    color: var(--el-text-color-secondary);
}

.stat-value {
    font-size: 26px;
    font-weight: 600;
    line-height: 1.4;
    margin-top: 6px;
}

.stat-note {
    font-size: 12px;
    line-height: 18px;
    min-height: 18px;
    color: var(--el-text-color-secondary);
}

.stat-success {
    color: var(--el-color-success);
}

.stat-warning {
    color: var(--el-color-warning);
}

.stat-danger {
    color: var(--el-color-danger);
}

.sync-hint {
    font-size: 12px;
    color: var(--el-text-color-secondary);
    margin-right: 12px;
}

.node-name {
    font-weight: 600;
}

.node-sub {
    font-size: 12px;
    color: var(--el-text-color-secondary);
    margin-top: 2px;
    line-height: 18px;
}

.node-sub-strong {
    font-size: 12px;
    line-height: 20px;
}

.net-line {
    display: flex;
    align-items: center;
    gap: 4px;
    font-size: 12px;
    line-height: 20px;
    font-variant-numeric: tabular-nums;
}

.net-up {
    color: var(--el-color-warning);
}

.net-down {
    color: var(--el-color-success);
}

.ip-public {
    font-size: 13px;
    line-height: 22px;
}

.ip-tag {
    margin-right: 4px;
}

/* Internal IPs are de-emphasised: smaller and lighter than the public IP. */
.ip-internal {
    font-size: 12px;
    line-height: 18px;
    color: var(--el-text-color-placeholder);
}

.ip-label {
    margin-right: 4px;
}

.disk-line {
    margin-bottom: 8px;
}

.disk-line-head {
    display: flex;
    justify-content: space-between;
    font-size: 12px;
    margin-bottom: 2px;
    gap: 8px;
}

.disk-path {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
}

.risk-card {
    position: sticky;
    top: 12px;
}

.risk-header {
    display: flex;
    align-items: center;
    gap: 8px;
}

.risk-item {
    display: flex;
    gap: 8px;
    align-items: flex-start;
    padding: 8px 0;
    border-bottom: 1px solid var(--el-border-color-lighter);
}

.risk-item:last-child {
    border-bottom: none;
}

.risk-body {
    min-width: 0;
}

.risk-title {
    font-weight: 600;
    font-size: 13px;
}
</style>

<template>
    <div class="cluster-nodes-card">
        <CardWithHeader :header="$t('home.clusterNodes')" class="card-interval" fill>
            <template #header-r>
                <span class="refresh-hint">{{ $t('home.clusterRefreshHint') }}</span>
                <el-tooltip :content="$t('menu.clusterResource')" placement="top">
                    <el-button class="h-button-setting" link icon="Monitor" @click="routerToName('ClusterResource')" />
                </el-tooltip>
            </template>
            <template #body>
                <el-scrollbar class="cluster-scrollbar">
                    <div v-if="!loaded" class="cluster-empty">{{ $t('commons.msg.loading') }}</div>
                    <div v-else-if="nodes.length === 0" class="cluster-empty">{{ $t('home.clusterNoNode') }}</div>
                    <div v-for="node in nodes" :key="node.nodeId" class="cluster-node">
                        <div class="cluster-node__head">
                            <span class="status-dot" :class="node.online ? 'is-online' : 'is-offline'" />
                            <span class="cluster-node__name" :title="node.name">{{ node.name }}</span>
                            <span class="cluster-node__ip">{{ node.ip }}</span>
                        </div>
                        <div v-if="node.online" class="cluster-node__rings">
                            <div v-for="m in node.metrics" :key="m.key" class="ring-item">
                                <el-progress
                                    type="circle"
                                    :width="46"
                                    :stroke-width="5"
                                    :percentage="m.value"
                                    :color="ringColor(m.value)"
                                >
                                    <span class="ring-value">{{ m.value }}%</span>
                                </el-progress>
                                <span class="ring-label">{{ m.label }}</span>
                            </div>
                        </div>
                        <div v-else class="cluster-node__offline">{{ node.error || $t('cluster.offline') }}</div>
                    </div>
                </el-scrollbar>
            </template>
        </CardWithHeader>
    </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue';
import CardWithHeader from '@/components/card-with-header/index.vue';
import { loadClusterMonitorRealtime, loadClusterMonitorStatic } from '@/api/modules/cluster';
import { Cluster } from '@/api/interface/cluster';
import { routerToName } from '@/utils/router';
import i18n from '@/lang';

// Loaded once on mount, then every 30 seconds while the home page is open.
const REFRESH_MS = 30000;

const staticNodes = ref<Cluster.MonitorStatic[]>([]);
const cpuMap = ref<Record<number, number>>({});
const loaded = ref(false);
let timer: ReturnType<typeof setInterval> | undefined;

const clamp = (v: number) => Math.max(0, Math.min(100, Math.round(Number(v) || 0)));

const nodes = computed(() =>
    staticNodes.value.map((node) => ({
        nodeId: node.nodeId,
        name: node.nodeId === 0 ? i18n.global.t('xpack.alert.clusterMasterNode') : node.name,
        ip: node.entryHost || node.publicIP || node.internalIPs?.[0] || '',
        online: node.online,
        error: node.error,
        metrics: [
            { key: 'cpu', label: 'CPU', value: clamp(cpuMap.value[node.nodeId]) },
            { key: 'mem', label: i18n.global.t('xpack.alert.clusterMemory'), value: clamp(node.memoryUsedPercent) },
            { key: 'disk', label: i18n.global.t('home.disk'), value: clamp(node.diskUsedPercent) },
        ],
    })),
);

const ringColor = (value: number) => {
    if (value >= 90) return '#f56c6c';
    if (value >= 75) return '#e6a23c';
    return 'var(--el-color-primary)';
};

const load = async () => {
    try {
        const [staticRes, realtimeRes] = await Promise.allSettled([
            loadClusterMonitorStatic(false),
            loadClusterMonitorRealtime(),
        ]);
        if (staticRes.status === 'fulfilled') {
            staticNodes.value = staticRes.value.data?.nodes || [];
        }
        if (realtimeRes.status === 'fulfilled') {
            const map: Record<number, number> = {};
            for (const item of realtimeRes.value.data?.nodes || []) {
                map[item.nodeId] = item.cpuUsedPercent;
            }
            cpuMap.value = map;
        }
    } finally {
        loaded.value = true;
    }
};

onMounted(() => {
    load();
    timer = setInterval(load, REFRESH_MS);
});

onBeforeUnmount(() => {
    if (timer) clearInterval(timer);
});
</script>

<style scoped lang="scss">
.cluster-nodes-card {
    min-height: 0;
}

.cluster-scrollbar {
    height: 536px;
}

@media only screen and (min-width: 992px) {
    .cluster-nodes-card {
        display: flex;
        flex-direction: column;
    }
    .cluster-scrollbar {
        height: 100%;
        min-height: 0;
    }
}

.refresh-hint {
    font-size: 12px;
    color: var(--el-text-color-secondary);
}

.cluster-empty {
    padding: 40px 0;
    text-align: center;
    color: var(--el-text-color-secondary);
    font-size: 13px;
}

.cluster-node {
    padding: 10px 12px;
    margin: 0 4px 6px 0;
    border: 1px solid var(--el-border-color-lighter);
    border-radius: 10px;
    transition: border-color 0.18s ease;

    &:hover {
        border-color: var(--el-color-primary-light-5);
    }

    &__head {
        display: flex;
        align-items: center;
        gap: 8px;
        min-width: 0;
    }

    &__name {
        font-weight: 600;
        font-size: 14px;
        color: var(--panel-text-color);
        overflow: hidden;
        text-overflow: ellipsis;
        white-space: nowrap;
    }

    &__ip {
        margin-left: auto;
        font-size: 12px;
        color: var(--el-text-color-secondary);
        white-space: nowrap;
    }

    &__rings {
        display: flex;
        justify-content: space-around;
        margin-top: 8px;
    }

    &__offline {
        margin-top: 8px;
        font-size: 12px;
        color: var(--el-color-danger);
        overflow: hidden;
        text-overflow: ellipsis;
        white-space: nowrap;
    }
}

.status-dot {
    flex: 0 0 auto;
    width: 8px;
    height: 8px;
    border-radius: 50%;

    &.is-online {
        background: var(--el-color-success);
        box-shadow: 0 0 0 3px var(--el-color-success-light-8);
    }

    &.is-offline {
        background: var(--el-color-danger);
        box-shadow: 0 0 0 3px var(--el-color-danger-light-8);
    }
}

.ring-item {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 2px;
}

.ring-value {
    font-size: 11px;
    color: var(--panel-text-color);
}

.ring-label {
    font-size: 12px;
    color: var(--el-text-color-secondary);
}
</style>

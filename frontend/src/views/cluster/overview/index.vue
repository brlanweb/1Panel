<template>
    <div>
        <LayoutContent :title="$t('menu.clusterOverview')" v-loading="loading">
            <template #leftToolBar>
                <el-select class="p-w-300" v-model="scopeNodeId" @change="onScopeChange" clearable>
                    <template #prefix>{{ $t('cluster.viewScope') }}</template>
                    <el-option :label="$t('cluster.allNodes')" :value="0"></el-option>
                    <el-option
                        v-for="item in aggregate.nodes"
                        :key="item.nodeId"
                        :label="item.name"
                        :value="item.nodeId"
                    />
                </el-select>
                <el-text type="info" size="small">{{ $t('cluster.scopeHint') }}</el-text>
            </template>
            <template #rightToolBar>
                <TableRefresh @search="loadAggregate()" />
            </template>
            <template #main>
                <el-row :gutter="12" class="mb-4">
                    <el-col :span="6">
                        <el-card shadow="hover">
                            <el-statistic :title="$t('cluster.totalNodes')" :value="summary.total" />
                        </el-card>
                    </el-col>
                    <el-col :span="6">
                        <el-card shadow="hover">
                            <el-statistic :title="$t('cluster.onlineNodes')" :value="summary.online" />
                        </el-card>
                    </el-col>
                    <el-col :span="6">
                        <el-card shadow="hover">
                            <el-statistic :title="$t('cluster.totalApps')" :value="summary.totalApps" />
                        </el-card>
                    </el-col>
                    <el-col :span="6">
                        <el-card shadow="hover">
                            <el-statistic :title="$t('cluster.totalWebsites')" :value="summary.totalWebsites" />
                        </el-card>
                    </el-col>
                </el-row>
                <el-row :gutter="12">
                    <el-col :span="12" v-for="node in visibleNodes" :key="node.nodeId" class="mb-4">
                        <el-card shadow="hover">
                            <template #header>
                                <div class="node-card-header">
                                    <span class="node-card-name">{{ node.name }}</span>
                                    <el-tag :type="statusTagType(node.status)" size="small">
                                        {{ statusLabel(node.status) }}
                                    </el-tag>
                                </div>
                            </template>
                            <div class="node-card-body">
                                <div class="node-card-url">
                                    <el-link type="primary" :underline="false" @click="openUrl(nodeEntryUrl(node))">
                                        {{ nodeEntryUrl(node) }}
                                    </el-link>
                                </div>
                                <div class="node-card-metric">
                                    <span>{{ $t('cluster.cpu') }} {{ node.cpuUsedPercent.toFixed(1) }}%</span>
                                    <el-progress
                                        :percentage="node.cpuUsedPercent"
                                        :show-text="false"
                                        :stroke-width="8"
                                    />
                                </div>
                                <div class="node-card-metric">
                                    <span>{{ $t('cluster.memory') }} {{ node.memUsedPercent.toFixed(1) }}%</span>
                                    <el-progress
                                        :percentage="node.memUsedPercent"
                                        :show-text="false"
                                        :stroke-width="8"
                                    />
                                </div>
                                <div class="node-card-counts">
                                    <span>
                                        {{ $t('cluster.appCount') }}: {{ node.appCount }} &nbsp;·&nbsp;
                                        {{ $t('cluster.websiteCount') }}: {{ node.websiteCount }}
                                    </span>
                                </div>
                                <el-text v-if="node.fetchError" type="danger" size="small">
                                    {{ $t('cluster.fetchError') }}: {{ node.fetchError }}
                                </el-text>
                                <div class="node-card-actions">
                                    <el-button size="small" @click="openUrl(nodeOrigin(node) + '/terminal')">
                                        {{ $t('cluster.openTerminal') }}
                                    </el-button>
                                    <el-button size="small" @click="openUrl(nodeOrigin(node) + '/logs')">
                                        {{ $t('cluster.openLogs') }}
                                    </el-button>
                                    <el-button size="small" type="primary" @click="openUrl(nodeEntryUrl(node))">
                                        {{ $t('cluster.openPanel') }}
                                    </el-button>
                                </div>
                            </div>
                        </el-card>
                    </el-col>
                </el-row>
                <el-empty v-if="!visibleNodes.length" :description="$t('commons.msg.noneData')" />
            </template>
        </LayoutContent>
    </div>
</template>

<script lang="ts" setup>
import { computed, onMounted, reactive, ref } from 'vue';
import i18n from '@/lang';
import { getClusterAggregateOverview, searchClusterNodes } from '@/api/modules/cluster';
import { Cluster } from '@/api/interface/cluster';

const loading = ref(false);
const scopeNodeId = ref(0);
const aggregate = reactive<Cluster.AggregateOverview>({
    total: 0,
    online: 0,
    offline: 0,
    unknown: 0,
    totalApps: 0,
    totalWebsites: 0,
    nodes: [],
});
const entryUrls = ref<Record<number, string>>({});

const visibleNodes = computed(() => {
    if (!scopeNodeId.value) {
        return aggregate.nodes;
    }
    return aggregate.nodes.filter((node) => node.nodeId === scopeNodeId.value);
});

const summary = computed(() => {
    const nodes = visibleNodes.value;
    return {
        total: scopeNodeId.value ? nodes.length : aggregate.total,
        online: nodes.filter((node) => node.status === 'online').length,
        totalApps: nodes.reduce((sum, node) => sum + node.appCount, 0),
        totalWebsites: nodes.reduce((sum, node) => sum + node.websiteCount, 0),
    };
});

const nodeEntryUrl = (node: Cluster.NodeSnapshot) => {
    return (entryUrls.value[node.nodeId] || '').replace(/\/$/, '');
};

// Routes live at the panel origin (/terminal, /logs); the entry URL's
// trailing security-entrance segment is a single-segment gate, not a path
// prefix, so appending a route to it would 404.
const nodeOrigin = (node: Cluster.NodeSnapshot) => {
    const entryUrl = nodeEntryUrl(node);
    try {
        return new URL(entryUrl).origin;
    } catch {
        return entryUrl;
    }
};

const openUrl = (url: string) => {
    if (url) {
        window.open(url, '_blank');
    }
};

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

const onScopeChange = () => {
    // Totals and cards are computed from the selected scope; no refetch needed.
};

const loadAggregate = async () => {
    loading.value = true;
    await getClusterAggregateOverview()
        .then((res) => {
            aggregate.total = res.data.total;
            aggregate.online = res.data.online;
            aggregate.offline = res.data.offline;
            aggregate.unknown = res.data.unknown;
            aggregate.totalApps = res.data.totalApps;
            aggregate.totalWebsites = res.data.totalWebsites;
            aggregate.nodes = res.data.nodes || [];
        })
        .finally(() => {
            loading.value = false;
        });
};

const loadEntryUrls = async () => {
    // The aggregate snapshot has no entry URL; fetch the registry once for jump links.
    try {
        const res = await searchClusterNodes({ page: 1, pageSize: 500 });
        const map: Record<number, string> = {};
        for (const item of res.data.items) {
            map[item.id] = item.entryUrl;
        }
        entryUrls.value = map;
    } catch {
        // The request interceptor already reports the error; cards still render.
    }
};

onMounted(() => {
    loadAggregate();
    loadEntryUrls();
});
</script>

<style scoped>
.mb-4 {
    margin-bottom: 16px;
}

.node-card-header {
    display: flex;
    justify-content: space-between;
    align-items: center;
}

.node-card-name {
    font-weight: 600;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
}

.node-card-body {
    display: flex;
    flex-direction: column;
    gap: 10px;
}

.node-card-url {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
}

.node-card-metric {
    display: flex;
    flex-direction: column;
    gap: 4px;
}

.node-card-actions {
    display: flex;
    gap: 8px;
    margin-top: 4px;
}
</style>

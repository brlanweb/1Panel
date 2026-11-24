<template>
    <div class="top-node-switcher" v-if="showSwitcher">
        <el-popover
            v-model:visible="popoverVisible"
            placement="bottom-end"
            :show-arrow="false"
            :offset="6"
            :width="260"
            trigger="click"
            @before-enter="loadNodes"
            popper-class="top-node-switcher-popover"
        >
            <template #reference>
                <div class="switcher-trigger" :class="{ switching: switchingNode }">
                    <span class="status-dot" :class="currentStatusClass" />
                    <span class="trigger-name">{{ currentDisplayName }}</span>
                    <el-tag
                        v-if="currentNodeMode && currentNode !== 'local'"
                        size="small"
                        effect="plain"
                        class="mode-tag"
                    >
                        {{ modeLabel(currentNodeMode) }}
                    </el-tag>
                    <el-icon class="trigger-arrow"><ArrowDown /></el-icon>
                </div>
            </template>

            <div class="switcher-panel" v-loading="loading || switchingNode">
                <div class="switcher-search" v-if="nodeOptions.length > 6">
                    <el-input
                        v-model="keyword"
                        size="small"
                        clearable
                        :placeholder="$t('cluster.searchNodes')"
                        prefix-icon="Search"
                    />
                </div>
                <el-scrollbar max-height="300px" :noresize="true">
                    <div
                        v-for="item in filteredNodes"
                        :key="item.name"
                        class="switcher-item"
                        :class="{ active: item.name === currentNode }"
                        @click="changeNode(item)"
                    >
                        <span class="status-dot" :class="nodeDotClass(item)" />
                        <div class="item-info">
                            <div class="item-title">
                                <span class="item-name">{{ displayNodeName(item) }}</span>
                                <el-tag
                                    v-if="item.name !== 'local' && item.mode"
                                    size="small"
                                    effect="plain"
                                    class="mode-tag"
                                >
                                    {{ modeLabel(item.mode) }}
                                </el-tag>
                            </div>
                            <div class="item-addr" v-if="item.addr">{{ item.addr }}</div>
                        </div>
                        <el-icon v-if="item.name === currentNode" class="active-check"><Check /></el-icon>
                    </div>
                </el-scrollbar>
                <el-divider class="switcher-divider" />
                <div class="switcher-footer">
                    <el-button link size="small" type="primary" @click="goClusterOverview">
                        {{ isXpackOrEE ? $t('xpack.node.multiOverview') : $t('menu.clusterOverview') }}
                    </el-button>
                    <el-button v-if="!isXpackOrEE" link size="small" @click="goClusterNodes">
                        {{ $t('cluster.manageNodes') }}
                    </el-button>
                </div>
            </div>
        </el-popover>
    </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue';
import { ArrowDown, Check } from '@element-plus/icons-vue';
import i18n from '@/lang';
import { MsgError } from '@/utils/message';
import { getAgentSettingInfo } from '@/api/modules/setting';
import { listClusterNodeOptions, testClusterNode } from '@/api/modules/cluster';
import { changeToLocal, listNodes, setDefaultNodeInfo } from '@/utils/node';
import { loadProductProFromDB } from '@/utils/xpack';
import { routerToNameWithQuery } from '@/utils/router';
import { syncAuthInfo } from '@/utils/rbac';
import { useGlobalStore } from '@/composables/useGlobalStore';
import bus from '@/global/bus';

const { globalStore, currentNode, currentNodeAddr, defaultNetwork, isAdmin, isEnterprise, isXpackOrEE } =
    useGlobalStore();

const popoverVisible = ref(false);
const loading = ref(false);
const switchingNode = ref(false);
const keyword = ref('');
const nodeOptions = ref<any[]>([]);

const showSwitcher = computed(() => {
    return isXpackOrEE.value || nodeOptions.value.length > 0;
});

const currentDisplayName = computed(() => {
    if (!currentNode.value || currentNode.value === 'local') {
        return globalStore.getMasterAlias();
    }
    return currentNode.value;
});

const currentOption = computed(() => nodeOptions.value.find((item) => item.name === currentNode.value));

const currentNodeMode = computed(() => currentOption.value?.mode || '');

const currentStatusClass = computed(() => {
    if (!currentNode.value || currentNode.value === 'local') return 'online';
    return nodeDotClass(currentOption.value);
});

const filteredNodes = computed(() => {
    const q = keyword.value.trim().toLowerCase();
    if (!q) return nodeOptions.value;
    return nodeOptions.value.filter(
        (item) => displayNodeName(item).toLowerCase().includes(q) || (item.addr || '').toLowerCase().includes(q),
    );
});

const displayNodeName = (item: any) => {
    return item.name === 'local' ? globalStore.getMasterAlias() : item.name;
};

const nodeDotClass = (item: any) => {
    if (!item || item.name === 'local' || item.status === 'Healthy') return 'online';
    if (item.status === 'Unhealthy') return 'offline';
    return 'unknown';
};

const modeLabel = (mode: string) => {
    if (mode === 'agent') return i18n.global.t('cluster.modeAgent');
    if (mode === 'agent-tunnel') return i18n.global.t('cluster.modeTunnel');
    return i18n.global.t('cluster.modePanel');
};

const loadNodes = async () => {
    loading.value = true;
    try {
        if (isXpackOrEE.value) {
            const res = await listNodes('all');
            nodeOptions.value = res || [];
            if (nodeOptions.value.length === 0) {
                setDefaultNodeInfo();
            }
            return;
        }
        const res = await listClusterNodeOptions();
        const clusterNodes = (res.data || []).map((item: any) => ({ ...item, source: 'cluster' }));
        nodeOptions.value = [
            {
                id: 0,
                name: 'local',
                addr: '127.0.0.1',
                status: 'Healthy',
                isBound: true,
                mode: 'panel',
                source: 'local',
            },
            ...clusterNodes,
        ];
        if (currentNode.value !== 'local' && !nodeOptions.value.some((item) => item.name === currentNode.value)) {
            changeToLocal();
        }
    } catch {
        setDefaultNodeInfo();
    } finally {
        loading.value = false;
    }
};

const loadGlobalSetting = async (nodeName?: string, skipAuthRedirect = false) => {
    await getAgentSettingInfo(nodeName, skipAuthRedirect ? { skipAuthRedirect: true } : undefined).then((res) => {
        defaultNetwork.value = res.data.defaultNetwork;
    });
};

const changeNode = async (item: any) => {
    if (!item || currentNode.value === item.name || switchingNode.value) {
        popoverVisible.value = false;
        return;
    }
    switchingNode.value = true;
    try {
        if (item.source === 'cluster') {
            if (!isAdmin.value) {
                MsgError(i18n.global.t('cluster.switchAdminOnly'));
                return;
            }
            if (item.status !== 'Healthy') {
                try {
                    const res = await testClusterNode(item.id);
                    if (res?.data?.status !== 'online') {
                        MsgError(res?.data?.lastError || i18n.global.t('cluster.switchTestFailed'));
                        return;
                    }
                    item.status = 'Healthy';
                } catch {
                    MsgError(i18n.global.t('cluster.switchTestFailed'));
                    return;
                }
            }
            try {
                await loadGlobalSetting(item.name, true);
            } catch (e: any) {
                MsgError(e?.response?.data?.message || e?.message || i18n.global.t('cluster.switchTestFailed'));
                return;
            }
            localStorage.removeItem('dashboardCache');
            localStorage.removeItem('upgradeChecked');
            currentNode.value = item.name;
            currentNodeAddr.value = item.addr;
            popoverVisible.value = false;
            loadProductProFromDB();
            routerToNameWithQuery('home', { t: Date.now() });
            return;
        }

        if (item.name === 'local') {
            if (isEnterprise.value) {
                await syncAuthInfo('local');
            }
            await loadGlobalSetting('local');
            if (!isEnterprise.value && !item.isXpack) {
                globalStore.isProductPro = false;
                globalStore.productProExpires = 0;
            }
            currentNode.value = 'local';
            currentNodeAddr.value = item.addr;
            localStorage.removeItem('dashboardCache');
            localStorage.removeItem('upgradeChecked');
            popoverVisible.value = false;
            loadProductProFromDB();
            routerToNameWithQuery('home', { t: Date.now() });
            return;
        }

        if (!item.isBound) {
            MsgError(i18n.global.t('xpack.node.nodeUnbindHelper'));
            return;
        }
        if (item.status !== 'Healthy') {
            MsgError(i18n.global.t('xpack.node.nodeUnhealthyHelper'));
            return;
        }
        await loadGlobalSetting(item.name);
        localStorage.removeItem('dashboardCache');
        localStorage.removeItem('upgradeChecked');
        currentNode.value = item.name;
        currentNodeAddr.value = item.addr;
        if (isEnterprise.value) {
            await syncAuthInfo(item.name);
        }
        popoverVisible.value = false;
        loadProductProFromDB();
        routerToNameWithQuery('home', { t: Date.now() });
    } finally {
        switchingNode.value = false;
    }
};

const goClusterOverview = () => {
    popoverVisible.value = false;
    if (isXpackOrEE.value) {
        routerToNameWithQuery('NodeDashboard', { uncached: 'true' });
        return;
    }
    routerToNameWithQuery('ClusterOverview', { uncached: 'true' });
};

const goClusterNodes = () => {
    popoverVisible.value = false;
    routerToNameWithQuery('ClusterNodes', { uncached: 'true' });
};

bus.on('refreshClusterNodes', () => {
    loadNodes();
});

onMounted(() => {
    loadNodes();
});
</script>

<style scoped lang="scss">
.top-node-switcher {
    display: inline-flex;
    align-items: center;
    flex-shrink: 0;
}

.switcher-trigger {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    height: 30px;
    padding: 0 12px;
    border-radius: 15px;
    border: 1px solid var(--el-border-color);
    background-color: var(--el-bg-color);
    color: var(--el-text-color-primary);
    font-size: 13px;
    cursor: pointer;
    transition:
        border-color 0.2s,
        color 0.2s;
    &:hover {
        border-color: var(--el-color-primary);
        color: var(--el-color-primary);
    }
}

.trigger-name {
    max-width: 140px;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    font-weight: 500;
}

.trigger-arrow {
    font-size: 12px;
    color: var(--el-text-color-secondary);
}

.status-dot {
    width: 7px;
    height: 7px;
    border-radius: 50%;
    flex-shrink: 0;
    &.online {
        background-color: var(--el-color-success);
    }
    &.offline {
        background-color: var(--el-color-danger);
    }
    &.unknown {
        background-color: var(--el-color-info);
    }
}

.mode-tag {
    height: 18px;
    line-height: 16px;
    padding: 0 5px;
    font-size: 11px;
}

.switcher-search {
    padding: 4px 4px 8px;
}

.switcher-item {
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 7px 10px;
    border-radius: 6px;
    cursor: pointer;
    transition: background-color 0.15s;
    &:hover {
        background-color: var(--el-fill-color-light);
    }
    &.active {
        background-color: var(--el-color-primary-light-9);
        color: var(--el-color-primary);
    }
}

.item-info {
    flex: 1;
    min-width: 0;
}

.item-title {
    display: flex;
    align-items: center;
    gap: 6px;
}

.item-name {
    font-size: 13px;
    font-weight: 500;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
}

.item-addr {
    font-size: 11px;
    color: var(--el-text-color-secondary);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
}

.active-check {
    font-size: 14px;
    color: var(--el-color-primary);
}

.switcher-divider {
    margin: 6px 0;
}

.switcher-footer {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 0 4px;
}
</style>

<template>
    <div>
        <LayoutContent :title="$t('compute.title')">
            <template #leftToolBar>
                <el-radio-group v-model="tab" class="compute-tabs">
                    <el-radio-button v-for="item in tabs" :key="item" :value="item">
                        {{ $t('compute.tab.' + item) }}
                        <el-badge
                            v-if="item === 'overview' && suggestionCount"
                            :value="suggestionCount"
                            type="warning"
                            class="tab-badge"
                        />
                    </el-radio-button>
                </el-radio-group>
            </template>
            <template #rightToolBar>
                <span class="sync-hint">
                    <el-tag v-if="paused" type="info" size="small">{{ $t('compute.paused') }}</el-tag>
                    <span v-else-if="refreshedAt">{{ $t('compute.refreshedAt') }} {{ refreshedAt }}</span>
                </span>
                <el-button :icon="Refresh" @click="tick(true)">{{ $t('commons.button.refresh') }}</el-button>
            </template>
            <template #main>
                <OverviewTab v-if="tab === 'overview'" ref="currentRef" @goto="(v: string) => (tab = v)" />
                <HostsTab v-else-if="tab === 'hosts'" ref="currentRef" />
                <InstancesTab v-else-if="tab === 'instances'" ref="currentRef" />
                <TemplatesTab v-else-if="tab === 'templates'" ref="currentRef" />
                <PolicyTab v-else-if="tab === 'policy'" ref="currentRef" />
                <AccessTab v-else-if="tab === 'access'" ref="currentRef" />
                <JobsTab v-else-if="tab === 'jobs'" ref="currentRef" />
            </template>
        </LayoutContent>
    </div>
</template>

<script lang="ts" setup>
import { nextTick, onActivated, onDeactivated, onMounted, onUnmounted, ref, watch } from 'vue';
import { Refresh } from '@element-plus/icons-vue';
import OverviewTab from './overview/index.vue';
import HostsTab from './hosts/index.vue';
import InstancesTab from './instances/index.vue';
import TemplatesTab from './templates/index.vue';
import PolicyTab from './policy/index.vue';
import AccessTab from './access/index.vue';
import JobsTab from './jobs/index.vue';
import { computeWatch, listComputeSuggestions } from '@/api/modules/compute';

// Polling model (resource friendly):
// - only the visible tab is refreshed, every POLL ms (tabs that are pure
//   configuration expose no `refresh` and are never polled);
// - a heartbeat tells the backend somebody watches, which switches the
//   collector to the 5s cadence for 30s; without it the fleet is sampled
//   every 15s (60s for hosts without instances);
// - everything stops while the browser tab is hidden or the page is
//   deactivated by the keep-alive cache.
const POLL = 5000;
const HEARTBEAT = 20000;

const tabs = ['overview', 'hosts', 'instances', 'templates', 'policy', 'access', 'jobs'];
const tab = ref(localStorage.getItem('compute-tab') || 'overview');
const currentRef = ref<any>();
const paused = ref(false);
const refreshedAt = ref('');
const suggestionCount = ref(0);

let timer: ReturnType<typeof setTimeout> | undefined;
let lastBeat = 0;
let active = false;
let inFlight = false;

const tick = async (force = false) => {
    if (!active) return;
    if (document.hidden) {
        paused.value = true;
        schedule();
        return;
    }
    paused.value = false;
    const now = Date.now();
    if (now - lastBeat > HEARTBEAT) {
        lastBeat = now;
        computeWatch(true).catch(() => undefined);
        listComputeSuggestions()
            .then((res) => (suggestionCount.value = (res.data || []).length))
            .catch(() => undefined);
    }
    const cur = currentRef.value;
    if (cur?.refresh && !inFlight) {
        inFlight = true;
        try {
            await cur.refresh(force);
            refreshedAt.value = new Date().toLocaleTimeString();
        } catch {
            // keep the last data on screen
        } finally {
            inFlight = false;
        }
    }
    schedule();
};

const schedule = () => {
    if (!active) return;
    clearTimeout(timer);
    timer = setTimeout(() => tick(false), POLL);
};

const onVisibility = () => {
    if (!document.hidden && active) {
        lastBeat = 0;
        clearTimeout(timer);
        tick(false);
    }
};

const start = () => {
    if (active) return;
    active = true;
    lastBeat = 0;
    document.addEventListener('visibilitychange', onVisibility);
    nextTick(() => tick(true));
};

const stop = () => {
    if (!active) return;
    active = false;
    clearTimeout(timer);
    document.removeEventListener('visibilitychange', onVisibility);
    computeWatch(false).catch(() => undefined);
};

watch(tab, (v) => {
    localStorage.setItem('compute-tab', v);
    refreshedAt.value = '';
    nextTick(() => {
        clearTimeout(timer);
        tick(true);
    });
});

onMounted(start);
onActivated(start);
onDeactivated(stop);
onUnmounted(stop);
</script>

<style scoped>
.sync-hint {
    font-size: 12px;
    color: var(--el-text-color-secondary);
    margin-right: 12px;
}
.tab-badge {
    margin-left: 4px;
    vertical-align: top;
}
.compute-tabs :deep(.el-badge__content) {
    transform: translateY(-4px);
}
</style>

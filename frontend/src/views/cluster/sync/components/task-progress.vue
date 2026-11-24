<template>
    <DrawerPro
        v-model="visible"
        :header="$t('cluster.taskDetail')"
        :resource="taskId ? '#' + taskId : ''"
        size="large"
        @close="onClose"
    >
        <div v-if="task" class="task-summary">
            <el-tag :type="taskTagType(task.status)">
                <span class="flx-align-center">
                    {{ taskStatusLabel(task.status) }}
                    <el-icon v-if="task.status === 'running'" class="is-loading ml-1"><Loading /></el-icon>
                </span>
            </el-tag>
            <span>{{ $t('cluster.sourceNode') }}：{{ task.sourceName }}</span>
            <span>{{ $t('cluster.succeeded') }}：{{ task.succeeded }}</span>
            <span>{{ $t('cluster.skipped') }}：{{ task.skipped }}</span>
            <span>{{ $t('cluster.failed') }}：{{ task.failed }}</span>
            <el-button
                v-if="task.status === 'running' && cancellable"
                class="ml-auto"
                type="danger"
                plain
                size="small"
                :loading="cancelling"
                @click="onCancel"
            >
                {{ $t('cluster.cancelTask') }}
            </el-button>
        </div>
        <div v-if="task && task.status === 'running'" class="progress-box mt-3">
            <div class="flx-align-center">
                <el-icon class="is-loading mr-1"><Loading /></el-icon>
                <span class="font-medium">{{ task.step || $t('cluster.progressWaiting') }}</span>
            </div>
            <template v-if="hasBytes">
                <el-progress
                    class="mt-2"
                    :percentage="percent"
                    :stroke-width="16"
                    text-inside
                    :status="stalledSeconds >= 60 ? 'warning' : undefined"
                />
                <div class="progress-meta mt-1">
                    <span>{{ computeSize(task.progressDone || 0) }} / {{ computeSize(task.progressTotal || 0) }}</span>
                    <span>{{ $t('cluster.progressSpeed') }}：{{ speedText }}</span>
                    <span>{{ $t('cluster.progressEta') }}：{{ etaText }}</span>
                </div>
            </template>
            <div v-else class="progress-meta mt-1">{{ $t('cluster.progressWaiting') }}</div>
            <div class="progress-meta mt-1">
                <span>{{ $t('cluster.progressUpdated') }}：{{ updatedText }}</span>
            </div>
            <el-alert
                v-if="hasBytes && stalledSeconds >= 60"
                class="mt-2"
                type="warning"
                :closable="false"
                show-icon
                :title="$t('cluster.progressStalled', [stalledSeconds])"
            />
        </div>
        <el-table :data="records" class="mt-4" v-loading="loading && !records.length">
            <el-table-column :label="$t('cluster.resourceName')" min-width="180">
                <template #default="{ row }">{{ row.resourceName || row.resourceKey }}</template>
            </el-table-column>
            <el-table-column :label="$t('xpack.sync.targetTitle')" prop="targetName" min-width="120" />
            <el-table-column :label="$t('cluster.action')" width="130" align="center">
                <template #default="{ row }">
                    <el-tag :type="actionTagType(row.action)" size="small">{{ actionLabel(row.action) }}</el-tag>
                </template>
            </el-table-column>
            <el-table-column :label="$t('cluster.reason')" prop="reason" min-width="200" show-overflow-tooltip />
            <el-table-column :label="$t('cluster.duration')" width="100" align="right">
                <template #default="{ row }">{{ formatDuration(row.durationMs) }}</template>
            </el-table-column>
        </el-table>
        <el-empty v-if="!records.length && !loading" :description="runningHint" />
    </DrawerPro>
</template>

<script lang="ts" setup>
import { computed, onBeforeUnmount, ref } from 'vue';
import { Loading } from '@element-plus/icons-vue';
import i18n from '@/lang';
import { cancelClusterSyncTask, listClusterSyncTaskRecords, searchClusterSyncTasks } from '@/api/modules/cluster';
import { Cluster } from '@/api/interface/cluster';
import { computeSize } from '@/utils/size';
import { dateFormatSimple } from '@/utils/date';
import { ElMessageBox } from 'element-plus';
import { MsgSuccess } from '@/utils/message';

// File, image and model transfers can be cancelled (temp files are removed;
// a re-sent model resumes from the files already on the target).
// App migration installs on the target first, so cancel is not offered.
const cancellable = computed(() => ['file', 'image', 'model'].includes(task.value?.kind || ''));
const cancelling = ref(false);
const onCancel = () => {
    ElMessageBox.confirm(i18n.global.t('cluster.cancelTaskConfirm'), i18n.global.t('cluster.cancelTask'), {
        confirmButtonText: i18n.global.t('commons.button.confirm'),
        cancelButtonText: i18n.global.t('commons.button.cancel'),
        type: 'warning',
    }).then(async () => {
        cancelling.value = true;
        try {
            await cancelClusterSyncTask(taskId.value);
            MsgSuccess(i18n.global.t('cluster.cancelTaskSubmitted'));
            refresh();
        } finally {
            cancelling.value = false;
        }
    });
};

const emit = defineEmits(['finished']);

const visible = ref(false);
const loading = ref(false);
const taskId = ref(0);
const task = ref<Cluster.SyncTask | null>(null);
const records = ref<Cluster.SyncTaskRecord[]>([]);
let timer: ReturnType<typeof setTimeout> | undefined;

// Stall detection: when progressDone last changed (client clock).
const lastDone = ref(-1);
const lastMoveAt = ref(Date.now());
const now = ref(Date.now());

const hasBytes = computed(() => (task.value?.progressTotal || 0) > 0);
const percent = computed(() => {
    const total = task.value?.progressTotal || 0;
    if (!total) return 0;
    return Math.min(100, Math.floor(((task.value?.progressDone || 0) * 1000) / total) / 10);
});
const stalledSeconds = computed(() => Math.max(0, Math.floor((now.value - lastMoveAt.value) / 1000)));
const formatSeconds = (sec: number) => {
    if (!isFinite(sec) || sec <= 0) return '—';
    const h = Math.floor(sec / 3600);
    const m = Math.floor((sec % 3600) / 60);
    const s = Math.floor(sec % 60);
    if (h) return `${h}h ${m}m`;
    if (m) return `${m}m ${s}s`;
    return `${s}s`;
};
const speedText = computed(() => {
    const rate = task.value?.progressRate || 0;
    return rate > 0 ? computeSize(rate) + '/s' : '—';
});
const etaText = computed(() => {
    const rate = task.value?.progressRate || 0;
    const left = (task.value?.progressTotal || 0) - (task.value?.progressDone || 0);
    return rate > 0 ? formatSeconds(left / rate) : '—';
});
const updatedText = computed(() => (task.value?.updatedAt ? dateFormatSimple(task.value.updatedAt) : '—'));

const trackMovement = () => {
    now.value = Date.now();
    const done = task.value?.progressDone || 0;
    if (done !== lastDone.value) {
        lastDone.value = done;
        lastMoveAt.value = now.value;
    }
};

const runningHint = computed(() =>
    task.value?.status === 'running'
        ? i18n.global.t('xpack.sync.submitSuccess')
        : i18n.global.t('commons.msg.noneData'),
);

const actionTagType = (action: string) => {
    if (action === 'create') return 'success';
    if (action === 'skip-same') return 'info';
    if (action === 'skip-conflict') return 'warning';
    if (action === 'cancelled') return 'info';
    return 'danger';
};
const actionLabel = (action: string) => {
    if (action === 'cancelled') return i18n.global.t('cluster.taskStatus_cancelled');
    if (action === 'create') return i18n.global.t('cluster.actionCreate');
    if (action === 'skip-same') return i18n.global.t('cluster.actionSkipSame');
    if (action === 'skip-conflict') return i18n.global.t('cluster.actionSkipConflict');
    if (action === 'failed') return i18n.global.t('cluster.actionFailed');
    return action;
};
const taskTagType = (status: string) => {
    if (status === 'success') return 'success';
    if (status === 'failed') return 'danger';
    if (status === 'partial' || status === 'running') return 'warning';
    return 'info';
};
const taskStatusLabel = (status: string) => {
    const key = 'cluster.taskStatus_' + status;
    const text = i18n.global.t(key);
    return text === key ? status : text;
};
const formatDuration = (ms: number) => {
    if (!ms) return '—';
    if (ms < 1000) return `${ms} ms`;
    return `${(ms / 1000).toFixed(1)} s`;
};

const stopPolling = () => {
    if (timer) {
        clearTimeout(timer);
        timer = undefined;
    }
};

const refresh = async () => {
    const id = taskId.value;
    if (!id) return;
    loading.value = true;
    try {
        // Tasks are listed newest first; the task just submitted is on the
        // first page.
        const [taskRes, recordRes] = await Promise.all([
            searchClusterSyncTasks({ page: 1, pageSize: 100 }),
            listClusterSyncTaskRecords(id),
        ]);
        if (id !== taskId.value) return;
        task.value = (taskRes.data.items || []).find((item) => item.id === id) || task.value;
        records.value = recordRes.data || [];
        trackMovement();
    } catch {
        // reported by the interceptor
    } finally {
        loading.value = false;
    }
    if (visible.value && task.value?.status === 'running') {
        timer = setTimeout(refresh, 3000);
    } else if (task.value && task.value.status !== 'running') {
        emit('finished', task.value);
    }
};

const open = (id: number, initial?: Cluster.SyncTask) => {
    stopPolling();
    taskId.value = id;
    task.value = initial || null;
    records.value = [];
    lastDone.value = -1;
    lastMoveAt.value = Date.now();
    visible.value = true;
    refresh();
};

const onClose = () => {
    stopPolling();
    visible.value = false;
};

onBeforeUnmount(stopPolling);

defineExpose({ open });
</script>

<style scoped>
.progress-box {
    padding: 12px 16px;
    border-radius: 6px;
    background: var(--el-fill-color-light);
}
.progress-meta {
    display: flex;
    flex-wrap: wrap;
    gap: 16px;
    font-size: 12px;
    color: var(--el-text-color-secondary);
}
.task-summary {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 16px;
}
</style>

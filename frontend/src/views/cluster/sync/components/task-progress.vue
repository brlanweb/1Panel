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
import { listClusterSyncTaskRecords, searchClusterSyncTasks } from '@/api/modules/cluster';
import { Cluster } from '@/api/interface/cluster';

const emit = defineEmits(['finished']);

const visible = ref(false);
const loading = ref(false);
const taskId = ref(0);
const task = ref<Cluster.SyncTask | null>(null);
const records = ref<Cluster.SyncTaskRecord[]>([]);
let timer: ReturnType<typeof setTimeout> | undefined;

const runningHint = computed(() =>
    task.value?.status === 'running'
        ? i18n.global.t('xpack.sync.submitSuccess')
        : i18n.global.t('commons.msg.noneData'),
);

const actionTagType = (action: string) => {
    if (action === 'create') return 'success';
    if (action === 'skip-same') return 'info';
    if (action === 'skip-conflict') return 'warning';
    return 'danger';
};
const actionLabel = (action: string) => {
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
.task-summary {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 16px;
}
</style>

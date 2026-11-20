<template>
    <div>
        <LayoutContent :title="$t('menu.clusterSync')" v-loading="loading">
            <template #main>
                <el-tabs v-model="activeKind" @tab-change="onKindChange">
                    <el-tab-pane :label="$t('cluster.syncKindFile')" name="file" />
                    <el-tab-pane :label="$t('cluster.syncKindImage')" name="image" />
                    <el-tab-pane :label="$t('cluster.syncKindApp')" name="app" />
                    <el-tab-pane :label="$t('cluster.syncKindCert')" name="cert" />
                </el-tabs>

                <el-card shadow="never" class="mb-4">
                    <el-form :inline="true">
                        <el-form-item :label="$t('cluster.sourceNode')">
                            <el-select
                                v-model="sourceNodeId"
                                class="p-w-200"
                                :placeholder="$t('cluster.sourceNode')"
                                @change="onSourceChange"
                                clearable
                            >
                                <el-option
                                    v-for="item in onlineNodes"
                                    :key="item.id"
                                    :label="item.name"
                                    :value="item.id"
                                />
                            </el-select>
                        </el-form-item>
                        <el-form-item :label="$t('cluster.targetNodes')">
                            <el-select
                                v-model="targetNodeIds"
                                class="p-w-300"
                                multiple
                                collapse-tags
                                :placeholder="$t('cluster.targetNodes')"
                                clearable
                            >
                                <el-option
                                    v-for="item in targetOptions"
                                    :key="item.id"
                                    :label="item.name"
                                    :value="item.id"
                                />
                            </el-select>
                        </el-form-item>
                        <el-form-item v-if="activeKind === 'file'" :label="$t('cluster.dirPath')">
                            <el-input v-model="dirPath" class="p-w-200" />
                        </el-form-item>
                        <el-form-item>
                            <el-button type="primary" :disabled="!sourceNodeId" @click="searchResources">
                                {{ $t('cluster.searchResources') }}
                            </el-button>
                        </el-form-item>
                    </el-form>
                </el-card>

                <el-card shadow="never" class="mb-4">
                    <el-table
                        :data="resources"
                        style="width: 100%"
                        @selection-change="onResourceSelect"
                        v-loading="resourceLoading"
                        row-key="key"
                    >
                        <el-table-column type="selection" width="48" />
                        <el-table-column :label="$t('cluster.resourceName')" prop="name" min-width="200px" />
                        <el-table-column :label="$t('cluster.resourceDetail')" prop="detail" min-width="200px" />
                        <el-table-column :label="$t('cluster.resourceSize')" prop="size" width="120px" align="right">
                            <template #default="{ row }">
                                {{ formatSize(row.size) }}
                            </template>
                        </el-table-column>
                    </el-table>
                    <el-empty v-if="!resources.length && !resourceLoading" :description="$t('commons.msg.noneData')" />
                    <div class="sync-actions">
                        <el-button
                            type="primary"
                            :disabled="!canPreview"
                            :loading="previewLoading"
                            @click="runPreview"
                        >
                            {{ $t('cluster.preview') }}
                        </el-button>
                    </div>
                </el-card>

                <el-card shadow="never" class="mb-4" v-if="previewItems.length">
                    <el-table :data="previewItems" style="width: 100%">
                        <el-table-column :label="$t('cluster.resourceName')" prop="resourceName" min-width="200px" />
                        <el-table-column :label="$t('cluster.targetNodes')" prop="targetName" width="160px" />
                        <el-table-column :label="$t('cluster.action')" prop="action" width="140px" align="center">
                            <template #default="{ row }">
                                <el-tag :type="previewTagType(row.action)" size="small">
                                    {{ previewActionLabel(row.action) }}
                                </el-tag>
                            </template>
                        </el-table-column>
                        <el-table-column :label="$t('cluster.reason')" prop="reason" min-width="200px" />
                    </el-table>
                    <div class="sync-actions">
                        <el-checkbox v-model="overwriteConflicts">
                            {{ $t('cluster.overwriteConflicts') }}
                        </el-checkbox>
                        <el-text type="warning" size="small">{{ $t('cluster.overwriteWarning') }}</el-text>
                        <el-button
                            type="primary"
                            :disabled="!hasCreateItems"
                            :loading="executeLoading"
                            @click="runExecute"
                        >
                            {{ $t('cluster.execute') }}
                        </el-button>
                    </div>
                    <el-text v-if="!hasCreateItems" type="info" size="small">
                        {{ $t('cluster.noCreateItems') }}
                    </el-text>
                </el-card>

                <el-card shadow="never">
                    <template #header>
                        <div class="task-header">
                            <span>{{ $t('cluster.taskHistory') }}</span>
                            <TableRefresh @search="searchTasks()" />
                        </div>
                    </template>
                    <ComplexTable
                        :pagination-config="taskPagination"
                        :data="tasks"
                        @search="searchTasks()"
                        :columns="taskColumns"
                        localKey="clusterSyncTaskColumn"
                        row-key="id"
                    >
                        <el-table-column :label="$t('cluster.syncTask')" prop="id" width="80px" />
                        <el-table-column :label="$t('cluster.type')" prop="kind" width="90px" />
                        <el-table-column :label="$t('cluster.sourceNode')" prop="sourceName" min-width="140px" />
                        <el-table-column :label="$t('cluster.taskStatus')" prop="status" width="120px">
                            <template #default="{ row }">
                                <el-tag :type="taskTagType(row.status)" size="small">{{ row.status }}</el-tag>
                            </template>
                        </el-table-column>
                        <el-table-column :label="$t('cluster.succeeded')" prop="succeeded" width="90px" align="center" />
                        <el-table-column :label="$t('cluster.skipped')" prop="skipped" width="90px" align="center" />
                        <el-table-column :label="$t('cluster.failed')" prop="failed" width="90px" align="center" />
                        <el-table-column :label="$t('cluster.createdAt')" prop="createdAt" width="170px">
                            <template #default="{ row }">
                                {{ row.createdAt ? dateFormatSimple(row.createdAt) : '—' }}
                            </template>
                        </el-table-column>
                        <fu-table-operations
                            width="120px"
                            :buttons="taskButtons"
                            :label="$t('commons.table.operate')"
                            fix
                        />
                    </ComplexTable>
                </el-card>
            </template>
        </LayoutContent>

        <DrawerPro
            v-model="detailVisible"
            :header="$t('cluster.taskDetail')"
            :resource="detailTaskId ? String(detailTaskId) : ''"
            @close="detailVisible = false"
            size="large"
        >
            <el-table :data="taskRecords" style="width: 100%" v-loading="recordLoading">
                <el-table-column :label="$t('cluster.resourceName')" prop="resourceName" min-width="180px" />
                <el-table-column :label="$t('cluster.targetNodes')" prop="targetName" width="140px" />
                <el-table-column :label="$t('cluster.action')" prop="action" width="130px" align="center">
                    <template #default="{ row }">
                        <el-tag :type="previewTagType(row.action)" size="small">
                            {{ previewActionLabel(row.action) }}
                        </el-tag>
                    </template>
                </el-table-column>
                <el-table-column :label="$t('cluster.reason')" prop="reason" min-width="180px" />
                <el-table-column :label="$t('cluster.duration')" prop="durationMs" width="110px" align="right">
                    <template #default="{ row }">{{ row.durationMs }} ms</template>
                </el-table-column>
            </el-table>
            <el-empty v-if="!taskRecords.length && !recordLoading" :description="$t('commons.msg.noneData')" />
        </DrawerPro>
    </div>
</template>

<script lang="ts" setup>
import { computed, onMounted, ref } from 'vue';
import i18n from '@/lang';
import {
    executeClusterSync,
    listClusterSyncTaskRecords,
    previewClusterSync,
    searchClusterNodes,
    searchClusterSyncResources,
    searchClusterSyncTasks,
} from '@/api/modules/cluster';
import { Cluster } from '@/api/interface/cluster';
import { MsgError, MsgSuccess } from '@/utils/message';
import { dateFormatSimple } from '@/utils/date';
import { usePageState } from '@/composables/usePageState';

const loading = ref(false);
const resourceLoading = ref(false);
const previewLoading = ref(false);
const executeLoading = ref(false);
const recordLoading = ref(false);

const activeKind = ref<Cluster.SyncKind>('file');
const allNodes = ref<Cluster.NodeInfo[]>([]);
const sourceNodeId = ref<number | undefined>(undefined);
const targetNodeIds = ref<number[]>([]);
const dirPath = ref('/');
const resources = ref<Cluster.SyncResource[]>([]);
const selectedResources = ref<Cluster.SyncResource[]>([]);
const previewItems = ref<Cluster.SyncPreviewItem[]>([]);
const overwriteConflicts = ref(false);
const tasks = ref<Cluster.SyncTask[]>([]);
const taskColumns = ref([]);
const detailVisible = ref(false);
const detailTaskId = ref(0);
const taskRecords = ref<Cluster.SyncTaskRecord[]>([]);

const pageState = usePageState(() => ({
    taskPagination: {
        currentPage: 1,
        pageSize: 20,
        total: 0,
    },
}));
const taskPagination = pageState.taskPagination;

const onlineNodes = computed(() => allNodes.value.filter((node) => node.status === 'online'));
const targetOptions = computed(() =>
    onlineNodes.value.filter((node) => node.id !== sourceNodeId.value),
);
const canPreview = computed(
    () => !!sourceNodeId.value && targetNodeIds.value.length > 0 && selectedResources.value.length > 0,
);
const hasCreateItems = computed(() => previewItems.value.some((item) => item.action === 'create'));

const formatSize = (size: number) => {
    if (!size) return '—';
    if (size < 1024) return `${size} B`;
    if (size < 1024 * 1024) return `${(size / 1024).toFixed(1)} KB`;
    if (size < 1024 * 1024 * 1024) return `${(size / 1024 / 1024).toFixed(1)} MB`;
    return `${(size / 1024 / 1024 / 1024).toFixed(2)} GB`;
};

const previewTagType = (action: string) => {
    if (action === 'create') return 'success';
    if (action === 'skip-same') return 'info';
    if (action === 'skip-conflict') return 'warning';
    return 'danger';
};

const previewActionLabel = (action: string) => {
    if (action === 'create') return i18n.global.t('cluster.actionCreate');
    if (action === 'skip-same') return i18n.global.t('cluster.actionSkipSame');
    if (action === 'skip-conflict') return i18n.global.t('cluster.actionSkipConflict');
    if (action === 'failed') return i18n.global.t('cluster.actionFailed');
    return action;
};

const taskTagType = (status: string) => {
    if (status === 'success' || status === 'completed') return 'success';
    if (status === 'failed') return 'danger';
    if (status === 'running') return 'warning';
    return 'info';
};

const clearStepResults = () => {
    resources.value = [];
    selectedResources.value = [];
    previewItems.value = [];
};

const onKindChange = () => {
    clearStepResults();
};

const onSourceChange = () => {
    targetNodeIds.value = targetNodeIds.value.filter((id) => id !== sourceNodeId.value);
    clearStepResults();
};

const onResourceSelect = (selection: Cluster.SyncResource[]) => {
    selectedResources.value = selection;
    previewItems.value = [];
};

const loadNodes = async () => {
    loading.value = true;
    try {
        const res = await searchClusterNodes({ page: 1, pageSize: 500 });
        allNodes.value = res.data.items;
    } catch {
        // The request interceptor already reports the error.
    } finally {
        loading.value = false;
    }
};

const searchResources = async () => {
    if (!sourceNodeId.value) {
        MsgError(i18n.global.t('cluster.selectSourceFirst'));
        return;
    }
    resourceLoading.value = true;
    previewItems.value = [];
    await searchClusterSyncResources({
        kind: activeKind.value,
        sourceNodeId: sourceNodeId.value,
        path: activeKind.value === 'file' ? dirPath.value : undefined,
        page: 1,
        pageSize: 100,
    })
        .then((res) => {
            resources.value = res.data || [];
        })
        .finally(() => {
            resourceLoading.value = false;
        });
};

const runPreview = async () => {
    if (!sourceNodeId.value) {
        MsgError(i18n.global.t('cluster.selectSourceFirst'));
        return;
    }
    if (!targetNodeIds.value.length) {
        MsgError(i18n.global.t('cluster.selectTargetsFirst'));
        return;
    }
    if (!selectedResources.value.length) {
        MsgError(i18n.global.t('cluster.selectResourcesFirst'));
        return;
    }
    previewLoading.value = true;
    await previewClusterSync({
        kind: activeKind.value,
        sourceNodeId: sourceNodeId.value,
        targetNodeIds: targetNodeIds.value,
        resourceKeys: selectedResources.value.map((item) => item.key),
        path: activeKind.value === 'file' ? dirPath.value : undefined,
    })
        .then((res) => {
            previewItems.value = res.data || [];
            if (!previewItems.value.length) {
                MsgError(i18n.global.t('cluster.previewEmpty'));
            }
        })
        .finally(() => {
            previewLoading.value = false;
        });
};

const pollTask = async (taskId: number, attempts: number) => {
    if (attempts <= 0) {
        return;
    }
    await searchTasks();
    const task = tasks.value.find((item) => item.id === taskId);
    if (task && (task.status === 'running' || task.status === 'pending')) {
        setTimeout(() => pollTask(taskId, attempts - 1), 3000);
    }
};

const runExecute = async () => {
    if (!sourceNodeId.value || !targetNodeIds.value.length || !selectedResources.value.length) {
        return;
    }
    executeLoading.value = true;
    await executeClusterSync({
        kind: activeKind.value,
        sourceNodeId: sourceNodeId.value,
        targetNodeIds: targetNodeIds.value,
        resourceKeys: selectedResources.value.map((item) => item.key),
        path: activeKind.value === 'file' ? dirPath.value : undefined,
        overwriteConflicts: overwriteConflicts.value,
    })
        .then((res) => {
            MsgSuccess(i18n.global.t('cluster.executeSuccess', [res.data.taskId]));
            previewItems.value = [];
            pollTask(res.data.taskId, 10);
        })
        .finally(() => {
            executeLoading.value = false;
        });
};

const searchTasks = async () => {
    await searchClusterSyncTasks({
        page: taskPagination.currentPage,
        pageSize: taskPagination.pageSize,
        kind: activeKind.value,
    })
        .then((res) => {
            tasks.value = res.data.items;
            taskPagination.total = res.data.total;
        })
        .catch(() => {
            // The request interceptor already reports the error.
        });
};

const openTaskDetail = async (row: Cluster.SyncTask) => {
    detailTaskId.value = row.id;
    detailVisible.value = true;
    recordLoading.value = true;
    await listClusterSyncTaskRecords(row.id)
        .then((res) => {
            taskRecords.value = res.data || [];
        })
        .finally(() => {
            recordLoading.value = false;
        });
};

const taskButtons = [
    {
        label: i18n.global.t('cluster.taskDetail'),
        click: (row: Cluster.SyncTask) => {
            openTaskDetail(row);
        },
    },
];

onMounted(() => {
    loadNodes();
    searchTasks();
});
</script>

<style scoped>
.mb-4 {
    margin-bottom: 16px;
}

.sync-actions {
    display: flex;
    align-items: center;
    gap: 12px;
    margin-top: 16px;
    flex-wrap: wrap;
}

.task-header {
    display: flex;
    justify-content: space-between;
    align-items: center;
}
</style>

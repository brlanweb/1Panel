<template>
    <div>
        <SyncHeader />
        <el-card shadow="never" class="mt-3">
            <template #header>
                <div class="flex flex-wrap items-center justify-between gap-2">
                    <div>
                        <div class="font-medium">{{ $t('xpack.sync.image') }}</div>
                        <div class="sync-desc">{{ $t('xpack.sync.imageBatchDesc') }}</div>
                    </div>
                    <div class="flex items-center gap-2">
                        <el-select v-model="source" class="p-w-200" filterable :placeholder="$t('cluster.sourceNode')">
                            <el-option
                                v-for="item in selectableNodes"
                                :key="item.key"
                                :label="item.name"
                                :value="item.key"
                            />
                        </el-select>
                        <TableSearch v-model:searchName="search" @search="load(1)" />
                        <TableRefresh @search="load()" />
                    </div>
                </div>
            </template>
            <el-alert v-if="error" :title="error" type="error" show-icon :closable="false" class="mb-2" />
            <ComplexTable
                ref="tableRef"
                :cache-pagination="false"
                :pagination-config="pagination"
                :data="rows"
                :heightDiff="520"
                row-key="key"
                v-model:selects="selects"
                v-loading="loading"
                @search="load()"
            >
                <el-table-column type="selection" width="45" reserve-selection />
                <el-table-column label="ID" width="150">
                    <template #default="{ row }">{{ shortImageID(row.key) }}</template>
                </el-table-column>
                <el-table-column :label="$t('container.tag')" min-width="260">
                    <template #default="{ row }">
                        <div class="flex flex-wrap gap-1">
                            <el-tag v-for="tag in row.meta?.tags || []" :key="tag" size="small">{{ tag }}</el-tag>
                            <span v-if="!(row.meta?.tags || []).length">&lt;none&gt;</span>
                        </div>
                    </template>
                </el-table-column>
                <el-table-column :label="$t('container.size')" width="120">
                    <template #default="{ row }">{{ computeSize(row.size) }}</template>
                </el-table-column>
            </ComplexTable>
        </el-card>

        <TargetNodes
            v-model="targets"
            :nodes="nodes"
            :source="source"
            :has-resource="selects.length > 0"
            :submitting="submitting"
            @handle="onHandle"
        />

        <BatchConfirm
            ref="confirmRef"
            :title="$t('xpack.sync.image')"
            summary-key="xpack.sync.imageBatchConfirmPrefix"
            :submitting="submitting"
            @confirm="onConfirm"
        />
        <TaskProgress ref="progressRef" />
    </div>
</template>

<script lang="ts" setup>
import { onMounted, reactive, ref, watch } from 'vue';
import i18n from '@/lang';
import { executeClusterSync, searchClusterSyncResources } from '@/api/modules/cluster';
import { Cluster } from '@/api/interface/cluster';
import { computeSize } from '@/utils/size';
import { MsgSuccess } from '@/utils/message';
import SyncHeader from '../components/header.vue';
import TargetNodes from '../components/target-nodes.vue';
import BatchConfirm from '../components/batch-confirm.vue';
import TaskProgress from '../components/task-progress.vue';
import { LOCAL_KEY, shortImageID, useSyncNodes } from '../common';

defineOptions({ name: 'ClusterSyncImage' });

const { nodes, selectableNodes, loadNodes, nodeName, nodeId } = useSyncNodes();
const source = ref(LOCAL_KEY);
const search = ref('');
const rows = ref<Cluster.SyncResource[]>([]);
const selects = ref<Cluster.SyncResource[]>([]);
const targets = ref<string[]>([]);
const loading = ref(false);
const submitting = ref(false);
const error = ref('');
const tableRef = ref();
const confirmRef = ref();
const progressRef = ref();
const pagination = reactive({ currentPage: 1, pageSize: 50, total: 0 });

let requestId = 0;
const load = async (page?: number) => {
    const id = nodeId(source.value);
    if (id === undefined) return;
    if (page) pagination.currentPage = page;
    const current = ++requestId;
    loading.value = true;
    error.value = '';
    try {
        const res = await searchClusterSyncResources({
            kind: 'image',
            sourceNodeId: id,
            search: search.value,
            page: pagination.currentPage,
            pageSize: pagination.pageSize,
        });
        if (current !== requestId) return;
        rows.value = res.data.items || [];
        pagination.total = res.data.total || 0;
    } catch (err: any) {
        if (current !== requestId) return;
        rows.value = [];
        error.value = err?.message || i18n.global.t('cluster.fetchError');
    } finally {
        if (current === requestId) loading.value = false;
    }
};

watch(source, () => {
    tableRef.value?.clearSelects();
    selects.value = [];
    search.value = '';
    load(1);
});

const imageLabel = (row: Cluster.SyncResource) => {
    const tags: string[] = row.meta?.tags || [];
    return tags.length ? tags.join(', ') : shortImageID(row.key);
};

const onHandle = () => {
    confirmRef.value.open(
        selects.value.map((row) => ({ key: row.key, label: imageLabel(row) })),
        targets.value.map((key) => ({ key, label: nodeName(key) })),
    );
};

const onConfirm = async (resourceKeys: string[], targetKeys: string[]) => {
    submitting.value = true;
    try {
        const res = await executeClusterSync({
            kind: 'image',
            sourceNodeId: nodeId(source.value),
            targetNodeIds: targetKeys.map((key) => nodeId(key)),
            resourceKeys,
            overwriteConflicts: false,
        });
        confirmRef.value.close();
        MsgSuccess(i18n.global.t('xpack.sync.submitSuccess'));
        progressRef.value.open(res.data.taskId);
    } finally {
        submitting.value = false;
    }
};

onMounted(async () => {
    await loadNodes();
    load(1);
});
</script>

<style scoped>
.sync-desc {
    font-size: 12px;
    color: var(--el-text-color-secondary);
    margin-top: 4px;
}
</style>

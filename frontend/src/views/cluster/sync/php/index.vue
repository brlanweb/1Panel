<template>
    <div>
        <SyncHeader />
        <el-card shadow="never" class="mt-3">
            <template #header>
                <div class="flex flex-wrap items-center justify-between gap-2">
                    <div>
                        <div class="font-medium">{{ $t('xpack.sync.phpExtensionsTitle') }}</div>
                        <div class="sync-desc">{{ $t('xpack.sync.phpExtensionsDesc') }}</div>
                    </div>
                    <div class="flex items-center gap-2">
                        <el-select
                            v-model="source"
                            class="p-w-200"
                            filterable
                            :disabled="submitting"
                            :placeholder="$t('cluster.sourceNode')"
                        >
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
                :cache-pagination="false"
                :pagination-config="pagination"
                :data="rows"
                :heightDiff="520"
                v-loading="loading"
                @search="load()"
            >
                <el-table-column :label="$t('commons.table.name')" prop="name" width="180" show-overflow-tooltip />
                <el-table-column :label="$t('xpack.sync.phpExtensionsList')" min-width="300">
                    <template #default="{ row }">
                        <div class="flex flex-wrap gap-1">
                            <el-tag v-for="ext in extList(row)" :key="ext" size="small" type="info">{{ ext }}</el-tag>
                        </div>
                    </template>
                </el-table-column>
                <el-table-column :label="$t('commons.table.operate')" width="100" fixed="right">
                    <template #default="{ row }">
                        <el-button link type="primary" :disabled="selected?.key === row.key" @click="selected = row">
                            {{ $t('commons.button.migrate') }}
                        </el-button>
                    </template>
                </el-table-column>
            </ComplexTable>
        </el-card>

        <TargetNodes
            v-model="targets"
            :nodes="nodes"
            :source="source"
            :has-resource="!!selected"
            :submitting="submitting"
            @handle="onHandle"
        >
            <template #selected>
                <div v-if="selected" class="mb-3">
                    <el-tag closable @close="selected = null">{{ selected.name }}</el-tag>
                </div>
            </template>
        </TargetNodes>

        <BatchConfirm
            ref="confirmRef"
            :title="$t('xpack.sync.phpExtensionsTitle')"
            summary-key="xpack.sync.phpExtensionsConfirm"
            :hint="$t('xpack.sync.phpExtensionsConflictHint')"
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
import { MsgSuccess } from '@/utils/message';
import SyncHeader from '../components/header.vue';
import TargetNodes from '../components/target-nodes.vue';
import BatchConfirm from '../components/batch-confirm.vue';
import TaskProgress from '../components/task-progress.vue';
import { LOCAL_KEY, useSyncNodes } from '../common';

defineOptions({ name: 'ClusterSyncPhp' });

const { nodes, selectableNodes, loadNodes, nodeName, nodeId } = useSyncNodes();
const source = ref(LOCAL_KEY);
const search = ref('');
const rows = ref<Cluster.SyncResource[]>([]);
const selected = ref<Cluster.SyncResource | null>(null);
const targets = ref<string[]>([]);
const loading = ref(false);
const submitting = ref(false);
const error = ref('');
const confirmRef = ref();
const progressRef = ref();
const pagination = reactive({ currentPage: 1, pageSize: 5, total: 0 });

const extList = (row: Cluster.SyncResource) =>
    String(row.detail || '')
        .split(',')
        .map((item) => item.trim())
        .filter(Boolean);

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
            kind: 'php',
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
    selected.value = null;
    search.value = '';
    load(1);
});

const onHandle = () => {
    if (!selected.value) return;
    confirmRef.value.open(
        [{ key: selected.value.key, label: selected.value.name }],
        targets.value.map((key) => ({ key, label: nodeName(key) })),
    );
};

const onConfirm = async (resourceKeys: string[], targetKeys: string[]) => {
    submitting.value = true;
    try {
        const res = await executeClusterSync({
            kind: 'php',
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

<template>
    <div>
        <SyncHeader />
        <el-card shadow="never" class="mt-3">
            <template #header>
                <div class="flex flex-wrap items-center justify-between gap-2">
                    <div>
                        <div class="font-medium">{{ $t('xpack.sync.ssl') }}</div>
                        <div class="sync-desc">{{ $t('xpack.sync.sslBatchDesc') }}</div>
                    </div>
                    <div class="flex items-center gap-2">
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
                <el-table-column :label="$t('website.domain')" min-width="220" show-overflow-tooltip>
                    <template #default="{ row }">
                        {{ row.name }}
                        <span v-if="otherDomains(row)" class="sub-text">{{ otherDomains(row) }}</span>
                    </template>
                </el-table-column>
                <el-table-column :label="$t('commons.table.status')" width="120">
                    <template #default="{ row }">
                        <Status v-if="row.meta?.status" :status="row.meta.status" />
                    </template>
                </el-table-column>
                <el-table-column :label="$t('website.provider')" width="160">
                    <template #default="{ row }">{{ providerLabel(row.meta?.provider) }}</template>
                </el-table-column>
                <el-table-column :label="$t('website.expireDate')" width="180">
                    <template #default="{ row }">
                        {{ row.meta?.expireDate ? dateFormatSimple(row.meta.expireDate) : '—' }}
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
            :source="LOCAL_KEY"
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
            :title="$t('xpack.sync.ssl')"
            summary-key="xpack.sync.sslBatchConfirmPrefix"
            :submitting="submitting"
            @confirm="onConfirm"
        />
        <TaskProgress ref="progressRef" />
    </div>
</template>

<script lang="ts" setup>
import { onMounted, reactive, ref } from 'vue';
import i18n from '@/lang';
import { executeClusterSync, searchClusterSyncResources } from '@/api/modules/cluster';
import { Cluster } from '@/api/interface/cluster';
import { dateFormatSimple } from '@/utils/date';
import { MsgSuccess } from '@/utils/message';
import SyncHeader from '../components/header.vue';
import TargetNodes from '../components/target-nodes.vue';
import BatchConfirm from '../components/batch-confirm.vue';
import TaskProgress from '../components/task-progress.vue';
import { LOCAL_KEY, useSyncNodes } from '../common';

defineOptions({ name: 'ClusterSyncSsl' });

// Certificates are always synced from this (master) panel.
const { nodes, loadNodes, nodeName, nodeId } = useSyncNodes();
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

const otherDomains = (row: Cluster.SyncResource) => {
    const list = String(row.meta?.domains || '')
        .split(',')
        .map((item) => item.trim())
        .filter((item) => item && item !== row.name);
    return list.length ? `(+${list.join(', ')})` : '';
};

const providerLabel = (provider: string) => {
    if (!provider) return '—';
    const key = 'ssl.' + provider;
    const text = i18n.global.t(key);
    return text === key ? provider : text;
};

const load = async (page?: number) => {
    if (page) pagination.currentPage = page;
    loading.value = true;
    error.value = '';
    try {
        const res = await searchClusterSyncResources({
            kind: 'cert',
            sourceNodeId: 0,
            search: search.value,
            page: pagination.currentPage,
            pageSize: pagination.pageSize,
        });
        rows.value = res.data.items || [];
        pagination.total = res.data.total || 0;
    } catch (err: any) {
        rows.value = [];
        error.value = err?.message || i18n.global.t('cluster.fetchError');
    } finally {
        loading.value = false;
    }
};

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
            kind: 'cert',
            sourceNodeId: 0,
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

onMounted(() => {
    loadNodes();
    load(1);
});
</script>

<style scoped>
.sync-desc {
    font-size: 12px;
    color: var(--el-text-color-secondary);
    margin-top: 4px;
}
.sub-text {
    font-size: 12px;
    color: var(--el-text-color-secondary);
    margin-left: 4px;
}
</style>

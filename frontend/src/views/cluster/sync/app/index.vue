<template>
    <div>
        <SyncHeader />
        <el-card shadow="never" class="mt-3">
            <template #header>
                <div class="flex flex-wrap items-center justify-between gap-2">
                    <div>
                        <div class="font-medium">{{ $t('xpack.sync.app') }}</div>
                        <div class="sync-desc">{{ $t('xpack.sync.appBatchDesc') }}</div>
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
                <el-table-column :label="$t('commons.table.name')" prop="name" min-width="160" show-overflow-tooltip />
                <el-table-column :label="$t('app.app')" min-width="140">
                    <template #default="{ row }">{{ row.meta?.appName || row.meta?.appKey }}</template>
                </el-table-column>
                <el-table-column :label="$t('app.version')" width="140">
                    <template #default="{ row }">{{ row.meta?.version }}</template>
                </el-table-column>
                <el-table-column :label="$t('commons.table.status')" width="120">
                    <template #default="{ row }">
                        <Status v-if="row.meta?.status" :status="row.meta.status" />
                    </template>
                </el-table-column>
                <el-table-column :label="$t('commons.table.operate')" width="100" fixed="right">
                    <template #default="{ row }">
                        <el-tooltip
                            :content="$t('xpack.sync.appUnsupportedResourceType')"
                            :disabled="!appUnsupported(row)"
                        >
                            <span>
                                <el-button
                                    link
                                    type="primary"
                                    :disabled="appUnsupported(row) || selected?.key === row.key || submitting"
                                    @click="onSelect(row)"
                                >
                                    {{ $t('commons.button.migrate') }}
                                </el-button>
                            </span>
                        </el-tooltip>
                    </template>
                </el-table-column>
            </ComplexTable>
        </el-card>

        <el-card shadow="never" class="mt-4">
            <template #header>
                <div class="flex flex-wrap items-center justify-between gap-2">
                    <div>
                        <div class="font-medium">{{ $t('xpack.sync.targetTitle') }}</div>
                        <div class="sync-desc">{{ $t('xpack.sync.targetDesc') }}</div>
                    </div>
                    <el-checkbox
                        :model-value="allChecked"
                        :indeterminate="indeterminate"
                        :disabled="!candidates.length || submitting"
                        @change="onCheckAll"
                    >
                        {{ $t('commons.button.selectAll') }}
                    </el-checkbox>
                </div>
            </template>
            <div v-if="selected" class="mb-3">
                <span class="sync-desc">{{ $t('xpack.sync.appSelectedApp') }}</span>
                <el-tag closable @close="onSelect(null)">{{ selected.name }}</el-tag>
            </div>
            <el-checkbox-group v-if="candidates.length" v-model="targets" :disabled="submitting" class="target-grid">
                <el-checkbox v-for="node in candidates" :key="node.key" :value="node.key" border class="target-item">
                    <div class="flex items-center justify-between gap-2">
                        <span class="font-medium">{{ node.name }}</span>
                        <el-tag size="small" :type="nodeStatusType(node.status)">
                            {{ nodeStatusLabel(node.status) }}
                        </el-tag>
                    </div>
                    <div v-if="node.addr" class="sync-desc">{{ node.addr }}</div>
                </el-checkbox>
            </el-checkbox-group>
            <el-empty
                v-else
                :image-size="60"
                :description="source ? $t('xpack.sync.noTargetNodes') : $t('xpack.sync.noSourceNode')"
            />
        </el-card>

        <el-card shadow="never" class="mt-4">
            <template #header>
                <div>
                    <div class="font-medium">{{ $t('xpack.sync.appPreviewTitle') }}</div>
                    <div class="sync-desc">{{ $t('xpack.sync.appPreviewDesc') }}</div>
                </div>
            </template>
            <el-empty
                v-if="!selected || !targets.length"
                :image-size="60"
                :description="$t('xpack.sync.reviewEmpty')"
            />
            <template v-else>
                <div v-for="key in targets" :key="key" class="preview-row">
                    <el-tag>{{ selected.name }} / {{ selected.meta?.version }}</el-tag>
                    <el-icon class="preview-arrow"><Right /></el-icon>
                    <el-tag type="success">{{ nodeName(key) }}</el-tag>
                    <div class="flex-1"></div>
                    <template v-if="checks[key]">
                        <el-icon v-if="checks[key].action === 'waiting'" class="is-loading"><Loading /></el-icon>
                        <el-tag v-else size="small" :type="checkTagType(checks[key].action)">
                            {{ checkLabel(checks[key].action) }}
                        </el-tag>
                        <span v-if="checks[key].reason" class="preview-reason">{{ checks[key].reason }}</span>
                    </template>
                </div>
                <el-form v-if="needsDB" label-position="top" class="mt-4">
                    <el-form-item :label="$t('xpack.sync.appDatabaseDependency')" required>
                        <el-select
                            v-model="dbHost"
                            class="p-w-300"
                            :loading="dbLoading"
                            :disabled="submitting"
                            :placeholder="$t('xpack.sync.appDatabaseDependency')"
                        >
                            <el-option
                                v-for="db in dbOptions"
                                :key="db.type + '/' + db.database"
                                :label="db.type + ' / ' + db.database"
                                :value="db.database"
                            />
                        </el-select>
                    </el-form-item>
                </el-form>
            </template>
            <div class="flex justify-end mt-4">
                <el-button type="primary" :disabled="!canExecute" :loading="submitting" @click="onExecute">
                    {{ $t('commons.button.handle') }}
                </el-button>
            </div>
            <div class="sync-desc text-right mt-2">
                {{ needsDB && !dbHost ? $t('xpack.sync.appResourceRequired') : $t('xpack.sync.appPreviewDesc') }}
            </div>
        </el-card>

        <TaskProgress ref="progressRef" />
    </div>
</template>

<script lang="ts" setup>
import { computed, h, onMounted, reactive, ref, watch } from 'vue';
import { Loading, Right } from '@element-plus/icons-vue';
import { ElMessageBox } from 'element-plus';
import i18n from '@/lang';
import {
    executeClusterSync,
    listClusterSyncAppDatabases,
    previewClusterSync,
    searchClusterSyncResources,
} from '@/api/modules/cluster';
import { Cluster } from '@/api/interface/cluster';
import { MsgSuccess } from '@/utils/message';
import SyncHeader from '../components/header.vue';
import TaskProgress from '../components/task-progress.vue';
import { appNeedsDB, appUnsupported, LOCAL_KEY, nodeStatusLabel, nodeStatusType, useSyncNodes } from '../common';

defineOptions({ name: 'ClusterSyncApp' });

const { nodes, selectableNodes, loadNodes, nodeName, nodeId } = useSyncNodes();
const source = ref(LOCAL_KEY);
const search = ref('');
const rows = ref<Cluster.SyncResource[]>([]);
const selected = ref<Cluster.SyncResource | null>(null);
const targets = ref<string[]>([]);
const loading = ref(false);
const submitting = ref(false);
const error = ref('');
const progressRef = ref();
const pagination = reactive({ currentPage: 1, pageSize: 10, total: 0 });

const checks = ref<Record<string, { action: string; reason: string }>>({});
const dbOptions = ref<Cluster.SyncAppDB[]>([]);
const dbLoading = ref(false);
const dbHost = ref('');

const candidates = computed(() =>
    source.value ? nodes.value.filter((node) => node.key !== source.value && node.status !== 'offline') : [],
);
const allChecked = computed(
    () => candidates.value.length > 0 && candidates.value.every((node) => targets.value.includes(node.key)),
);
const indeterminate = computed(
    () => !allChecked.value && candidates.value.some((node) => targets.value.includes(node.key)),
);
const onCheckAll = (checked: boolean) => {
    targets.value = checked ? candidates.value.map((node) => node.key) : [];
};
watch(candidates, (list) => {
    const keys = new Set(list.map((node) => node.key));
    targets.value = targets.value.filter((key) => keys.has(key));
});

const needsDB = computed(() => appNeedsDB(selected.value || undefined));
const checksDone = computed(() =>
    targets.value.every((key) => checks.value[key] && checks.value[key].action !== 'waiting'),
);
const canExecute = computed(
    () =>
        !!selected.value &&
        targets.value.length > 0 &&
        checksDone.value &&
        targets.value.some((key) => checks.value[key]?.action === 'create') &&
        (!needsDB.value || !!dbHost.value) &&
        !submitting.value,
);

const checkTagType = (action: string) => {
    if (action === 'create') return 'success';
    if (action === 'skip-same' || action === 'skip-conflict') return 'warning';
    return 'danger';
};
const checkLabel = (action: string) => {
    if (action === 'create') return i18n.global.t('cluster.actionCreate');
    if (action === 'skip-same') return i18n.global.t('cluster.actionSkipSame');
    if (action === 'skip-conflict') return i18n.global.t('cluster.actionSkipConflict');
    return i18n.global.t('cluster.actionFailed');
};

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
            kind: 'app',
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

// Per-target check (name / version already installed, unsupported app).
let checkId = 0;
const runChecks = async () => {
    const current = ++checkId;
    const app = selected.value;
    const keys = [...targets.value];
    if (!app || !keys.length) {
        checks.value = {};
        return;
    }
    const next: Record<string, { action: string; reason: string }> = {};
    keys.forEach((key) => (next[key] = { action: 'waiting', reason: '' }));
    checks.value = next;
    try {
        const res = await previewClusterSync({
            kind: 'app',
            sourceNodeId: nodeId(source.value),
            targetNodeIds: keys.map((key) => nodeId(key)),
            resourceKeys: [app.key],
        });
        if (current !== checkId) return;
        const result: Record<string, { action: string; reason: string }> = {};
        for (const key of keys) {
            const item = (res.data || []).find((row) => row.targetNodeId === nodeId(key));
            result[key] = item ? { action: item.action, reason: item.reason } : { action: 'failed', reason: '' };
        }
        checks.value = result;
    } catch (err: any) {
        if (current !== checkId) return;
        const result: Record<string, { action: string; reason: string }> = {};
        keys.forEach((key) => (result[key] = { action: 'failed', reason: err?.message || '' }));
        checks.value = result;
    }
};

// Database services present on every selected target (intersection).
let dbId = 0;
const loadDatabases = async () => {
    const current = ++dbId;
    dbOptions.value = [];
    if (!needsDB.value || !targets.value.length) {
        dbHost.value = '';
        return;
    }
    const dbType = String(selected.value?.meta?.dbType || '').toLowerCase();
    dbLoading.value = true;
    try {
        const res = await listClusterSyncAppDatabases({
            nodeIds: targets.value.map((key) => nodeId(key)),
            types: dbType || undefined,
        });
        if (current !== dbId) return;
        dbOptions.value = res.data || [];
        if (!dbOptions.value.some((db) => db.database === dbHost.value)) {
            const same = dbOptions.value.find((db) => db.database === selected.value?.meta?.dbHostName);
            dbHost.value = same ? same.database : '';
        }
    } catch {
        if (current === dbId) dbHost.value = '';
    } finally {
        if (current === dbId) dbLoading.value = false;
    }
};

watch([selected, () => targets.value.join(',')], () => {
    runChecks();
    loadDatabases();
});

const onSelect = (row: Cluster.SyncResource | null) => {
    selected.value = row;
    dbHost.value = '';
};

watch(source, () => {
    onSelect(null);
    search.value = '';
    load(1);
});

const onExecute = () => {
    if (!selected.value) return;
    const app = selected.value;
    const keys = targets.value.filter((key) => checks.value[key]?.action === 'create');
    ElMessageBox.confirm(
        h(
            'div',
            keys.map((key) => h('div', `${app.name} / ${app.meta?.version} → ${nodeName(key)}`)),
        ),
        i18n.global.t('xpack.sync.app'),
        {
            confirmButtonText: i18n.global.t('commons.button.confirm'),
            cancelButtonText: i18n.global.t('commons.button.cancel'),
            type: 'info',
        },
    ).then(async () => {
        submitting.value = true;
        try {
            const res = await executeClusterSync({
                kind: 'app',
                sourceNodeId: nodeId(source.value),
                targetNodeIds: keys.map((key) => nodeId(key)),
                resourceKeys: [app.key],
                appDbHost: needsDB.value ? dbHost.value : undefined,
                overwriteConflicts: false,
            });
            MsgSuccess(i18n.global.t('xpack.sync.submitSuccess'));
            progressRef.value.open(res.data.taskId);
        } finally {
            submitting.value = false;
        }
    });
};

onMounted(async () => {
    await loadNodes();
    load(1);
});
</script>

<style scoped lang="scss">
.sync-desc {
    font-size: 12px;
    color: var(--el-text-color-secondary);
    margin-top: 4px;
}
.target-grid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(240px, 1fr));
    gap: 12px;
}
.target-item {
    height: auto;
    margin-right: 0 !important;
    padding: 8px 12px;
    :deep(.el-checkbox__label) {
        width: 100%;
        min-width: 0;
    }
}
.preview-row {
    display: flex;
    align-items: center;
    gap: 12px;
    padding: 8px 0;
    border-bottom: 1px dashed var(--el-border-color-lighter);
}
.preview-arrow {
    color: var(--el-color-primary);
}
.preview-reason {
    font-size: 12px;
    color: var(--el-text-color-secondary);
    max-width: 50%;
}
</style>

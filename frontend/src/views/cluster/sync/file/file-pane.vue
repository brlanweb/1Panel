<template>
    <el-card shadow="never" class="file-pane">
        <div class="flex flex-wrap items-center gap-2 mb-3">
            <el-select v-model="node" class="p-w-200" filterable :placeholder="$t('cluster.sourceNode')">
                <el-option
                    v-for="item in nodes"
                    :key="item.key"
                    :label="item.name"
                    :value="item.key"
                    :disabled="item.status === 'offline'"
                />
            </el-select>
            <div class="flex-1"></div>
            <TableSearch v-model:searchName="search" @search="load(1)" :disabled="!node" />
            <TableRefresh @search="load()" />
        </div>

        <div class="path-bar">
            <el-button link :disabled="!node" @click="go('/')">
                <el-icon><HomeFilled /></el-icon>
            </el-button>
            <el-input
                v-if="editing"
                ref="pathInputRef"
                v-model="pathInput"
                size="small"
                @keyup.enter="submitPath"
                @blur="editing = false"
            />
            <div v-else class="path-crumbs" @click="startEdit">
                <template v-for="(crumb, index) in crumbs" :key="crumb.path">
                    <span class="path-sep" v-if="index > 0">/</span>
                    <el-link underline="never" @click.stop="go(crumb.path)">{{ crumb.name }}</el-link>
                </template>
            </div>
        </div>

        <el-alert v-if="error" :title="error" type="error" show-icon :closable="false" class="mb-2" />

        <ComplexTable
            :cache-pagination="false"
            :pagination-config="pagination"
            :data="rows"
            :heightDiff="420"
            v-loading="loading"
            @search="load()"
        >
            <el-table-column :label="$t('commons.table.name')" min-width="220" show-overflow-tooltip>
                <template #default="{ row }">
                    <div class="file-name" :class="{ 'is-dir': isDir(row) }" @click="isDir(row) && go(row.key)">
                        <svg-icon v-if="isDir(row)" iconName="p-file-folder" className="file-icon" />
                        <svg-icon v-else :iconName="getIcon(extOf(row))" className="file-icon" />
                        <span>{{ row.name }}</span>
                        <span v-if="row.meta?.isSymlink" class="file-link">→ {{ row.meta?.linkPath }}</span>
                    </div>
                </template>
            </el-table-column>
            <el-table-column :label="$t('commons.table.operate')" width="110" align="center">
                <template #default="{ row }">
                    <el-tooltip :content="transferTip" :disabled="!!peerNode && peerNode !== node">
                        <el-button
                            link
                            type="primary"
                            :icon="side === 'left' ? Right : Back"
                            :disabled="!peerNode || peerNode === node"
                            @click="emit('transfer', row)"
                        />
                    </el-tooltip>
                    <el-button link type="danger" :icon="Close" @click="onDelete(row)" />
                </template>
            </el-table-column>
        </ComplexTable>
    </el-card>
</template>

<script lang="ts" setup>
import { computed, nextTick, reactive, ref, watch } from 'vue';
import { Back, Close, HomeFilled, Right } from '@element-plus/icons-vue';
import { ElMessageBox } from 'element-plus';
import i18n from '@/lang';
import { deleteClusterSyncFile, searchClusterSyncResources } from '@/api/modules/cluster';
import { Cluster } from '@/api/interface/cluster';
import { getIcon } from '@/utils/file';
import { MsgSuccess } from '@/utils/message';
import { SyncNode } from '../common';

const props = defineProps({
    side: { type: String as () => 'left' | 'right', required: true },
    nodes: { type: Array as () => SyncNode[], default: () => [] },
    modelValue: { type: String, default: '' },
    // node selected in the other pane (transfer target)
    peerNode: { type: String, default: '' },
    nodeId: { type: Function, required: true },
});
const emit = defineEmits(['update:modelValue', 'transfer']);

const PAGE_SIZE_KEY = 'exchange-page-size';
const pagination = reactive({
    currentPage: 1,
    pageSize: Number(localStorage.getItem(PAGE_SIZE_KEY)) || 50,
    total: 0,
});
watch(
    () => pagination.pageSize,
    (size) => localStorage.setItem(PAGE_SIZE_KEY, String(size)),
);

const node = computed({
    get: () => props.modelValue,
    set: (value: string) => emit('update:modelValue', value),
});
const path = ref('/');
const search = ref('');
const rows = ref<Cluster.SyncResource[]>([]);
const loading = ref(false);
const error = ref('');
const editing = ref(false);
const pathInput = ref('');
const pathInputRef = ref();

const transferTip = computed(() => i18n.global.t('xpack.sync.noSourceNode'));

const crumbs = computed(() => {
    // The root is the HomeFilled button; crumbs are the path segments.
    const list: { name: string; path: string }[] = [];
    let acc = '';
    for (const part of path.value.split('/').filter(Boolean)) {
        acc += '/' + part;
        list.push({ name: part, path: acc });
    }
    return list;
});

const isDir = (row: Cluster.SyncResource) => row.extra === 'dir';
const extOf = (row: Cluster.SyncResource) => {
    if (row.meta?.extension) return row.meta.extension;
    const idx = row.name.lastIndexOf('.');
    return idx > 0 ? row.name.substring(idx) : '';
};

let requestId = 0;
const load = async (page?: number) => {
    if (!node.value) {
        rows.value = [];
        pagination.total = 0;
        return;
    }
    const id = props.nodeId(node.value);
    if (id === undefined) return;
    if (page) pagination.currentPage = page;
    const current = ++requestId;
    loading.value = true;
    error.value = '';
    try {
        const res = await searchClusterSyncResources({
            kind: 'file',
            sourceNodeId: id,
            path: path.value,
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
        pagination.total = 0;
        error.value = err?.message || i18n.global.t('cluster.fetchError');
    } finally {
        if (current === requestId) loading.value = false;
    }
};

const go = (target: string) => {
    if (!node.value) return;
    path.value = target || '/';
    search.value = '';
    load(1);
};

const startEdit = () => {
    if (!node.value) return;
    pathInput.value = path.value;
    editing.value = true;
    nextTick(() => pathInputRef.value?.focus());
};
const submitPath = () => {
    let target = pathInput.value.trim() || '/';
    if (!target.startsWith('/')) target = '/' + target;
    if (target.length > 1) target = target.replace(/\/+$/, '');
    editing.value = false;
    go(target);
};

const onDelete = (row: Cluster.SyncResource) => {
    ElMessageBox.confirm(i18n.global.t('commons.msg.delete'), i18n.global.t('commons.button.delete'), {
        confirmButtonText: i18n.global.t('commons.button.confirm'),
        cancelButtonText: i18n.global.t('commons.button.cancel'),
        type: 'warning',
    }).then(async () => {
        await deleteClusterSyncFile({ nodeId: props.nodeId(node.value), path: row.key, isDir: isDir(row) });
        MsgSuccess(i18n.global.t('commons.msg.deleteSuccess'));
        load();
    });
};

watch(
    () => props.modelValue,
    () => {
        path.value = '/';
        search.value = '';
        load(1);
    },
);

defineExpose({ load, currentPath: () => path.value });
</script>

<style scoped lang="scss">
.file-pane {
    height: 100%;
}
.path-bar {
    display: flex;
    align-items: center;
    gap: 6px;
    padding: 6px 8px;
    margin-bottom: 8px;
    border: 1px solid var(--el-border-color-lighter);
    border-radius: 4px;
    min-height: 32px;
}
.path-crumbs {
    flex: 1;
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 2px;
    cursor: text;
    min-height: 20px;
}
.path-sep {
    color: var(--el-text-color-secondary);
}
.file-name {
    display: flex;
    align-items: center;
    gap: 6px;
    &.is-dir {
        cursor: pointer;
        color: var(--el-color-primary);
    }
}
.file-icon {
    font-size: 18px;
}
.file-link {
    color: var(--el-text-color-secondary);
    font-size: 12px;
}
</style>

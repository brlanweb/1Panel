<template>
    <DrawerPro v-model="visible" :header="$t('cluster.taskHistory')" size="large" @close="visible = false">
        <div class="flex justify-end mb-2">
            <TableRefresh @search="search()" />
        </div>
        <ComplexTable
            :cache-pagination="false"
            :pagination-config="pagination"
            :data="tasks"
            :heightDiff="260"
            @search="search()"
            v-loading="loading"
        >
            <el-table-column label="ID" prop="id" width="70" />
            <el-table-column :label="$t('cluster.type')" width="110">
                <template #default="{ row }">{{ kindLabel(row.kind) }}</template>
            </el-table-column>
            <el-table-column :label="$t('cluster.sourceNode')" prop="sourceName" min-width="120" />
            <el-table-column :label="$t('cluster.taskStatus')" width="100">
                <template #default="{ row }">
                    <el-tag :type="taskTagType(row.status)" size="small">{{ statusLabel(row.status) }}</el-tag>
                </template>
            </el-table-column>
            <el-table-column :label="$t('cluster.succeeded')" prop="succeeded" width="70" align="center" />
            <el-table-column :label="$t('cluster.skipped')" prop="skipped" width="70" align="center" />
            <el-table-column :label="$t('cluster.failed')" prop="failed" width="70" align="center" />
            <el-table-column :label="$t('cluster.createdAt')" width="170">
                <template #default="{ row }">{{ row.createdAt ? dateFormatSimple(row.createdAt) : '—' }}</template>
            </el-table-column>
            <el-table-column :label="$t('commons.table.operate')" width="90" fixed="right">
                <template #default="{ row }">
                    <el-button link type="primary" @click="progressRef.open(row.id, row)">
                        {{ $t('cluster.taskDetail') }}
                    </el-button>
                </template>
            </el-table-column>
        </ComplexTable>
        <TaskProgress ref="progressRef" @finished="search()" />
    </DrawerPro>
</template>

<script lang="ts" setup>
import { reactive, ref } from 'vue';
import i18n from '@/lang';
import { searchClusterSyncTasks } from '@/api/modules/cluster';
import { Cluster } from '@/api/interface/cluster';
import { dateFormatSimple } from '@/utils/date';
import TaskProgress from './task-progress.vue';

const visible = ref(false);
const loading = ref(false);
const tasks = ref<Cluster.SyncTask[]>([]);
const progressRef = ref();
const pagination = reactive({ currentPage: 1, pageSize: 20, total: 0 });

const kindLabel = (kind: string) => {
    switch (kind) {
        case 'file':
            return i18n.global.t('xpack.sync.file');
        case 'image':
            return i18n.global.t('xpack.sync.image');
        case 'app':
            return i18n.global.t('xpack.sync.app');
        case 'cert':
            return i18n.global.t('xpack.sync.ssl');
        case 'php':
            return i18n.global.t('xpack.sync.phpExtensionsTitle');
    }
    return kind;
};
const taskTagType = (status: string) => {
    if (status === 'success') return 'success';
    if (status === 'failed') return 'danger';
    if (status === 'partial' || status === 'running') return 'warning';
    return 'info';
};
const statusLabel = (status: string) => {
    const key = 'cluster.taskStatus_' + status;
    const text = i18n.global.t(key);
    return text === key ? status : text;
};

const search = async () => {
    loading.value = true;
    try {
        const res = await searchClusterSyncTasks({ page: pagination.currentPage, pageSize: pagination.pageSize });
        tasks.value = res.data.items || [];
        pagination.total = res.data.total;
    } catch {
        // reported by the interceptor
    } finally {
        loading.value = false;
    }
};

const open = () => {
    visible.value = true;
    search();
};

defineExpose({ open });
</script>

<template>
    <div v-loading="loading">
        <div v-if="!servedName" class="toolbar">
            <el-input
                v-model="info"
                class="p-w-250"
                clearable
                :placeholder="$t('compute.job.search')"
                @change="refresh(true)"
            />
        </div>
        <el-table :data="items" row-key="id" :empty-text="$t('compute.job.empty')" @row-click="openDetail">
            <el-table-column label="ID" prop="id" width="70" />
            <el-table-column :label="$t('compute.job.type')" width="120">
                <template #default="{ row }">{{ $t('compute.job.type_' + row.type) }}</template>
            </el-table-column>
            <el-table-column :label="$t('commons.table.status')" width="120">
                <template #default="{ row }">
                    <el-tag size="small" :type="jobStatusType(row.status)">
                        {{ $t('compute.job.status_' + row.status) }}
                    </el-tag>
                </template>
            </el-table-column>
            <el-table-column :label="$t('compute.servedName')" prop="servedName" min-width="170" />
            <el-table-column :label="$t('compute.job.source')" width="110">
                <template #default="{ row }">
                    <el-tag size="small" effect="plain" :type="row.source === 'auto' ? 'warning' : 'info'">
                        {{ $t('compute.job.source_' + (row.source || 'manual')) }}
                    </el-tag>
                </template>
            </el-table-column>
            <el-table-column :label="$t('compute.job.progress')" min-width="220">
                <template #default="{ row }">
                    <span class="sub">{{ lastStep(row) }}</span>
                </template>
            </el-table-column>
            <el-table-column :label="$t('compute.job.startAt')" width="170">
                <template #default="{ row }">{{ fmt(row.startedAt || row.createdAt) }}</template>
            </el-table-column>
            <el-table-column :label="$t('compute.job.duration')" width="100">
                <template #default="{ row }">{{ duration(row) }}</template>
            </el-table-column>
        </el-table>
        <el-pagination
            class="mt-3"
            layout="total, prev, pager, next"
            :total="total"
            :page-size="pageSize"
            v-model:current-page="page"
            @current-change="refresh(true)"
        />

        <DrawerPro
            v-model="detailVisible"
            :header="$t('compute.job.detail')"
            :resource="detail ? '#' + detail.id : ''"
            size="normal"
            @close="closeDetail"
        >
            <template v-if="detail">
                <div class="kv mb-3">
                    <span class="k">{{ $t('compute.job.type') }}</span>
                    <span>{{ $t('compute.job.type_' + detail.type) }}</span>
                    <span class="k">{{ $t('commons.table.status') }}</span>
                    <span>
                        <el-tag size="small" :type="jobStatusType(detail.status)">
                            {{ $t('compute.job.status_' + detail.status) }}
                        </el-tag>
                    </span>
                    <span class="k">{{ $t('compute.servedName') }}</span>
                    <span>{{ detail.servedName }}</span>
                    <span class="k">{{ $t('compute.job.source') }}</span>
                    <span>{{ $t('compute.job.source_' + (detail.source || 'manual')) }}</span>
                    <template v-if="detail.reason">
                        <span class="k">{{ $t('compute.job.reason') }}</span>
                        <span>{{ detail.reason }}</span>
                    </template>
                </div>
                <el-alert
                    v-if="detail.error"
                    type="error"
                    :closable="false"
                    show-icon
                    class="mb-3"
                    :title="detail.error"
                />
                <el-timeline>
                    <el-timeline-item
                        v-for="(s, i) in parseSteps(detail.steps)"
                        :key="i"
                        :type="stepType(s.status)"
                        :timestamp="s.finishedAt ? `${((s.finishedAt - s.startedAt) / 1000).toFixed(1)} s` : ''"
                        placement="top"
                    >
                        <div class="step">
                            <span class="strong">{{ s.name }}</span>
                            <el-icon v-if="s.status === 'running'" class="is-loading ml-2"><Loading /></el-icon>
                            <pre v-if="s.message">{{ s.message }}</pre>
                        </div>
                    </el-timeline-item>
                </el-timeline>
            </template>
        </DrawerPro>
    </div>
</template>

<script lang="ts" setup>
import { ref } from 'vue';
import { Loading } from '@element-plus/icons-vue';
import { Compute } from '@/api/interface/compute';
import { getComputeJob, searchComputeJobs } from '@/api/modules/compute';
import { jobStatusType, parseSteps } from '../helper';

// servedName shows the operation records of one model (pool detail).
const props = defineProps({ servedName: { type: String, default: '' } });
const loading = ref(false);
const items = ref<Compute.Job[]>([]);
const total = ref(0);
const page = ref(1);
const pageSize = 20;
const info = ref(props.servedName);
const detailVisible = ref(false);
const detail = ref<Compute.Job>();

const fmt = (s: string) => (s ? new Date(s).toLocaleString() : '—');
const duration = (row: Compute.Job) => {
    if (!row.startedAt) return '—';
    const end = row.finishedAt ? new Date(row.finishedAt).getTime() : Date.now();
    const sec = Math.max(0, Math.round((end - new Date(row.startedAt).getTime()) / 1000));
    return sec >= 60 ? `${Math.floor(sec / 60)}m ${sec % 60}s` : `${sec}s`;
};
const lastStep = (row: Compute.Job) => {
    const steps = parseSteps(row.steps);
    if (!steps.length) return row.error || '—';
    const s = steps[steps.length - 1];
    return `${steps.length} · ${s.name}`;
};
const stepType = (s: string) =>
    s === 'success' ? 'success' : s === 'failed' ? 'danger' : s === 'running' ? 'primary' : 'info';

// Running jobs are polled (list + open detail); finished history is static.
const refresh = async (force = false) => {
    if (force) loading.value = !items.value.length;
    try {
        const res = await searchComputeJobs({ page: page.value, pageSize, info: info.value });
        items.value = res.data.items || [];
        total.value = res.data.total;
        if (detailVisible.value && detail.value && detail.value.status === 'running') {
            const d = await getComputeJob(detail.value.id);
            detail.value = d.data;
        }
    } finally {
        loading.value = false;
    }
};

const openDetail = async (row: Compute.Job) => {
    detail.value = row;
    detailVisible.value = true;
    const d = await getComputeJob(row.id);
    detail.value = d.data;
};
const closeDetail = () => {
    detailVisible.value = false;
    detail.value = undefined;
};

defineExpose({ refresh });
</script>

<style scoped src="../style.css"></style>

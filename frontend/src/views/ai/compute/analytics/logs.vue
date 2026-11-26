<template>
    <el-card shadow="never" class="mt-3">
        <template #header>
            <div class="card-head">
                <span>
                    <span class="strong">{{ $t('compute.ana.logs') }}</span>
                    <span class="sub ml-2">{{ $t('compute.ana.logsHelper', [retention]) }}</span>
                </span>
                <el-radio-group v-model="status" size="small" @change="load(1)">
                    <el-radio-button value="">{{ $t('commons.table.all') }}</el-radio-button>
                    <el-radio-button value="success">{{ $t('compute.ana.ok') }}</el-radio-button>
                    <el-radio-button value="failed">{{ $t('compute.ana.failed') }}</el-radio-button>
                </el-radio-group>
            </div>
        </template>
        <el-table :data="items" size="small" :empty-text="$t('compute.ana.noData')">
            <el-table-column :label="$t('compute.ana.time')" width="170">
                <template #default="{ row }">{{ time(row.time) }}</template>
            </el-table-column>
            <el-table-column :label="$t('compute.ana.consumer')" prop="consumer" min-width="130" />
            <el-table-column :label="$t('compute.ana.model')" min-width="210">
                <template #default="{ row }">
                    <div>{{ row.model || '—' }}</div>
                    <div class="sub">
                        {{ row.instance || '' }}
                        <el-tag v-if="row.stream" size="small" effect="plain">stream</el-tag>
                        <el-tag v-if="row.retries" size="small" type="warning" effect="plain">
                            {{ $t('compute.ana.retried', [row.retries]) }}
                        </el-tag>
                    </div>
                </template>
            </el-table-column>
            <el-table-column :label="$t('commons.table.status')" width="90">
                <template #default="{ row }">
                    <el-tooltip :disabled="!row.error" :content="row.error" placement="top">
                        <el-tag size="small" :type="row.success ? 'success' : 'danger'">
                            {{ row.status || 'ERR' }}
                        </el-tag>
                    </el-tooltip>
                </template>
            </el-table-column>
            <el-table-column label="Token" width="150">
                <template #default="{ row }">
                    {{ fmt(row.promptTokens + row.completionTokens) }}
                    <div class="sub">{{ fmt(row.promptTokens) }} / {{ fmt(row.completionTokens) }}</div>
                </template>
            </el-table-column>
            <el-table-column :label="$t('compute.ana.ttft')" width="110">
                <template #default="{ row }">{{ ms(row.ttftMs) }}</template>
            </el-table-column>
            <el-table-column :label="$t('compute.ana.latency')" width="110">
                <template #default="{ row }">{{ ms(row.latencyMs) }}</template>
            </el-table-column>
            <el-table-column :label="$t('compute.ana.speed')" width="110">
                <template #default="{ row }">{{ speed(row) }}</template>
            </el-table-column>
            <el-table-column label="IP" prop="clientIp" width="130" />
        </el-table>
        <el-pagination
            class="mt-2 pager"
            layout="total, sizes, prev, pager, next"
            :total="total"
            :page-size="size"
            :current-page="page"
            :page-sizes="[20, 50, 100]"
            @current-change="load"
            @size-change="onSize"
        />
    </el-card>
</template>

<script lang="ts" setup>
import { ref } from 'vue';
import { Compute } from '@/api/interface/compute';
import { searchComputeCallLogs } from '@/api/modules/compute';
import { ms } from '../helper';
import { fmtCount as fmt } from './format';

const props = defineProps<{ model: string; consumer: string; start: number; retention: number }>();
const items = ref<Compute.CallLog[]>([]);
const total = ref(0);
const page = ref(1);
const size = ref(20);
const status = ref<'' | 'success' | 'failed'>('');

const pad = (n: number) => String(n).padStart(2, '0');
const time = (ts: number) => {
    const d = new Date(ts);
    return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}`;
};
// Generation speed: output tokens over the time after the first token.
const speed = (row: Compute.CallLog) => {
    const gen = row.latencyMs - row.ttftMs;
    if (!row.completionTokens || gen <= 0) return '—';
    return `${(row.completionTokens / (gen / 1000)).toFixed(1)} t/s`;
};

const load = async (p = page.value) => {
    page.value = p;
    const res = await searchComputeCallLogs({
        page: p,
        pageSize: size.value,
        model: props.model,
        consumer: props.consumer,
        status: status.value,
        start: props.start,
    });
    items.value = res.data?.items || [];
    total.value = res.data?.total || 0;
};
const onSize = (n: number) => {
    size.value = n;
    load(1);
};

defineExpose({ load });
</script>

<style scoped src="../style.css"></style>
<style scoped>
.pager {
    justify-content: flex-end;
}
</style>

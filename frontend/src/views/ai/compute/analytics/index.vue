<template>
    <div>
        <RouterButton :buttons="buttons" />
        <LayoutContent v-loading="loading" :title="$t('compute.ana.title')">
            <template #leftToolBar>
                <el-radio-group v-model="range" @change="reload">
                    <el-radio-button v-for="r in ranges" :key="r" :value="r">{{ rangeLabel(r) }}</el-radio-button>
                </el-radio-group>
            </template>
            <template #rightToolBar>
                <el-select v-model="model" clearable filterable class="filter" @change="reload">
                    <template #prefix>{{ $t('compute.ana.model') }}</template>
                    <el-option v-for="m in data?.models || []" :key="m" :label="m" :value="m" />
                </el-select>
                <el-select v-model="consumer" clearable filterable class="filter" @change="reload">
                    <template #prefix>{{ $t('compute.ana.consumer') }}</template>
                    <el-option v-for="c in data?.consumers || []" :key="c" :label="c" :value="c" />
                </el-select>
                <el-switch v-model="auto" class="ml-2" :active-text="$t('compute.ana.autoRefresh')" />
                <el-button class="ml-2" :icon="Refresh" @click="reload" />
            </template>
            <template #prompt>
                <el-alert type="info" :closable="false" :title="$t('compute.ana.helper', [retention])" />
            </template>
            <template #main>
                <div class="kpis">
                    <div v-for="k in kpis" :key="k.label" class="kpi" :title="k.title || k.sub">
                        <div class="kpi-label">{{ k.label }}</div>
                        <div class="kpi-value" :class="k.cls">{{ k.value }}</div>
                        <div class="kpi-sub">{{ k.sub }}</div>
                    </div>
                </div>
                <el-row :gutter="12" class="mt-3">
                    <el-col :xs="24" :md="9">
                        <el-card shadow="never">
                            <template #header>
                                <div class="card-head">
                                    <span class="strong">{{ $t('compute.ana.distribution') }}</span>
                                    <el-radio-group v-model="shareBy" size="small">
                                        <el-radio-button value="model">{{ $t('compute.ana.byModel') }}</el-radio-button>
                                        <el-radio-button value="consumer">
                                            {{ $t('compute.ana.byConsumer') }}
                                        </el-radio-button>
                                    </el-radio-group>
                                </div>
                            </template>
                            <Chart v-if="shares.length" :option="pieOption" height="300px" />
                            <el-empty v-else :image-size="80" :description="$t('compute.ana.noData')" />
                        </el-card>
                    </el-col>
                    <el-col :xs="24" :md="15">
                        <el-card shadow="never">
                            <template #header>
                                <div class="card-head">
                                    <span class="strong">{{ $t('compute.ana.modelCalls') }}</span>
                                    <el-radio-group v-model="trendMetric" size="small">
                                        <el-radio-button value="tokens">Token</el-radio-button>
                                        <el-radio-button value="requests">
                                            {{ $t('compute.ana.calls') }}
                                        </el-radio-button>
                                        <el-radio-button value="latency">
                                            {{ $t('compute.ana.latency') }}
                                        </el-radio-button>
                                    </el-radio-group>
                                </div>
                            </template>
                            <Chart v-if="buckets.length" :option="trendOption" height="300px" />
                            <el-empty v-else :image-size="80" :description="$t('compute.ana.noData')" />
                        </el-card>
                    </el-col>
                </el-row>
                <el-card shadow="never" class="mt-3">
                    <template #header>
                        <span class="strong">{{ $t('compute.ana.detailBy', [shareLabel]) }}</span>
                    </template>
                    <el-table :data="shares" size="small" :empty-text="$t('compute.ana.noData')">
                        <el-table-column :label="shareLabel" prop="name" min-width="200" />
                        <el-table-column :label="$t('compute.ana.calls')" width="120">
                            <template #default="{ row }">{{ fmt(row.requests) }}</template>
                        </el-table-column>
                        <el-table-column label="Token" width="140">
                            <template #default="{ row }">{{ fmt(row.tokens) }}</template>
                        </el-table-column>
                        <el-table-column :label="$t('compute.ana.share')" min-width="160">
                            <template #default="{ row }">
                                <el-progress :percentage="sharePct(row)" :stroke-width="8" />
                            </template>
                        </el-table-column>
                        <el-table-column :label="$t('compute.ana.successRate')" width="110">
                            <template #default="{ row }">
                                <span :class="rate(row) < 95 ? 'stat-warning' : ''">{{ rate(row).toFixed(1) }}%</span>
                            </template>
                        </el-table-column>
                        <el-table-column :label="$t('compute.ana.ttft')" width="120">
                            <template #default="{ row }">{{ ms(row.avgTtftMs) }}</template>
                        </el-table-column>
                    </el-table>
                </el-card>
                <CallLogs
                    ref="logsRef"
                    :model="model"
                    :consumer="consumer"
                    :start="windowStart"
                    :retention="retention"
                />
            </template>
        </LayoutContent>
    </div>
</template>

<script lang="ts" setup>
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue';
import { Refresh } from '@element-plus/icons-vue';
import i18n from '@/lang';
import { Compute } from '@/api/interface/compute';
import { loadComputeAnalytics } from '@/api/modules/compute';
import { ms, t } from '../helper';
import { fmtCount as fmt } from './format';
import Chart from './chart.vue';
import CallLogs from './logs.vue';

const buttons = computed(() => [
    { label: i18n.global.t('aiTools.model.services'), path: '/ai/model-services' },
    { label: i18n.global.t('compute.pool.title'), path: '/ai/model-services/pools' },
    { label: i18n.global.t('compute.ana.title'), path: '/ai/model-services/analytics' },
]);

const ranges = [1, 6, 24, 48, 72];
const range = ref(24);
const model = ref('');
const consumer = ref('');
const auto = ref(true);
const loading = ref(false);
const data = ref<Compute.Analytics>();
const shareBy = ref<'model' | 'consumer'>('model');
const trendMetric = ref<'tokens' | 'requests' | 'latency'>('tokens');
const logsRef = ref();
const windowStart = ref(0);
let timer: ReturnType<typeof setInterval> | undefined;

const retention = computed(() => data.value?.retentionHours || 24);
const rangeLabel = (h: number) => (h < 24 ? t('compute.ana.hours', [h]) : t('compute.ana.days', [h / 24]));

const kpis = computed(() => {
    const s = data.value?.summary;
    if (!s) return [];
    const rateCls = s.requests ? (s.successRate < 95 ? 'stat-danger' : 'stat-success') : '';
    // Totals are persisted (all time); the window figures come from memory.
    const tot = data.value?.totals;
    const win = rangeLabel(range.value);
    return [
        {
            label: t('compute.ana.totalCalls'),
            value: fmt(tot ? tot.requests : s.requests),
            sub: tot
                ? `${t('compute.ana.today')} ${fmt(tot.todayRequests)} · ${win} ${fmt(s.requests)} · ${t('compute.ana.failures', [fmt(tot.failures)])}`
                : t('compute.ana.failures', [fmt(s.failures)]),
            title: tot?.since ? t('compute.ana.since', [tot.since]) : '',
        },
        {
            label: t('compute.ana.totalTokens'),
            value: fmt(tot ? tot.totalTokens : s.totalTokens),
            sub: tot
                ? `${t('compute.ana.today')} ${fmt(tot.todayTokens)} · ${win} ${fmt(s.totalTokens)} · ${t('compute.ana.input')} ${fmt(tot.promptTokens)} / ${t('compute.ana.output')} ${fmt(tot.completionTokens)}`
                : `${t('compute.ana.input')} ${fmt(s.promptTokens)} · ${t('compute.ana.output')} ${fmt(s.completionTokens)}`,
            title: tot?.since ? t('compute.ana.since', [tot.since]) : '',
        },
        {
            label: t('compute.ana.avgRpm'),
            value: s.avgRpm.toFixed(s.avgRpm < 10 ? 2 : 1),
            sub: t('compute.ana.rpmHelper'),
        },
        {
            label: t('compute.ana.successRate'),
            value: s.requests ? s.successRate.toFixed(2) + '%' : '—',
            sub: `${fmt(s.requests - s.failures)} / ${fmt(s.requests)}`,
            cls: rateCls,
        },
        { label: t('compute.ana.ttft'), value: ms(s.avgTtftMs), sub: t('compute.ana.ttftHelper') },
        { label: t('compute.ana.avgLatency'), value: ms(s.avgLatencyMs), sub: t('compute.ana.latencyHelper') },
    ];
});

const shares = computed(() => (shareBy.value === 'model' ? data.value?.byModel : data.value?.byConsumer) || []);
const shareLabel = computed(() => (shareBy.value === 'model' ? t('compute.ana.model') : t('compute.ana.consumer')));
const shareTotal = computed(() => shares.value.reduce((a, s) => a + (s.tokens || s.requests), 0));
const sharePct = (row: Compute.AnalyticsShare) =>
    shareTotal.value ? Number((((row.tokens || row.requests) * 100) / shareTotal.value).toFixed(1)) : 0;
const rate = (row: Compute.AnalyticsShare) =>
    row.requests ? ((row.requests - row.failures) * 100) / row.requests : 100;

const pieOption = computed(() => ({
    tooltip: {
        trigger: 'item',
        formatter: (p: any) =>
            `${p.name}<br/>Token ${fmt(p.data.tokens)} (${p.percent}%)<br/>${t('compute.ana.calls')} ${fmt(p.data.requests)}`,
    },
    legend: { bottom: 0, left: 'center' },
    series: [
        {
            type: 'pie',
            radius: ['45%', '70%'],
            center: ['50%', '45%'],
            itemStyle: { borderRadius: 4 },
            label: { show: false },
            emphasis: { label: { show: true, fontSize: 13, fontWeight: 'bold', formatter: '{b}\n{d}%' } },
            data: shares.value.map((s) => ({
                name: s.name || '—',
                value: s.tokens || s.requests,
                tokens: s.tokens,
                requests: s.requests,
            })),
        },
    ],
}));

const buckets = computed(() => data.value?.buckets || []);
const axisLabel = (ts: number) => {
    const d = new Date(ts);
    const hm = `${String(d.getHours()).padStart(2, '0')}:${String(d.getMinutes()).padStart(2, '0')}`;
    return range.value > 24 ? `${d.getMonth() + 1}-${d.getDate()} ${hm}` : hm;
};

const tokenSeries = () => {
    const totals: Record<string, number> = {};
    for (const b of buckets.value) {
        for (const [m, v] of Object.entries(b.byModel || {})) totals[m] = (totals[m] || 0) + v;
    }
    const names = Object.keys(totals).sort((a, b) => totals[b] - totals[a]);
    const top = names.slice(0, 8);
    const bar = (name: string, values: number[]) => ({
        name,
        type: 'bar',
        stack: 'tokens',
        barMaxWidth: 24,
        data: values,
    });
    const series = top.map((m) =>
        bar(
            m,
            buckets.value.map((b) => b.byModel?.[m] || 0),
        ),
    );
    if (names.length > 8) {
        series.push(
            bar(
                t('compute.ana.other'),
                buckets.value.map((b) =>
                    Object.entries(b.byModel || {})
                        .filter(([m]) => !top.includes(m))
                        .reduce((a, [, v]) => a + v, 0),
                ),
            ),
        );
    }
    return series;
};

const trendOption = computed(() => {
    const x = buckets.value.map((b) => axisLabel(b.time));
    const grid = { left: 56, right: 50, top: 40, bottom: 30 };
    const base = { legend: { top: 0 }, grid, xAxis: { type: 'category', data: x } };
    if (trendMetric.value === 'tokens') {
        return {
            ...base,
            tooltip: { trigger: 'axis', axisPointer: { type: 'shadow' }, valueFormatter: (v: number) => fmt(v) },
            yAxis: { type: 'value', name: 'Token', axisLabel: { formatter: (v: number) => fmt(v) } },
            series: tokenSeries(),
        };
    }
    if (trendMetric.value === 'requests') {
        return {
            ...base,
            tooltip: { trigger: 'axis' },
            yAxis: [
                { type: 'value', name: t('compute.ana.calls'), minInterval: 1 },
                { type: 'value', name: '%', min: 0, max: 100, splitLine: { show: false } },
            ],
            series: [
                {
                    name: t('compute.ana.ok'),
                    type: 'bar',
                    stack: 'req',
                    barMaxWidth: 24,
                    data: buckets.value.map((b) => b.requests - b.failures),
                },
                {
                    name: t('compute.ana.failed'),
                    type: 'bar',
                    stack: 'req',
                    barMaxWidth: 24,
                    itemStyle: { color: '#ef4444' },
                    data: buckets.value.map((b) => b.failures),
                },
                {
                    name: t('compute.ana.successRate'),
                    type: 'line',
                    yAxisIndex: 1,
                    smooth: true,
                    showSymbol: false,
                    connectNulls: true,
                    data: buckets.value.map((b) =>
                        b.requests ? Number((((b.requests - b.failures) * 100) / b.requests).toFixed(1)) : null,
                    ),
                },
            ],
        };
    }
    const line = (name: string, values: (number | null)[], area = false) => ({
        name,
        type: 'line',
        smooth: true,
        showSymbol: false,
        connectNulls: true,
        ...(area ? { areaStyle: { opacity: 0.12 } } : {}),
        data: values,
    });
    return {
        ...base,
        tooltip: { trigger: 'axis', valueFormatter: (v: number) => (v == null ? '—' : ms(v)) },
        yAxis: { type: 'value', name: 'ms' },
        series: [
            line(
                t('compute.ana.ttft'),
                buckets.value.map((b) => (b.requests ? Math.round(b.avgTtftMs) : null)),
                true,
            ),
            line(
                t('compute.ana.avgLatency'),
                buckets.value.map((b) => (b.requests ? Math.round(b.avgLatencyMs) : null)),
            ),
        ],
    };
});

const loadAnalytics = async () => {
    const end = Date.now();
    windowStart.value = end - range.value * 3600 * 1000;
    const res = await loadComputeAnalytics({
        start: windowStart.value,
        end,
        model: model.value,
        consumer: consumer.value,
    });
    data.value = res.data;
};

const reload = async () => {
    loading.value = true;
    try {
        await loadAnalytics();
        await logsRef.value?.load(1);
    } finally {
        loading.value = false;
    }
};

// Quiet refresh: no spinner, keeps the log page.
const refreshQuiet = async () => {
    if (document.hidden) return;
    await loadAnalytics();
    await logsRef.value?.load();
};

watch(
    auto,
    (on) => {
        if (timer) clearInterval(timer);
        timer = on ? setInterval(refreshQuiet, 15000) : undefined;
    },
    { immediate: true },
);

onMounted(reload);
onBeforeUnmount(() => timer && clearInterval(timer));
</script>

<style scoped src="../style.css"></style>
<style scoped>
.filter {
    width: 210px;
    margin-left: 8px;
}
.kpis {
    display: grid;
    grid-template-columns: repeat(6, minmax(0, 1fr));
    gap: 12px;
}
@media (max-width: 1200px) {
    .kpis {
        grid-template-columns: repeat(3, minmax(0, 1fr));
    }
}
@media (max-width: 640px) {
    .kpis {
        grid-template-columns: repeat(2, minmax(0, 1fr));
    }
}
.kpi {
    padding: 14px 16px;
    border: 1px solid var(--el-border-color-lighter);
    border-radius: 8px;
    background: var(--el-bg-color);
}
.kpi-label {
    font-size: 13px;
    color: var(--el-text-color-secondary);
}
.kpi-value {
    margin-top: 6px;
    font-size: 26px;
    font-weight: 600;
    line-height: 1.2;
    color: var(--el-text-color-primary);
    font-variant-numeric: tabular-nums;
}
.kpi-sub {
    margin-top: 4px;
    font-size: 12px;
    color: var(--el-text-color-secondary);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
}
</style>

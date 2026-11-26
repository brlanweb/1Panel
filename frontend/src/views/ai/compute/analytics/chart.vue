<template>
    <div ref="el" :style="{ height, width: '100%' }" />
</template>

<script lang="ts" setup>
import { onBeforeUnmount, onMounted, ref, watch } from 'vue';
import echarts from '@/utils/echarts';
import { loadChartPalette } from '@/utils/market-theme';

// Thin echarts wrapper: the caller passes a full option; text, grid and
// series colors follow the panel theme and are re-applied when the theme
// (class / style of <html>) changes.
const props = withDefaults(defineProps<{ option: Record<string, any>; height?: string }>(), { height: '280px' });
const el = ref<HTMLElement>();
let chart: ReturnType<typeof echarts.init> | undefined;
let resize: ResizeObserver | undefined;
let themeWatch: MutationObserver | undefined;

const css = (name: string, fallback: string) =>
    getComputedStyle(document.documentElement).getPropertyValue(name).trim() || fallback;

// Extra colors after the theme palette for charts with many models.
const EXTRA = ['#0ea5e9', '#10b981', '#a855f7', '#f97316', '#14b8a6', '#ec4899'];

const render = () => {
    if (!el.value) return;
    if (!chart) chart = echarts.init(el.value);
    const text = css('--el-text-color-secondary', '#909399');
    const border = css('--el-border-color-lighter', '#ebeef5');
    const bg = css('--el-bg-color-overlay', '#ffffff');
    const primary = css('--el-text-color-primary', '#303133');
    const axis = (a: any) =>
        a && {
            ...a,
            axisLabel: { color: text, ...a.axisLabel },
            nameTextStyle: { color: text, ...a.nameTextStyle },
            axisLine: { lineStyle: { color: border }, ...a.axisLine },
            splitLine: { lineStyle: { color: border, type: 'dashed' }, ...a.splitLine },
        };
    const mapAxis = (a: any) => (Array.isArray(a) ? a.map(axis) : axis(a));
    const opt = props.option;
    chart.setOption(
        {
            color: [...loadChartPalette(), ...EXTRA],
            textStyle: { color: primary },
            tooltip: {
                backgroundColor: bg,
                borderColor: border,
                textStyle: { color: primary },
                ...opt.tooltip,
            },
            legend: opt.legend && { textStyle: { color: text }, type: 'scroll', ...opt.legend },
            ...opt,
            ...(opt.xAxis ? { xAxis: mapAxis(opt.xAxis) } : {}),
            ...(opt.yAxis ? { yAxis: mapAxis(opt.yAxis) } : {}),
        },
        true,
    );
};

watch(() => props.option, render, { deep: true });

onMounted(() => {
    render();
    resize = new ResizeObserver(() => chart?.resize());
    if (el.value) resize.observe(el.value);
    themeWatch = new MutationObserver(() => requestAnimationFrame(render));
    themeWatch.observe(document.documentElement, { attributes: true, attributeFilter: ['class', 'style'] });
});

onBeforeUnmount(() => {
    resize?.disconnect();
    themeWatch?.disconnect();
    chart?.dispose();
    chart = undefined;
});
</script>

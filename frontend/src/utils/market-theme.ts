import { loadShanghaiComposite } from '@/api/modules/setting';

// Shanghai Composite decides the panel accent: red when it is up, ginkgo
// gold when it is down or flat. It is read once per page load (login and
// entering the panel both reload the page), never on a timer.

interface MarketPalette {
    // Accent used for buttons, menus, links, favicon.
    accent: string;
    // Line chart series colors, first one is always the accent.
    chart: string[];
}

// Ginkgo: autumn leaf gold, paired with ink blue / bark brown / slate so
// multi-series charts stay readable without fighting the accent.
export const MARKET_THEME_GINKGO = '#b07d12';
// Red: crimson, paired with warm amber / indigo / slate instead of the old
// teal + green + pink mix that clashed with it.
export const MARKET_THEME_RED = '#e11d48';

const PALETTES: Record<'ginkgo' | 'red', { light: MarketPalette; dark: MarketPalette }> = {
    ginkgo: {
        light: { accent: MARKET_THEME_GINKGO, chart: [MARKET_THEME_GINKGO, '#2f6fb0', '#8c5a2b', '#7a8899'] },
        dark: { accent: '#e3b341', chart: ['#e3b341', '#6aa5e8', '#c99567', '#9aa7b6'] },
    },
    red: {
        light: { accent: MARKET_THEME_RED, chart: [MARKET_THEME_RED, '#f59e0b', '#6366f1', '#64748b'] },
        dark: { accent: '#fb7185', chart: ['#fb7185', '#fbbf24', '#818cf8', '#94a3b8'] },
    },
};

const parseHex = (value: string) => {
    const normalized = value.replace('#', '');
    return [0, 2, 4].map((index) => parseInt(normalized.slice(index, index + 2), 16));
};

export const hexToRgba = (hex: string, alpha: number) => {
    const [r, g, b] = parseHex(hex);
    return `rgba(${r}, ${g}, ${b}, ${alpha})`;
};

const mix = (hex: string, target: string, weight: number) => {
    const [r1, g1, b1] = parseHex(hex);
    const [r2, g2, b2] = parseHex(target);
    const channel = (from: number, to: number) => Math.round(from + (to - from) * weight);
    return [channel(r1, r2), channel(g1, g2), channel(b1, b2)]
        .map((item) => item.toString(16).padStart(2, '0'))
        .join('');
};

const lightScale = (hex: string) => ({
    '--panel-color-primary': hex,
    '--panel-color-primary-light-8': `#${mix(hex, 'ffffff', 0.15)}`,
    '--panel-color-primary-light-1': `#${mix(hex, 'ffffff', 0.15)}`,
    '--panel-color-primary-light-2': `#${mix(hex, 'ffffff', 0.3)}`,
    '--panel-color-primary-light-3': `#${mix(hex, 'ffffff', 0.42)}`,
    '--panel-color-primary-light-4': `#${mix(hex, 'ffffff', 0.55)}`,
    '--panel-color-primary-light-5': `#${mix(hex, 'ffffff', 0.68)}`,
    '--panel-color-primary-light-6': `#${mix(hex, 'ffffff', 0.78)}`,
    '--panel-color-primary-light-7': `#${mix(hex, 'ffffff', 0.86)}`,
    '--panel-color-primary-light-9': `#${mix(hex, 'ffffff', 0.94)}`,
});

const darkScale = (hex: string) => ({
    '--panel-color-primary': hex,
    '--panel-color-primary-light-8': `#${mix(hex, '0f172a', 0.15)}`,
    '--panel-color-primary-light-1': `#${mix(hex, 'ffffff', 0.35)}`,
    '--panel-color-primary-light-2': `#${mix(hex, '0f172a', 0.25)}`,
    '--panel-color-primary-light-3': `#${mix(hex, '0f172a', 0.38)}`,
    '--panel-color-primary-light-4': `#${mix(hex, '0f172a', 0.5)}`,
    '--panel-color-primary-light-5': `#${mix(hex, '0f172a', 0.68)}`,
    '--panel-color-primary-light-6': `#${mix(hex, '0f172a', 0.58)}`,
    '--panel-color-primary-light-7': `#${mix(hex, '0f172a', 0.72)}`,
    '--panel-color-primary-light-9': `#${mix(hex, '0f172a', 0.84)}`,
});

const applyScale = (scale: Record<string, string>, palette: MarketPalette) => {
    const root = document.documentElement;
    const accent = palette.accent;
    Object.entries(scale).forEach(([name, value]) => root.style.setProperty(name, value));
    const [r, g, b] = parseHex(accent);
    root.style.setProperty('--panel-accent-rgb', `${r}, ${g}, ${b}`);
    root.style.setProperty('--el-box-shadow-light', `0px 0px 4px rgba(${r}, ${g}, ${b}, 0.1)`);
    root.style.setProperty('--panel-menu-bg-color', `rgba(${r}, ${g}, ${b}, 0.1)`);
    root.style.setProperty('--panel-alert-bg-color', `rgba(${r}, ${g}, ${b}, 0.03)`);
    root.style.setProperty('--panel-chart-colors', palette.chart.join(','));
    root.style.setProperty('--market-accent', accent);
};

export const applyMarketTheme = (rising: boolean) => {
    const dark = document.documentElement.classList.contains('dark');
    const palette = PALETTES[rising ? 'red' : 'ginkgo'][dark ? 'dark' : 'light'];
    applyScale(dark ? darkScale(palette.accent) : lightScale(palette.accent), palette);
};

// Chart series colors of the active market theme. Read at render time so
// charts follow the theme even when they mounted before it was applied.
export const loadChartPalette = (): string[] => {
    const value = getComputedStyle(document.documentElement).getPropertyValue('--panel-chart-colors').trim();
    const colors = value
        .split(',')
        .map((item) => item.trim())
        .filter((item) => /^#[0-9a-fA-F]{6}$/.test(item));
    return colors.length ? colors : PALETTES.ginkgo.light.chart;
};

// Rising only when today's close/last price is above yesterday's close.
// A failed request keeps the default (ginkgo) theme instead of guessing.
export const loadShanghaiCompositeRising = async (): Promise<boolean> => {
    try {
        const res = await loadShanghaiComposite();
        return res.data?.rising === true;
    } catch {
        return false;
    }
};

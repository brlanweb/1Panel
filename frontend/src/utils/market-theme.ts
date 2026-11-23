import { loadShanghaiComposite } from '@/api/modules/setting';

// Shanghai Composite decides the panel accent: red when it is up, the
// original green when it is down or flat. It is read once per page load
// (login and entering the panel both reload the page), never on a timer.

export const MARKET_THEME_GREEN = '#0d9488';
export const MARKET_THEME_RED = '#e11d48';

const mix = (hex: string, target: string, weight: number) => {
    const parse = (value: string) => {
        const normalized = value.replace('#', '');
        return [0, 2, 4].map((index) => parseInt(normalized.slice(index, index + 2), 16));
    };
    const [r1, g1, b1] = parse(hex);
    const [r2, g2, b2] = parse(target);
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

const applyScale = (scale: Record<string, string>, accent: string) => {
    const root = document.documentElement;
    Object.entries(scale).forEach(([name, value]) => root.style.setProperty(name, value));
    const [r, g, b] = accent
        .replace('#', '')
        .match(/.{2}/g)!
        .map((item) => parseInt(item, 16));
    root.style.setProperty('--el-box-shadow-light', `0px 0px 4px rgba(${r}, ${g}, ${b}, 0.1)`);
    root.style.setProperty('--panel-menu-bg-color', `rgba(${r}, ${g}, ${b}, 0.1)`);
    root.style.setProperty('--panel-alert-bg-color', `rgba(${r}, ${g}, ${b}, 0.03)`);
    root.style.setProperty('--market-accent', accent);
};

export const applyMarketTheme = (rising: boolean) => {
    const dark = document.documentElement.classList.contains('dark');
    const accent = rising ? MARKET_THEME_RED : MARKET_THEME_GREEN;
    applyScale(dark ? darkScale(accent) : lightScale(accent), accent);
};

// Rising only when today's close/last price is above yesterday's close.
// A failed request keeps the current (green) theme instead of guessing.
export const loadShanghaiCompositeRising = async (): Promise<boolean> => {
    try {
        const res = await loadShanghaiComposite();
        return res.data?.rising === true;
    } catch {
        return false;
    }
};

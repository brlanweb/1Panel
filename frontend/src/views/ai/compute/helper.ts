import i18n from '@/lang';
import { computeSize } from '@/utils/size';

export const t = (key: string, args?: any) => i18n.global.t(key, args);

export const pct = (v: number) => Math.min(100, Math.max(0, Number((v || 0).toFixed(1))));

export const barColor = (v: number) => {
    if (v >= 90) return 'var(--el-color-danger)';
    if (v >= 80) return 'var(--el-color-warning)';
    return 'var(--el-color-primary)';
};

export const tempColor = (v: number) => {
    if (v >= 90) return 'var(--el-color-danger)';
    if (v >= 80) return 'var(--el-color-warning)';
    return '';
};

export const size = (b: number) => (b ? computeSize(Math.round(b)) : '0 B');
export const rate = (b: number) => `${computeSize(Math.round(b || 0))}/s`;

export const num = (v: number, digits = 1) => {
    if (!v) return '0';
    if (v >= 1e6) return (v / 1e6).toFixed(digits) + 'M';
    if (v >= 1e3) return (v / 1e3).toFixed(digits) + 'k';
    return Number(v.toFixed(digits)).toString();
};

export const ms = (v: number) => {
    if (!v) return '—';
    if (v >= 1000) return (v / 1000).toFixed(2) + ' s';
    return Math.round(v) + ' ms';
};

export const ago = (ts: number | string) => {
    if (!ts) return '—';
    const time = typeof ts === 'number' ? ts : new Date(ts).getTime();
    if (!time || time < 0) return '—';
    const sec = Math.max(0, Math.floor((Date.now() - time) / 1000));
    if (sec < 60) return t('compute.secondsAgo', [sec]);
    if (sec < 3600) return t('compute.minutesAgo', [Math.floor(sec / 60)]);
    if (sec < 86400) return t('compute.hoursAgo', [Math.floor(sec / 3600)]);
    return t('compute.daysAgo', [Math.floor(sec / 86400)]);
};

export const instStatusType = (s: string) => {
    switch (s) {
        case 'running':
            return 'success';
        case 'starting':
        case 'draining':
            return 'warning';
        case 'failed':
            return 'danger';
        default:
            return 'info';
    }
};

export const jobStatusType = (s: string) => {
    switch (s) {
        case 'success':
            return 'success';
        case 'running':
        case 'pending':
            return 'warning';
        case 'rolledback':
            return 'warning';
        case 'failed':
            return 'danger';
        default:
            return 'info';
    }
};

// Warnings arrive as "kind:arg:value" codes so they can be translated.
export const warningText = (code: string) => {
    const parts = code.split(':');
    // overview warnings are prefixed with the host name
    let host = '';
    if (!['gpuTemp', 'npuTemp', 'npuHealth', 'rdmaErr', 'rdmaDown', 'memLow', 'swap', 'diskHigh'].includes(parts[0])) {
        host = parts.shift() + ' ';
    }
    const [kind, a, b] = parts;
    switch (kind) {
        case 'gpuTemp':
            return host + t('compute.warn.gpuTemp', [a, b]);
        case 'npuTemp':
            return host + t('compute.warn.npuTemp', [a, b]);
        case 'npuHealth':
            return host + t('compute.warn.npuHealth', [a, b]);
        case 'rdmaErr':
            return host + t('compute.warn.rdmaErr', [a, b]);
        case 'rdmaDown':
            return host + t('compute.warn.rdmaDown', [a]);
        case 'memLow':
            return host + t('compute.warn.memLow');
        case 'swap':
            return host + t('compute.warn.swap');
        case 'diskHigh':
            return host + t('compute.warn.diskHigh');
    }
    return code;
};

export const parseSteps = (s: string) => {
    try {
        return JSON.parse(s || '[]') as Array<{
            name: string;
            status: string;
            message: string;
            startedAt: number;
            finishedAt: number;
        }>;
    } catch {
        return [];
    }
};

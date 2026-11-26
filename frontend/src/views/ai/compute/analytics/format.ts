// Compact counts for call / token numbers: 1234 → 1234, 12345 → 12.3k.
export const fmtCount = (v: number) => {
    v = v || 0;
    if (v >= 1e9) return (v / 1e9).toFixed(2) + 'B';
    if (v >= 1e6) return (v / 1e6).toFixed(2) + 'M';
    if (v >= 1e4) return (v / 1e3).toFixed(1) + 'k';
    return String(Math.round(v));
};

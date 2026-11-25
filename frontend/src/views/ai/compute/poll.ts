import { nextTick, onActivated, onDeactivated, onMounted, onUnmounted, ref, type Ref } from 'vue';
import { computeWatch } from '@/api/modules/compute';

// Polling model shared by every page that embeds a compute view
// (Model Services → pools, GPU → scheduling hosts, vLLM → cluster):
// - only the visible view is refreshed, every POLL ms (views without a
//   `refresh` are never polled);
// - a heartbeat tells the backend somebody watches, which switches the
//   collector to the 5s cadence for 30s;
// - everything stops while the browser tab is hidden or the page is
//   deactivated by the keep-alive cache.
const POLL = 5000;
const HEARTBEAT = 20000;

export const useComputePoll = (target: Ref<any>, onBeat?: () => void) => {
    const paused = ref(false);
    const refreshedAt = ref('');
    let timer: ReturnType<typeof setTimeout> | undefined;
    let lastBeat = 0;
    let active = false;
    let inFlight = false;

    const schedule = () => {
        if (!active) return;
        clearTimeout(timer);
        timer = setTimeout(() => tick(false), POLL);
    };

    const tick = async (force = false) => {
        if (!active) return;
        if (document.hidden) {
            paused.value = true;
            schedule();
            return;
        }
        paused.value = false;
        const now = Date.now();
        if (now - lastBeat > HEARTBEAT) {
            lastBeat = now;
            computeWatch(true).catch(() => undefined);
            onBeat?.();
        }
        const cur = target.value;
        if (cur?.refresh && !inFlight) {
            inFlight = true;
            try {
                await cur.refresh(force);
                refreshedAt.value = new Date().toLocaleTimeString();
            } catch {
                // keep the last data on screen
            } finally {
                inFlight = false;
            }
        }
        schedule();
    };

    const onVisibility = () => {
        if (!document.hidden && active) {
            lastBeat = 0;
            clearTimeout(timer);
            tick(false);
        }
    };

    const start = () => {
        if (active) return;
        active = true;
        lastBeat = 0;
        document.addEventListener('visibilitychange', onVisibility);
        nextTick(() => tick(true));
    };

    const stop = () => {
        if (!active) return;
        active = false;
        clearTimeout(timer);
        document.removeEventListener('visibilitychange', onVisibility);
        computeWatch(false).catch(() => undefined);
    };

    // Re-run immediately, e.g. after switching the embedded view.
    const kick = () =>
        nextTick(() => {
            refreshedAt.value = '';
            clearTimeout(timer);
            tick(true);
        });

    onMounted(start);
    onActivated(start);
    onDeactivated(stop);
    onUnmounted(stop);

    return { paused, refreshedAt, tick, kick, start, stop };
};

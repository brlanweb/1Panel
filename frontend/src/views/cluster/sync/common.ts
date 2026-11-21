import { computed, ref } from 'vue';
import i18n from '@/lang';
import { searchClusterNodes } from '@/api/modules/cluster';
import { Cluster } from '@/api/interface/cluster';
import { useGlobalStore } from '@/composables/useGlobalStore';

// A sync node as shown on the sync pages. Keys are strings ('local',
// 'node-3'), never raw ids: the master node's id is 0, which is falsy in
// selects / v-if.
export interface SyncNode {
    key: string;
    id: number;
    name: string;
    addr: string;
    status: string;
}

export const LOCAL_KEY = 'local';
export const nodeKey = (id: number) => (id === 0 ? LOCAL_KEY : `node-${id}`);

export const useSyncNodes = () => {
    const { globalStore } = useGlobalStore();
    const localNode = (): SyncNode => ({
        key: LOCAL_KEY,
        id: 0,
        name: globalStore.getMasterAlias(),
        addr: '',
        status: 'online',
    });
    const nodes = ref<SyncNode[]>([localNode()]);
    const nodesLoading = ref(false);

    const loadNodes = async () => {
        nodes.value = [localNode()];
        nodesLoading.value = true;
        try {
            const res = await searchClusterNodes({ page: 1, pageSize: 500 } as any);
            const remote = (res?.data?.items || []).filter((item: Cluster.NodeInfo) => item.id !== 0);
            nodes.value = [
                localNode(),
                ...remote.map((item: Cluster.NodeInfo) => ({
                    key: nodeKey(item.id),
                    id: item.id,
                    name: item.name,
                    addr: item.entryUrl,
                    status: item.status || 'unknown',
                })),
            ];
        } catch {
            // The request interceptor reports the error; the local node
            // stays selectable.
        } finally {
            nodesLoading.value = false;
        }
    };

    const findNode = (key: string) => nodes.value.find((node) => node.key === key);
    const nodeName = (key: string) => findNode(key)?.name || key;
    const nodeId = (key: string) => findNode(key)?.id;
    const selectableNodes = computed(() => nodes.value.filter((node) => node.status !== 'offline'));

    return { nodes, selectableNodes, nodesLoading, loadNodes, findNode, nodeName, nodeId };
};

export const nodeStatusType = (status: string) => {
    if (status === 'online') return 'success';
    if (status === 'offline') return 'danger';
    return 'info';
};

export const nodeStatusLabel = (status: string) => {
    if (status === 'online') return i18n.global.t('cluster.onlineNodes');
    if (status === 'offline') return i18n.global.t('cluster.offlineNodes');
    return i18n.global.t('cluster.unknownNodes');
};

export const shortImageID = (id: string) => (id || '').replace('sha256:', '').substring(0, 12);

// Mirrors the backend rule (appUnsupportedReason): only apps without
// resource dependencies, or depending on MySQL / MariaDB / PostgreSQL.
export const appUnsupported = (row: Cluster.SyncResource) => {
    const meta = row.meta || {};
    if (meta.appKey === 'openresty') return true;
    if (meta.hasRedis || meta.hasMongo) return true;
    const dbType = String(meta.dbType || '').toLowerCase();
    return dbType !== '' && !['mysql', 'mariadb', 'postgresql'].includes(dbType);
};

export const appNeedsDB = (row?: Cluster.SyncResource) => !!row && !!(row.meta?.linkDB || row.meta?.hasDBHost);

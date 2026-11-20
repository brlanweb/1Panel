import http from '@/api';
import { ResPage } from '../interface';
import { Cluster } from '../interface/cluster';

export const searchClusterNodes = (req: Cluster.NodeSearch) => {
    return http.postLocalNode<ResPage<Cluster.NodeInfo>>(`/core/cluster/nodes/search`, req);
};

export const createClusterNode = (req: Cluster.NodeCreate) => {
    return http.postLocalNode<any>(`/core/cluster/nodes`, req);
};

export const updateClusterNode = (req: Cluster.NodeUpdate) => {
    return http.postLocalNode<any>(`/core/cluster/nodes/update`, req);
};

export const deleteClusterNode = (id: number) => {
    return http.postLocalNode<any>(`/core/cluster/nodes/del`, { id });
};

export const testClusterNode = (id: number) => {
    return http.postLocalNode<Cluster.NodeInfo>(`/core/cluster/nodes/test`, { id });
};

export const checkAllClusterNodes = () => {
    return http.postLocalNode<any>(`/core/cluster/nodes/check/all`, {});
};

export const getClusterNodeOverview = () => {
    return http.get<Cluster.NodeOverview>(`/core/cluster/nodes/overview`);
};

export const listClusterNodeOptions = () => {
    // Local-only core endpoint; the backend never proxies /api/v2/core/*.
    return http.get<Cluster.NodeOption[]>(`/core/cluster/nodes/options`);
};

export const getClusterAggregateOverview = () => {
    return http.get<Cluster.AggregateOverview>(`/core/cluster/nodes/overview/aggregate`);
};

export const getClusterWafOverview = () => {
    return http.get<Cluster.WafOverview>(`/core/cluster/waf/overview`);
};

export const searchClusterWafWebsites = (req: Cluster.WafWebsiteSearch) => {
    return http.postLocalNode<ResPage<Cluster.WafWebsite>>(`/core/cluster/waf/websites/search`, req);
};

export const searchClusterSyncResources = (req: Cluster.SyncResourceSearch) => {
    return http.postLocalNode<Cluster.SyncResource[]>(`/core/cluster/sync/resources/search`, req);
};

export const previewClusterSync = (req: Cluster.SyncPreviewReq) => {
    return http.postLocalNode<Cluster.SyncPreviewItem[]>(`/core/cluster/sync/preview`, req);
};

export const executeClusterSync = (req: Cluster.SyncExecuteReq) => {
    return http.postLocalNode<Cluster.SyncExecuteRes>(`/core/cluster/sync/execute`, req);
};

export const searchClusterSyncTasks = (req: Cluster.SyncTaskSearch) => {
    return http.postLocalNode<ResPage<Cluster.SyncTask>>(`/core/cluster/sync/tasks/search`, req);
};

export const listClusterSyncTaskRecords = (id: number) => {
    return http.postLocalNode<Cluster.SyncTaskRecord[]>(`/core/cluster/sync/tasks/records`, { id });
};

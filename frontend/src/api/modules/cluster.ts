import http from '@/api';
import { ResPage } from '../interface';
import { Cluster } from '../interface/cluster';

export const searchClusterNodes = (req: Cluster.NodeSearch) => {
    return http.post<ResPage<Cluster.NodeInfo>>(`/core/cluster/nodes/search`, req);
};

export const createClusterNode = (req: Cluster.NodeCreate) => {
    return http.post<any>(`/core/cluster/nodes`, req);
};

export const updateClusterNode = (req: Cluster.NodeUpdate) => {
    return http.post<any>(`/core/cluster/nodes/update`, req);
};

export const deleteClusterNode = (id: number) => {
    return http.post<any>(`/core/cluster/nodes/del`, { id });
};

export const testClusterNode = (id: number) => {
    return http.post<Cluster.NodeInfo>(`/core/cluster/nodes/test`, { id });
};

export const checkAllClusterNodes = () => {
    return http.post<any>(`/core/cluster/nodes/check/all`, {});
};

export const getClusterNodeOverview = () => {
    return http.get<Cluster.NodeOverview>(`/core/cluster/nodes/overview`);
};

export const getClusterAggregateOverview = () => {
    return http.get<Cluster.AggregateOverview>(`/core/cluster/nodes/overview/aggregate`);
};

export const getClusterWafOverview = () => {
    return http.get<Cluster.WafOverview>(`/core/cluster/waf/overview`);
};

export const searchClusterWafWebsites = (req: Cluster.WafWebsiteSearch) => {
    return http.post<ResPage<Cluster.WafWebsite>>(`/core/cluster/waf/websites/search`, req);
};

export const searchClusterSyncResources = (req: Cluster.SyncResourceSearch) => {
    return http.post<Cluster.SyncResource[]>(`/core/cluster/sync/resources/search`, req);
};

export const previewClusterSync = (req: Cluster.SyncPreviewReq) => {
    return http.post<Cluster.SyncPreviewItem[]>(`/core/cluster/sync/preview`, req);
};

export const executeClusterSync = (req: Cluster.SyncExecuteReq) => {
    return http.post<Cluster.SyncExecuteRes>(`/core/cluster/sync/execute`, req);
};

export const searchClusterSyncTasks = (req: Cluster.SyncTaskSearch) => {
    return http.post<ResPage<Cluster.SyncTask>>(`/core/cluster/sync/tasks/search`, req);
};

export const listClusterSyncTaskRecords = (id: number) => {
    return http.post<Cluster.SyncTaskRecord[]>(`/core/cluster/sync/tasks/records`, { id });
};

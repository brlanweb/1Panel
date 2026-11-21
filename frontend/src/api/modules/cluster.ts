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

export const searchClusterSyncResources = (req: Cluster.SyncResourceSearch) => {
    return http.postLocalNode<Cluster.SyncResource[]>(`/core/cluster/sync/resources/search`, req, 60000);
};

export const previewClusterSync = (req: Cluster.SyncPreviewReq) => {
    return http.postLocalNode<Cluster.SyncPreviewItem[]>(`/core/cluster/sync/preview`, req, 120000);
};

export const executeClusterSync = (req: Cluster.SyncExecuteReq) => {
    return http.postLocalNode<Cluster.SyncExecuteRes>(`/core/cluster/sync/execute`, req, 60000);
};

export const searchClusterSyncTasks = (req: Cluster.SyncTaskSearch) => {
    return http.postLocalNode<ResPage<Cluster.SyncTask>>(`/core/cluster/sync/tasks/search`, req);
};

export const listClusterSyncTaskRecords = (id: number) => {
    return http.postLocalNode<Cluster.SyncTaskRecord[]>(`/core/cluster/sync/tasks/records`, { id });
};

export const searchDbReplications = () => {
    return http.postLocalNode<Cluster.DbReplicationInfo[]>(`/core/cluster/db-replications/search`, {});
};

export const getDbReplication = (id: number) => {
    return http.postLocalNode<Cluster.DbReplicationInfo>(`/core/cluster/db-replications/get`, { id });
};

export const createDbReplication = (req: Cluster.DbReplicationCreate) => {
    return http.postLocalNode<{ id: number }>(`/core/cluster/db-replications`, req);
};

export const updateDbReplication = (req: Cluster.DbReplicationUpdate) => {
    return http.postLocalNode<any>(`/core/cluster/db-replications/update`, req);
};

export const deleteDbReplication = (id: number) => {
    return http.postLocalNode<any>(`/core/cluster/db-replications/del`, { id });
};

export const searchDbInstances = (nodeId: number, dbType: string) => {
    return http.postLocalNode<Cluster.DbInstance[]>(`/core/cluster/db-replications/instances/search`, {
        nodeId,
        dbType,
    });
};

export const searchDbSchemas = (nodeId: number, dbType: string, instance: string) => {
    return http.postLocalNode<Cluster.DbSchema[]>(`/core/cluster/db-replications/schemas/search`, {
        nodeId,
        dbType,
        instance,
    });
};

export const preflightDbReplication = (id: number) => {
    return http.postLocalNode<Cluster.DbPreflightCheck[]>(`/core/cluster/db-replications/preflight`, { id });
};

export const listDbManualSteps = (id: number) => {
    return http.postLocalNode<Cluster.DbManualStep[]>(`/core/cluster/db-replications/manual-steps`, { id });
};

export const markDbManualStepsDone = (id: number, manualStepKeys: string[]) => {
    return http.postLocalNode<any>(`/core/cluster/db-replications/manual-steps/done`, { id, manualStepKeys });
};

export const configureDbReplication = (id: number) => {
    return http.postLocalNode<any>(`/core/cluster/db-replications/configure`, { id });
};

export const startDbSnapshot = (id: number) => {
    return http.postLocalNode<any>(`/core/cluster/db-replications/snapshot/start`, { id });
};

export const confirmDbReplicationStarted = (id: number, manualStepKeys: string[]) => {
    return http.postLocalNode<any>(`/core/cluster/db-replications/confirm-started`, { id, manualStepKeys });
};

export const refreshDbReplication = (id: number) => {
    return http.postLocalNode<Cluster.DbReplicationInfo>(`/core/cluster/db-replications/refresh`, { id });
};

export const listDbPromoteSteps = (id: number) => {
    return http.postLocalNode<Cluster.DbManualStep[]>(`/core/cluster/db-replications/promote/steps`, { id });
};

export const promoteDbReplication = (id: number, reason: string, manualStepKeys: string[]) => {
    return http.postLocalNode<Cluster.DbFailoverInfo>(`/core/cluster/db-replications/promote`, {
        id,
        reason,
        manualStepKeys,
    });
};

export const listDbFailovers = (id: number) => {
    return http.postLocalNode<Cluster.DbFailoverInfo[]>(`/core/cluster/db-replications/failovers`, { id });
};

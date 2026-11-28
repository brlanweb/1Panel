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
    // Probes every node (status, counts, version, disks): allow more time.
    return http.get<Cluster.AggregateOverview>(`/core/cluster/nodes/overview/aggregate`, undefined, { timeout: 60000 });
};

// mode: 'master' = the master downloads once and pushes the package;
// 'direct' = the node downloads it from the release itself.
export const upgradeClusterNode = (id: number, version = '', mode: 'master' | 'direct' = 'direct') => {
    return http.postLocalNode<any>(`/core/cluster/nodes/upgrade`, { id, version, mode }, 60000);
};

export const setClusterNodeLoginLock = (req: Cluster.NodeLoginLockReq) => {
    return http.postLocalNode<Cluster.NodeLoginLockResult[]>(`/core/cluster/nodes/login-lock`, req, 60000);
};

// Join a Terminal → Hosts entry: install the panel over SSH when missing,
// create a master-only API key on it and register it as a node.
export const joinClusterNode = (req: Cluster.NodeJoinReq) => {
    return http.postLocalNode<any>(`/core/cluster/nodes/join`, req, 60000);
};

export const loadClusterNodeJoinStatus = (hostId: number) => {
    return http.postLocalNode<Cluster.NodeJoinStatus>(`/core/cluster/nodes/join/status`, { id: hostId });
};

export const loadClusterNodeUpgradeStatus = (id: number) => {
    return http.postLocalNode<Cluster.NodeUpgradeStatus>(`/core/cluster/nodes/upgrade/status`, { id });
};

export const searchClusterSyncResources = (req: Cluster.SyncResourceSearch) => {
    return http.postLocalNode<Cluster.SyncResourcePage>(`/core/cluster/sync/resources/search`, req, 60000);
};

export const listClusterSyncAppDatabases = (req: Cluster.SyncAppDBReq) => {
    return http.postLocalNode<Cluster.SyncAppDB[]>(`/core/cluster/sync/app/databases`, req, 60000);
};

export const deleteClusterSyncFile = (req: Cluster.SyncFileDelete) => {
    return http.postLocalNode<any>(`/core/cluster/sync/files/del`, req, 60000);
};

export const previewClusterSync = (req: Cluster.SyncPreviewReq) => {
    return http.postLocalNode<Cluster.SyncPreviewItem[]>(`/core/cluster/sync/preview`, req, 120000);
};

export const executeClusterSync = (req: Cluster.SyncExecuteReq) => {
    return http.postLocalNode<Cluster.SyncExecuteRes>(`/core/cluster/sync/execute`, req, 60000);
};

export const cancelClusterSyncTask = (id: number) => {
    return http.postLocalNode(`/core/cluster/sync/tasks/cancel`, { id });
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

// Cluster resource monitor. Both endpoints are cached server-side
// (realtime ~1s, static 30s), so polling them never fans out more than
// once per period no matter how many tabs are open.
export const loadClusterMonitorRealtime = () => {
    return http.get<Cluster.MonitorRealtimeRes>(
        `/core/cluster/resource/realtime`,
        {},
        { headers: { CurrentNode: 'local' } },
    );
};

// withHW also reads CPU temperature / GPU metrics on the nodes (vendor tool
// shell-outs); pass it only from pages that display them.
export const loadClusterMonitorStatic = (force = false, withHW = false) => {
    return http.postLocalNode<Cluster.MonitorStaticRes>(`/core/cluster/resource/static`, { force, withHW }, 30000);
};

import { ReqPage } from './index';

export namespace Cluster {
    export interface NodeInfo {
        id: number;
        name: string;
        entryUrl: string;
        remark: string;
        groupId: number;
        keyHint: string;
        status: string;
        latencyMs: number;
        version: string;
        lastCheckAt: string;
        lastError: string;
        cpuUsedPercent: number;
        memUsedPercent: number;
        cpuTemperature: number;
        gpuCount: number;
        gpuUtilMax: number;
        gpuTempMax: number;
        appCount: number;
        websiteCount: number;
        lastSyncStatus: string;
        lastSyncAt: string;
        loginLocked: boolean | null;
        loginUnlockUntil: number;
        createdAt: string;
        updatedAt: string;
    }

    export interface NodeLoginLockReq {
        nodeIds: number[];
        locked: boolean;
        unlockMinutes: number;
    }

    export interface NodeJoinReq {
        hostId: number;
        name: string;
        groupId: number;
        masterIp: string;
        entryHost: string;
        installDocker: boolean;
    }
    export interface NodeJoinStatus {
        hostId: number;
        stage: string;
        running: boolean;
        error: string;
        done: number;
        total: number;
        installed: boolean;
        nodeId: number;
        nodeName: string;
        entryUrl: string;
        username: string;
        password: string;
        logs: string[];
    }
    export interface NodeLoginLockResult {
        nodeId: number;
        name: string;
        success: boolean;
        supported: boolean;
        locked: boolean;
        unlockUntil: number;
        message: string;
    }

    export interface NodeCreate {
        name: string;
        entryUrl: string;
        remark: string;
        groupId: number;
        apiKey: string;
    }

    export interface NodeUpdate {
        id: number;
        name: string;
        entryUrl: string;
        remark: string;
        groupId: number;
        apiKey?: string;
    }

    export interface NodeSearch extends ReqPage {
        info?: string;
        status?: string;
        groupId?: number;
    }

    export interface NodeOverview {
        total: number;
        online: number;
        offline: number;
        unknown: number;
    }

    // One entry of the bottom node switcher. The master ("local") entry is
    // added by the frontend; the rest are registered panel nodes.
    export interface NodeOption {
        id: number;
        name: string;
        addr: string;
        status: string;
        isBound: boolean;
        version: string;
    }

    export interface NodeSnapshot {
        nodeId: number;
        name: string;
        status: string;
        cpuUsedPercent: number;
        memUsedPercent: number;
        cpuTotal: number;
        memTotal: number;
        appCount: number;
        websiteCount: number;
        fetchError: string;
        refreshedAt: string;
        version?: string;
        diskUsedPercent?: number;
        diskTotal?: number;
        diskUsed?: number;
        disks?: DiskInfo[];
    }

    export interface DiskInfo {
        path: string;
        total: number;
        used: number;
        usedPercent: number;
    }

    export interface NodeUpgradeStatus {
        id: number;
        stage: string;
        version: string;
        currentVersion: string;
        done: number;
        total: number;
        error: string;
        reachable: boolean;
    }

    export interface AggregateOverview extends NodeOverview {
        totalApps: number;
        totalWebsites: number;
        nodes: NodeSnapshot[];
        // The master panel itself (counts from its own agent; -1 = failed).
        local?: NodeSnapshot;
        localError?: string;
        latestVersion?: string;
    }

    export type SyncKind = 'file' | 'image' | 'app' | 'cert' | 'php' | 'model';

    export interface SyncResource {
        key: string;
        name: string;
        detail: string;
        size: number;
        extra: string;
        meta?: Record<string, any>;
    }

    export interface SyncResourcePage {
        items: SyncResource[];
        total: number;
        path: string;
    }

    export interface SyncResourceSearch {
        kind: SyncKind;
        sourceNodeId: number;
        path?: string;
        search?: string;
        sortOrder?: string;
        page?: number;
        pageSize?: number;
    }

    export interface SyncPreviewReq {
        kind: SyncKind;
        sourceNodeId: number;
        targetNodeIds: number[];
        resourceKeys: string[];
        path?: string;
        targetPath?: string;
        appDbHost?: string;
    }

    export interface SyncAppDBReq {
        nodeIds: number[];
        types?: string;
    }

    export interface SyncAppDB {
        type: string;
        database: string;
    }

    export interface SyncFileDelete {
        nodeId: number;
        path: string;
        isDir: boolean;
    }

    export interface SyncPreviewItem {
        resourceKey: string;
        resourceName: string;
        targetNodeId: number;
        targetName: string;
        action: string;
        reason: string;
    }

    export interface SyncExecuteReq extends SyncPreviewReq {
        overwriteConflicts: boolean;
    }

    export interface SyncExecuteRes {
        taskId: number;
    }

    export interface SyncTask {
        id: number;
        kind: string;
        sourceNodeId: number;
        sourceName: string;
        targetNodeIds: string;
        status: string;
        total: number;
        succeeded: number;
        skipped: number;
        failed: number;
        overwriteConflicts: boolean;
        createdAt: string;
        updatedAt: string;
        step?: string;
        progressDone?: number;
        progressTotal?: number;
        progressRate?: number;
    }

    export interface SyncTaskSearch extends ReqPage {
        kind?: string;
        status?: string;
    }

    export interface SyncTaskRecord {
        id: number;
        taskId: number;
        kind: string;
        resourceKey: string;
        resourceName: string;
        sourceNodeId: number;
        targetNodeId: number;
        targetName: string;
        action: string;
        reason: string;
        durationMs: number;
        createdAt: string;
    }

    // Cross-server one-way database replication (MySQL GTID + binlog, or
    // PostgreSQL logical replication). Replica lag is not readable through
    // any existing panel API, so lastDelaySeconds stays null (shown as —).
    export type DbReplicationType = 'mysql' | 'mariadb' | 'postgresql';

    export interface DbReplicationInfo {
        id: number;
        name: string;
        dbType: DbReplicationType;
        masterNodeId: number;
        masterName: string;
        masterInstance: string;
        masterHost: string;
        masterPort: number;
        database: string;
        replicaNodeId: number;
        replicaName: string;
        replicaInstance: string;
        replicaDatabase: string;
        replUsername: string;
        status: string;
        setupStep: string;
        manualSteps: string;
        preflight: string;
        masterServerId: number;
        replicaServerId: number;
        masterBinlogFile: string;
        masterBinlogPosition: string;
        snapshotStatus: string;
        snapshotDetail: string;
        lastDelaySeconds: number | null;
        lastError: string;
        lastCheckedAt: string;
        lastConfirmedAt: string;
        failoverSuggested: boolean;
        createdAt: string;
        updatedAt: string;
    }

    export interface DbReplicationCreate {
        name: string;
        dbType: DbReplicationType;
        masterNodeId: number;
        masterInstance: string;
        masterHost: string;
        masterPort: number;
        database: string;
        replicaNodeId: number;
        replicaInstance: string;
        replicaDatabase: string;
        replUsername: string;
        replPassword: string;
    }

    export interface DbReplicationUpdate {
        id: number;
        name?: string;
        replUsername?: string;
        replPassword?: string;
    }

    export interface DbPreflightCheck {
        name: string;
        status: 'pass' | 'fail' | 'warn' | 'manual';
        detail: string;
    }

    export interface DbManualStep {
        key: string;
        title: string;
        sql: string;
        done: boolean;
    }

    export interface DbInstance {
        id: number;
        name: string;
        type: string;
        version: string;
        port: number;
    }

    export interface DbSchema {
        name: string;
    }

    export interface DbFailoverInfo {
        id: number;
        replicationId: number;
        fromNodeId: number;
        fromName: string;
        toNodeId: number;
        toName: string;
        reason: string;
        repointed: number;
        createdAt: string;
    }

    export interface MonitorRealtime {
        nodeId: number;
        name: string;
        online: boolean;
        cpuUsedPercent: number;
        cpuTotal: number;
        load1: number;
        upSpeed: number;
        downSpeed: number;
        netBytesSent: number;
        netBytesRecv: number;
        error: string;
        sampledAt: number;
    }

    export interface MonitorDisk {
        path: string;
        type: string;
        device: string;
        total: number;
        used: number;
        free: number;
        usedPercent: number;
    }

    export interface MonitorStatic {
        nodeId: number;
        name: string;
        entryHost: string;
        online: boolean;
        hostname: string;
        os: string;
        prettyDistro: string;
        kernelArch: string;
        uptime: number;
        cpuTotal: number;
        cpuModelName: string;
        memoryTotal: number;
        memoryUsed: number;
        memoryUsedPercent: number;
        swapTotal: number;
        swapUsed: number;
        swapUsedPercent: number;
        diskTotal: number;
        diskUsed: number;
        diskUsedPercent: number;
        disks: MonitorDisk[];
        internalIPs: string[];
        publicIP: string;
        cpuTemperature: number;
        gpus: MonitorGPU[] | null;
        gpuUtilMax: number;
        gpuTempMax: number;
        error: string;
        sampledAt: number;
    }

    // One accelerator; null metrics were not reported by the vendor tool.
    export interface MonitorGPU {
        index: number;
        kind: string;
        vendor: string;
        name: string;
        utilization: number | null;
        temperature: number | null;
        memoryUsed: number | null;
        memoryTotal: number | null;
    }

    export interface MonitorRisk {
        nodeId: number;
        name: string;
        kind: 'offline' | 'cpu' | 'memory' | 'disk' | 'cpuTemp' | 'gpuTemp';
        level: 'warning' | 'critical';
        value: number;
        target: string;
        message: string;
    }

    export interface MonitorStaticRes {
        nodes: MonitorStatic[];
        risks: MonitorRisk[];
        total: number;
        online: number;
        offline: number;
        warning: number;
        critical: number;
        warnPercent: number;
        criticalPercent: number;
        warnTemp: number;
        criticalTemp: number;
    }

    export interface MonitorRealtimeRes {
        nodes: MonitorRealtime[];
    }
}

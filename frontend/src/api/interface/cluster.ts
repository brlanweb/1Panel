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
        appCount: number;
        websiteCount: number;
        lastSyncStatus: string;
        lastSyncAt: string;
        createdAt: string;
        updatedAt: string;
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
    }

    export interface AggregateOverview extends NodeOverview {
        totalApps: number;
        totalWebsites: number;
        nodes: NodeSnapshot[];
    }

    export interface WafOverview {
        totalNodes: number;
        totalWebsites: number;
        nodes: WafNodeStatus[];
    }

    export interface WafNodeStatus {
        nodeId: number;
        name: string;
        websiteCount: number;
        fetchError: string;
    }

    export interface WafWebsite {
        nodeId: number;
        nodeName: string;
        domain: string;
        type: string;
        status: string;
        enabled: boolean | null;
        mode: string;
        strength: string;
        rateLimit: boolean | null;
        schemaPending: boolean;
        jumpUrl: string;
    }

    export interface WafWebsiteSearch extends ReqPage {
        nodeId?: number;
    }

    export type SyncKind = 'file' | 'image' | 'app' | 'cert' | 'php';

    export interface SyncResource {
        key: string;
        name: string;
        detail: string;
        size: number;
        extra: string;
    }

    export interface SyncResourceSearch {
        kind: SyncKind;
        sourceNodeId: number;
        path?: string;
        page?: number;
        pageSize?: number;
    }

    export interface SyncPreviewReq {
        kind: SyncKind;
        sourceNodeId: number;
        targetNodeIds: number[];
        resourceKeys: string[];
        path?: string;
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
}

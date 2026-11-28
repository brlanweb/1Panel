export namespace Compute {
    export interface GPU {
        kind: string;
        index: number;
        card: number;
        chip: number;
        health: string;
        name: string;
        util: number;
        temp: number;
        power: number;
        memUsed: number;
        memTotal: number;
        unified: boolean;
    }
    export interface Nic {
        name: string;
        state: string;
        speedMbps: number;
        ip: string;
        rxRate: number;
        txRate: number;
    }
    export interface Rdma {
        dev: string;
        port: string;
        netDev: string;
        state: string;
        rateGbps: number;
        rxRate: number;
        txRate: number;
        rcvErrors: number;
        symbolErrors: number;
        linkDowned: number;
        xmitDiscards: number;
        linkErrRecovery: number;
        errDelta: number;
    }
    export interface Process {
        pid: number;
        engine: string;
        port: number;
        model: string;
        servedName: string;
        cmd: string;
        instanceId: number;
        // Non-head rank of a multi-host group (--headless / --node-rank > 0).
        worker?: boolean;
    }
    export interface HostRealtime {
        online: boolean;
        error: string;
        sampledAt: number;
        cpuPercent: number;
        load1: number;
        cores: number;
        memTotal: number;
        memAvailable: number;
        memUsedPercent: number;
        swapUsed: number;
        diskTotal: number;
        diskUsed: number;
        gpus: GPU[] | null;
        nics: Nic[] | null;
        rdma: Rdma[] | null;
        processes: Process[] | null;
        warnings: string[] | null;
        collectMs: number;
    }
    export interface Host {
        id: number;
        name: string;
        addr: string;
        source: 'terminal' | 'node' | 'local';
        refId: number;
        sourceName: string;
        sourceDetail: string;
        sourceMissing: boolean;
        mode: string;
        labels: string;
        remark: string;
        schedulable: boolean;
        status: string;
        lastError: string;
        hostname: string;
        arch: string;
        os: string;
        driver: string;
        gpuModel: string;
        gpuCount: number;
        cpuCores: number;
        memTotal: number;
        unifiedMem: boolean;
        realtime: HostRealtime;
        domain: string;
        instanceNames: string[] | null;
    }
    export interface HostReq {
        id: number;
        name: string;
        addr: string;
        labels: string;
        remark: string;
        schedulable?: boolean;
    }
    export interface HostCandidate {
        source: string;
        refId: number;
        name: string;
        addr: string;
        detail: string;
        status: string;
        imported: boolean;
        hostId: number;
    }
    export interface HostImportReq {
        source: string;
        refIds: number[];
        labels: string;
    }
    export interface Link {
        id: number;
        domain: string;
        hostAId: number;
        hostBId: number;
        ifaceA: string;
        ifaceB: string;
        ipA: string;
        ipB: string;
        rdmaA: string;
        rdmaB: string;
        rateGbps: number;
        state: string;
        source: string;
    }
    export interface Domain {
        name: string;
        hostIds: number[];
        links: Link[];
        healthy: boolean;
    }
    export interface Runtime {
        id: number;
        name: string;
        engine: string;
        device: string;
        backend: 'script' | 'vllm';
        image: string;
        multiNode: boolean;
        launchTemplate: string;
        stopTemplate: string;
        healthPath: string;
        metricsPath: string;
        builtin: boolean;
        remark: string;
    }
    export interface Profile {
        id: number;
        name: string;
        runtimeId: number;
        servedName: string;
        modelPath: string;
        nodeCount: number;
        needFabric: boolean;
        minMemGb: number;
        port: number;
        extraArgs: string;
        env: string;
        startTimeoutSec: number;
        remark: string;
        modelRepo: string;
        // Pinned hosts, head first (comma separated; empty = any host).
        hostIds: string;
        maxPerHost: number;
        image: string;
    }
    export interface ProfileReq extends Omit<Profile, 'hostIds'> {
        hostIds: number[];
    }
    export interface ModelHost {
        hostId: number;
        hostName: string;
        dir: string;
        size: number;
        recipeId: number;
    }
    export interface ModelRecipe {
        name: string;
        gpuType: string;
        image: string;
        args: string;
        env: string;
    }
    export interface PreparedModel {
        repoId: string;
        name: string;
        hosts: ModelHost[] | null;
        recipe: ModelRecipe | null;
    }
    export interface ModelList {
        items: PreparedModel[] | null;
        errors: string[] | null;
    }
    export interface CallLog {
        id: number;
        time: number;
        consumer: string;
        consumerId: number;
        model: string;
        instance: string;
        path: string;
        stream: boolean;
        status: number;
        success: boolean;
        promptTokens: number;
        completionTokens: number;
        ttftMs: number;
        latencyMs: number;
        retries: number;
        clientIp: string;
        error: string;
    }
    export interface CallLogSearch {
        page: number;
        pageSize: number;
        model?: string;
        consumer?: string;
        status?: '' | 'success' | 'failed';
        start?: number;
        end?: number;
    }
    export interface AnalyticsSummary {
        requests: number;
        failures: number;
        promptTokens: number;
        completionTokens: number;
        totalTokens: number;
        avgRpm: number;
        successRate: number;
        avgTtftMs: number;
        avgLatencyMs: number;
    }
    export interface AnalyticsBucket {
        time: number;
        requests: number;
        failures: number;
        tokens: number;
        avgTtftMs: number;
        avgLatencyMs: number;
        byModel: Record<string, number>;
    }
    export interface AnalyticsShare {
        name: string;
        requests: number;
        tokens: number;
        failures: number;
        avgTtftMs: number;
    }
    export interface UsageTotals {
        since: string;
        requests: number;
        failures: number;
        promptTokens: number;
        completionTokens: number;
        totalTokens: number;
        todayRequests: number;
        todayTokens: number;
    }
    export interface Analytics {
        totals?: UsageTotals;
        start: number;
        end: number;
        bucketSec: number;
        retentionHours: number;
        summary: AnalyticsSummary;
        buckets: AnalyticsBucket[] | null;
        byModel: AnalyticsShare[] | null;
        byConsumer: AnalyticsShare[] | null;
        models: string[] | null;
        consumers: string[] | null;
    }
    export interface EngineMetrics {
        healthy: boolean;
        running: number;
        waiting: number;
        kvUsage: number;
        ttftMs: number;
        queueMs: number;
        promptTps: number;
        genTps: number;
        inflight: number;
        lastActiveAt: number;
        sampledAt: number;
        error: string;
        reportedModel: string;
    }
    export interface Instance {
        id: number;
        name: string;
        profileId: number;
        servedName: string;
        // Model name the engine itself serves when it differs from servedName.
        upstreamModel?: string;
        headHostId: number;
        hostIds: string;
        port: number;
        endpoint: string;
        containerName: string;
        adopted: boolean;
        status: string;
        routeEnabled: boolean;
        weight: number;
        lastError: string;
        hostNames: string[] | null;
        profileName: string;
        metrics: EngineMetrics;
    }
    export interface JobStep {
        name: string;
        status: string;
        message: string;
        startedAt: number;
        finishedAt: number;
    }
    export interface Job {
        id: number;
        type: string;
        status: string;
        servedName: string;
        instanceId: number;
        profileId: number;
        hostIds: string;
        source: string;
        reason: string;
        steps: string;
        error: string;
        startedAt: string;
        finishedAt: string;
        createdAt: string;
    }
    export interface PrecheckItem {
        name: string;
        ok: boolean;
        message: string;
    }
    export interface PrecheckRes {
        ok: boolean;
        hostIds: number[] | null;
        port: number;
        items: PrecheckItem[] | null;
        jobId: number;
    }
    export interface Consumer {
        id: number;
        name: string;
        keyHint: string;
        allowedModels: string;
        rpm: number;
        enabled: boolean;
        requests: number;
        failures: number;
        promptTokens: number;
        completionTokens: number;
        lastUsedAt: string;
        remark: string;
    }
    export interface RouterSetting {
        enabled: boolean;
        listen: string;
        maxRetries: number;
        requestTimeout: number;
        logRetentionHours: number;
        running: boolean;
        error: string;
        schedulerPaused: boolean;
    }
    export interface Pool extends Policy {
        name: string;
        accountId: number;
        consumerId: number;
        profileName: string;
        nodeCount: number;
        instances: number;
        healthy: number;
        serviceMissing: boolean;
        baseUrl: string;
    }
    export interface PoolRes {
        id: number;
        accountId: number;
        key: string;
    }
    export interface Policy {
        ttftHigh: number;
        id: number;
        servedName: string;
        profileId: number;
        mode: string;
        minInstances: number;
        maxInstances: number;
        hotSpare: number;
        priority: number;
        preemptible: boolean;
        queueHigh: number;
        kvHigh: number;
        sustainRounds: number;
        idleMinutes: number;
        cooldownSec: number;
    }
    export interface Suggestion {
        id: string;
        kind: string;
        servedName: string;
        profileId: number;
        instanceId: number;
        hostIds: number[] | null;
        reason: string;
        severity: string;
        createdAt: number;
        actionable: boolean;
    }
    export interface ModelSummary {
        servedName: string;
        instances: number;
        healthy: number;
        running: number;
        waiting: number;
        kvUsage: number;
        ttftMs: number;
        genTps: number;
        policyMode: string;
        minInstances: number;
        maxInstances: number;
    }
    export interface Overview {
        hosts: number;
        hostsOnline: number;
        hostsIdle: number;
        hostsCordoned: number;
        gpus: number;
        npus: number;
        gpuUtilAvg: number;
        memTotal: number;
        memUsed: number;
        instances: number;
        instHealthy: number;
        domains: number;
        fabricIssues: number;
        runningJobs: number;
        suggestions: number;
        genTps: number;
        models: ModelSummary[] | null;
        warnings: string[] | null;
        router: RouterSetting;
    }
}

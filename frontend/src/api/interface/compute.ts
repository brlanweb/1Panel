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
        sshPort: number;
        sshUser: string;
        authMode: string;
        mode: string;
        clusterNodeId: number;
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
        hasCredential: boolean;
        realtime: HostRealtime;
        domain: string;
        instanceNames: string[] | null;
    }
    export interface HostReq {
        id?: number;
        name: string;
        addr: string;
        sshPort: number;
        sshUser: string;
        authMode: string;
        credential: string;
        passPhrase: string;
        clusterNodeId?: number;
        labels: string;
        remark: string;
        schedulable?: boolean;
    }
    export interface HostBatchReq {
        lines: string;
        sshUser: string;
        sshPort: number;
        authMode: string;
        credential: string;
        passPhrase: string;
        labels: string;
    }
    export interface HostTestRes {
        ok: boolean;
        message: string;
        hostname: string;
        arch: string;
        os: string;
        driver: string;
        gpuModel: string;
        latencyMs: number;
        sudo: boolean;
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
        running: boolean;
        error: string;
        schedulerPaused: boolean;
    }
    export interface Policy {
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

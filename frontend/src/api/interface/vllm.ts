import { ReqPage } from '.';

export namespace Vllm {
    export interface Settings {
        modelDir: string;
        tokenizerDir: string;
        hfEndpoint: string;
        hfToken: string;
        modelScopeUrl: string;
        modelScopeToken: string;
    }

    export interface Search extends ReqPage {
        info: string;
        kind?: string;
    }

    export interface Model {
        id: number;
        createdAt: string;
        repoId: string;
        source: string;
        kind: string;
        revision: string;
        dir: string;
        status: string;
        message: string;
        size: number;
        taskID: string;
        recipeId: number;
    }

    export interface ModelDownload {
        repoId: string;
        source: string;
        kind: string;
        revision: string;
        recipeId?: number;
        taskID: string;
    }

    export interface ModelImport {
        repoId: string;
        kind: string;
        dir: string;
    }

    export interface Recipe {
        id: number;
        name: string;
        gpuType: string;
        modelRepo: string;
        image: string;
        args: string;
        env: string;
        description: string;
        builtin?: boolean;
    }

    export interface Deployment {
        id: number;
        createdAt?: string;
        name: string;
        external: boolean;
        baseUrl: string;
        apiKey: string;
        image: string;
        containerName?: string;
        hostIP: string;
        port: number;
        modelId: number;
        modelName?: string;
        servedName: string;
        gpuType: string;
        gpuDevices: string;
        tensorParallel: number;
        maxModelLen: number;
        gpuMemoryUtil: string;
        shmSize: string;
        args: string;
        env: string;
        recipeId: number;
        dir?: string;
        status?: string;
        message?: string;
        endpoint?: string;
        taskID: string;
    }

    export interface DeploymentStats {
        id: number;
        status: string;
        message: string;
        healthy: boolean;
        cpuPercent: number;
        memoryMB: number;
    }

    export interface Health {
        healthy: boolean;
        models: string[];
        message: string;
    }
}

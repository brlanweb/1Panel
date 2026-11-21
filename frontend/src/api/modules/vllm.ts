import http from '@/api';
import { ResPage } from '../interface';
import { Vllm } from '../interface/vllm';

export const loadVllmSettings = () => {
    return http.get<Vllm.Settings>(`/ai/vllm/settings`);
};
export const updateVllmSettings = (req: Vllm.Settings) => {
    return http.post(`/ai/vllm/settings/update`, req);
};

export const pageVllmModels = (req: Vllm.Search) => {
    return http.post<ResPage<Vllm.Model>>(`/ai/vllm/models/search`, req);
};
export const downloadVllmModel = (req: Vllm.ModelDownload) => {
    return http.post(`/ai/vllm/models/download`, req);
};
export const importVllmModel = (req: Vllm.ModelImport) => {
    return http.post(`/ai/vllm/models/import`, req);
};
export const cancelVllmModel = (id: number) => {
    return http.post(`/ai/vllm/models/cancel`, { id: id });
};
export const retryVllmModel = (id: number) => {
    return http.post(`/ai/vllm/models/retry`, { id: id });
};
export const deleteVllmModel = (req: { id: number; deleteFile: boolean }) => {
    return http.post(`/ai/vllm/models/del`, req);
};

export const pageVllmRecipes = (req: Vllm.Search) => {
    return http.post<ResPage<Vllm.Recipe>>(`/ai/vllm/recipes/search`, req);
};
export const saveVllmRecipe = (req: Vllm.Recipe) => {
    return http.post(`/ai/vllm/recipes`, req);
};
export const deleteVllmRecipe = (req: { id: number }) => {
    return http.post(`/ai/vllm/recipes/del`, req);
};

export const pageVllmDeployments = (req: Vllm.Search) => {
    return http.post<ResPage<Vllm.Deployment>>(`/ai/vllm/deployments/search`, req);
};
export const saveVllmDeployment = (req: Vllm.Deployment) => {
    return http.post(`/ai/vllm/deployments`, req);
};
export const operateVllmDeployment = (id: number, operate: string) => {
    return http.post(`/ai/vllm/deployments/operate`, { id: id, operate: operate });
};
export const deleteVllmDeployment = (req: { id: number }) => {
    return http.post(`/ai/vllm/deployments/del`, req);
};
export const loadVllmDeploymentStats = (ids: number[]) => {
    return http.post<Vllm.DeploymentStats[]>(`/ai/vllm/deployments/stats`, { ids: ids });
};
export const checkVllmDeployment = (id: number) => {
    return http.post<Vllm.Health>(`/ai/vllm/deployments/check`, { id: id });
};

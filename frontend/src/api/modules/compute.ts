import http from '@/api';
import { ResPage, SearchWithPage } from '../interface';
import { Compute } from '../interface/compute';

// All compute endpoints live in the local (master) core only.
const local = { headers: { CurrentNode: 'local' } };
const get = <T>(url: string) => http.get<T>(url, undefined, local);
const post = <T>(url: string, body: object = {}, timeout?: number) => http.postLocalNode<T>(url, body, timeout);

export const getComputeOverview = () => get<Compute.Overview>(`/core/compute/overview`);
export const computeWatch = (fast: boolean) => post<any>(`/core/compute/watch`, { fast });

export const listComputeHosts = (info = '') => post<Compute.Host[]>(`/core/compute/hosts/search`, { info });
export const listComputeHostCandidates = (source: string) =>
    post<Compute.HostCandidate[]>(`/core/compute/hosts/candidates`, { source });
export const importComputeHosts = (req: Compute.HostImportReq) =>
    post<{ imported: number; error: string }>(`/core/compute/hosts/import`, req);
export const updateComputeHost = (req: Compute.HostReq) => post<any>(`/core/compute/hosts/update`, req);
export const deleteComputeHost = (id: number) => post<any>(`/core/compute/hosts/del`, { id });
export const setComputeSchedulable = (id: number, schedulable: boolean) =>
    post<any>(`/core/compute/hosts/schedulable`, { id, schedulable });
export const refreshComputeHost = (id: number) => post<any>(`/core/compute/hosts/refresh`, { id });

export const getComputeTopology = () => get<Compute.Domain[]>(`/core/compute/topology`);
export const createComputeLink = (req: object) => post<any>(`/core/compute/topology/link`, req);
export const deleteComputeLink = (id: number) => post<any>(`/core/compute/topology/link/del`, { id });

export const listComputeRuntimes = () => get<Compute.Runtime[]>(`/core/compute/runtimes`);
export const saveComputeRuntime = (req: Partial<Compute.Runtime>) => post<any>(`/core/compute/runtimes`, req);
export const deleteComputeRuntime = (id: number) => post<any>(`/core/compute/runtimes/del`, { id });
export const listComputeProfiles = () => get<Compute.Profile[]>(`/core/compute/profiles`);
export const saveComputeProfile = (req: Partial<Compute.Profile>) => post<any>(`/core/compute/profiles`, req);
export const deleteComputeProfile = (id: number) => post<any>(`/core/compute/profiles/del`, { id });

export const listComputeInstances = () => get<Compute.Instance[]>(`/core/compute/instances`);
export const saveComputeInstance = (req: object) => post<any>(`/core/compute/instances`, req);
export const adoptComputeInstance = (req: object) => post<any>(`/core/compute/instances/adopt`, req);
export const setComputeRoute = (id: number, routeEnabled: boolean, weight: number) =>
    post<any>(`/core/compute/instances/route`, { id, routeEnabled, weight });
export const operateComputeInstance = (id: number, operation: string, force = false) =>
    post<{ jobId: number }>(`/core/compute/instances/operate`, { id, operation, force });

// Precheck checks every target host (SSH / panel API): allow more time.
export const computeDeploy = (req: { profileId: number; hostIds: number[]; name: string; dryRun: boolean }) =>
    post<Compute.PrecheckRes>(`/core/compute/deploy`, req, 120000);
export const computeSwitch = (req: { instanceId: number; profileId: number; dryRun: boolean }) =>
    post<Compute.PrecheckRes>(`/core/compute/switch`, req, 120000);
export const searchComputeJobs = (req: SearchWithPage) => post<ResPage<Compute.Job>>(`/core/compute/jobs/search`, req);
export const getComputeJob = (id: number) => post<Compute.Job>(`/core/compute/jobs/get`, { id });

export const listComputeConsumers = () => get<Compute.Consumer[]>(`/core/compute/consumers`);
export const saveComputeConsumer = (req: object) => post<{ id: number; key: string }>(`/core/compute/consumers`, req);
export const resetComputeConsumer = (id: number) =>
    post<{ id: number; key: string }>(`/core/compute/consumers/reset`, { id });
export const deleteComputeConsumer = (id: number) => post<any>(`/core/compute/consumers/del`, { id });
export const getComputeSetting = () => get<Compute.RouterSetting>(`/core/compute/setting`);
export const updateComputeSetting = (req: Compute.RouterSetting) => post<any>(`/core/compute/setting`, req);

export const listComputePolicies = () => get<Compute.Policy[]>(`/core/compute/policies`);
export const saveComputePolicy = (req: Partial<Compute.Policy>) => post<any>(`/core/compute/policies`, req);
export const deleteComputePolicy = (id: number) => post<any>(`/core/compute/policies/del`, { id });
export const listComputeSuggestions = () => get<Compute.Suggestion[]>(`/core/compute/suggestions`);
export const operateComputeSuggestion = (id: string, operation: 'apply' | 'dismiss') =>
    post<{ jobId: number }>(`/core/compute/suggestions/operate`, { id, operation });

import http from '@/api';
import { TimeoutEnum } from '@/enums/http-enum';
import { ProxyNode } from '../interface/proxy-node';

export const loadProxyNodeStatus = () => {
    return http.get<ProxyNode.Status>(`/settings/proxy-node`);
};
export const saveProxyNode = (req: ProxyNode.Save) => {
    return http.post(`/settings/proxy-node/save`, req, TimeoutEnum.T_5M);
};
export const refreshProxyNode = () => {
    return http.post(`/settings/proxy-node/refresh`, {}, TimeoutEnum.T_5M);
};
export const selectProxyNode = (name: string) => {
    return http.post(`/settings/proxy-node/select`, { name });
};
export const testProxyNodes = () => {
    return http.post(`/settings/proxy-node/test`, {});
};
export const operateProxyNode = (operate: 'start' | 'stop') => {
    return http.post(`/settings/proxy-node/operate`, { operate }, TimeoutEnum.T_5M);
};

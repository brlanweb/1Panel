import { ResultEnum } from '@/enums/http-enum';
import i18n from '@/lang';
import router from '@/routers';
import { useGlobalStore } from '@/composables/useGlobalStore';
import { MsgError } from '@/utils/message';

export type AuthResponseAction = 'reject' | 'return';

export interface AuthResponseResult {
    handled: boolean;
    action?: AuthResponseAction;
    message: string;
}

interface AuthResponseOptions {
    showRBACMessage?: boolean;
    // When true, a 401/403 is reported as a plain rejection instead of
    // redirecting to the login page. This is for requests that are
    // deliberately sent to a *remote* panel through the cluster proxy:
    // a remote auth failure (API interface disabled, wrong key, ...) is
    // a node problem, not "my own session expired", so it must never
    // redirect the master UI to login.
    skipRedirect?: boolean;
}

const forbiddenMessage = () => i18n.global.t('commons.res.forbidden');

export const redirectToEntrance = () => {
    const { entrance, globalStore } = useGlobalStore();
    globalStore.setLogStatus(false);
    globalStore.clearAuthInfo();
    router.push({
        name: 'entrance',
        params: { code: entrance.value },
    });
};

export const redirectToExpired = () => {
    router.push({ name: 'Expired' });
};

export const handleAuthResponseCode = (data: any, options: AuthResponseOptions = {}): AuthResponseResult => {
    const message = data?.message || forbiddenMessage();
    switch (data?.code) {
        case ResultEnum.OVERDUE:
        case ResultEnum.FORBIDDEN:
            if (options.skipRedirect) {
                return { handled: true, action: 'reject', message };
            }
            redirectToEntrance();
            return { handled: true, action: 'reject', message };
        case ResultEnum.ERR_RBAC:
            if (options.showRBACMessage) {
                MsgError(message);
            }
            return { handled: true, action: 'reject', message };
        case ResultEnum.EXPIRED:
            redirectToExpired();
            return { handled: true, action: 'return', message };
        default:
            return { handled: false, message: '' };
    }
};

export const handleAuthResponseStatus = (status: number, options: AuthResponseOptions = {}): AuthResponseResult => {
    switch (status) {
        case ResultEnum.OVERDUE:
        case ResultEnum.FORBIDDEN:
            if (options.skipRedirect) {
                return { handled: true, action: 'reject', message: forbiddenMessage() };
            }
            redirectToEntrance();
            return { handled: true, action: 'reject', message: forbiddenMessage() };
        case ResultEnum.EXPIRED:
            redirectToExpired();
            return { handled: true, action: 'return', message: forbiddenMessage() };
        default:
            return { handled: false, message: '' };
    }
};

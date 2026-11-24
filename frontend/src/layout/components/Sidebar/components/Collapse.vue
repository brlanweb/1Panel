<template>
    <div>
        <el-popover
            v-model:visible="popoverVisible"
            placement="right-end"
            :show-arrow="false"
            :offset="0"
            :width="200"
            trigger="click"
            popper-class="custom-popover-dropdown"
        >
            <template #reference>
                <div class="el-dropdown-link" v-if="!menuStore.isCollapse">
                    <el-badge is-dot :value="taskCount" :show-zero="false" :offset="[5, 5]">
                        <el-button link>
                            <SvgIcon class="icon" iconName="p-renwuzhongxin1" />
                            <span class="ellipsis-text">{{ $t('menu.msgCenter') }}</span>
                        </el-button>
                    </el-badge>
                </div>
                <div v-else class="el-dropdown-link">
                    <el-badge is-dot :value="taskCount" :show-zero="false" :offset="[-5, 5]">
                        <SvgIcon class="icon" iconName="p-renwuzhongxin1" />
                    </el-badge>
                </div>
            </template>
            <div class="dropdown-menu">
                <div class="dropdown-item" @click="openTask">
                    <SvgIcon class="icon" iconName="p-renwuzhongxin1" />
                    {{ $t('menu.msgCenter') }}
                    <el-tag class="msg-tag" v-if="taskCount !== 0" size="small" round>{{ taskCount }}</el-tag>
                </div>
                <el-divider class="divider" />
                <div class="dropdown-item" @click="openNodeDashboard">
                    <SvgIcon class="icon" iconName="p-gailan1" />
                    {{ isXpackOrEE ? $t('xpack.node.multiOverview') : $t('menu.clusterOverview') }}
                </div>
            </div>
        </el-popover>
        <UserInfo ref="userInfoRef" :currentUser="currentUser" @search="loadCurrentUser()" />
    </div>
</template>

<script setup lang="ts">
import { MenuStore } from '@/store';
import { countExecutingTask } from '@/api/modules/log';
import { MsgSuccess } from '@/utils/message';
import i18n from '@/lang';
import { onMounted, ref } from 'vue';
import bus from '@/global/bus';
import { logOutApi } from '@/api/modules/auth';
import { submitSAML2Navigation } from '@/utils/saml2';
import router from '@/routers';
import { routerToNameWithQuery } from '@/utils/router';
import { Login } from '@/api/interface/auth';
import { syncAuthInfo } from '@/utils/rbac';
import UserInfo from './user-info/index.vue';
import { useGlobalStore } from '@/composables/useGlobalStore';

const currentUser = ref<Login.AuthInfo>();
const { globalStore, entrance, isXpackOrEE } = useGlobalStore();
const menuStore = MenuStore();
const popoverVisible = ref(false);
const userInfoRef = ref();
defineProps({
    version: String,
});

const emit = defineEmits(['openTask', 'nodeSwitching', 'userChange']);
bus.on('refreshTask', () => {
    checkTask();
});

const taskCount = ref(0);
const checkTask = async () => {
    try {
        const res = await countExecutingTask();
        taskCount.value = res.data;
    } catch (error) {}
};

const openTask = () => {
    popoverVisible.value = false;
    emit('openTask');
};

const openNodeDashboard = () => {
    popoverVisible.value = false;
    if (isXpackOrEE.value) {
        routerToNameWithQuery('NodeDashboard', { uncached: 'true' });
        return;
    }
    routerToNameWithQuery('ClusterOverview', { uncached: 'true' });
};

const logout = () => {
    popoverVisible.value = false;
    ElMessageBox.confirm(i18n.global.t('commons.msg.sureLogOut'), i18n.global.t('commons.msg.infoTitle'), {
        confirmButtonText: i18n.global.t('commons.button.confirm'),
        cancelButtonText: i18n.global.t('commons.button.cancel'),
        type: 'warning',
    })
        .then(async () => {
            const res = await logOutApi();
            globalStore.setLogStatus(false);
            globalStore.clearAuthInfo();
            if (res.data?.saml2Navigation) {
                try {
                    submitSAML2Navigation(res.data.saml2Navigation);
                    return;
                } catch {}
            }
            router.push({ name: 'entrance', params: { code: entrance.value } });
            MsgSuccess(i18n.global.t('commons.msg.operationSuccess'));
        })
        .catch(() => {});
};

const loadCurrentUser = async (currentNode?: string) => {
    const authInfo = await syncAuthInfo(currentNode);
    if (authInfo) {
        currentUser.value = authInfo;
        emit('userChange', authInfo);
    }
};
const changeUserInfo = () => {
    popoverVisible.value = false;
    loadCurrentUser();
    userInfoRef.value?.openDrawer();
};

defineExpose({ openUserInfo: changeUserInfo, logout });

onMounted(() => {
    checkTask();
    loadCurrentUser();
});
</script>

<style scoped lang="scss">
@use '../index';

.el-dropdown-link {
    display: flex;
    align-items: center;
    box-sizing: border-box;
    border-top: 1px solid var(--panel-footer-border);
    height: 48px;
    .icon {
        margin-left: 25px;
        font-size: 8px;
        margin-right: 7px;
        color: var(--panel-main-bg-color-1);
    }
    &:hover {
        .icon {
            color: var(--el-color-primary);
        }
        .el-button {
            color: var(--el-color-primary);
        }
    }
}
.custom-popover-dropdown {
    padding: 0 !important;
    border: 1px solid #e4e7ed !important;
    box-shadow: 0 2px 8px 0 rgba(0, 0, 0, 0.1) !important;
    background-color: var(--el-menu-item-bg-color);
    .divider {
        display: block;
        height: 1px;
        width: 91%;
        margin: 3px 8px;
        border-top: 1px var(--el-border-color) var(--el-border-style);
    }
}

.dropdown-menu {
    min-width: 120px;
}

.dropdown-item {
    display: flex;
    align-items: center;
    gap: 6px;
    padding: 5px 8px;
    cursor: pointer;
    line-height: 26px;
    transition: background 0.3s;
    .icon {
        font-size: 8px;
    }
    .icon-status {
        font-size: 16px;
    }
    .msg-tag {
        margin-top: 3px;
        float: right;
        background-color: transparent;
        color: var(--panel-main-bg-color-1);
    }
    &:hover {
        color: var(--el-color-primary);
        .icon {
            color: var(--el-color-primary);
        }
        .msg-tag {
            color: var(--el-color-primary);
        }
    }
}
.node-name {
    flex: 1;
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
}
.more-node-button {
    margin: 4px 8px 0;
    padding: 5px 8px 5px 26px;
    border-radius: 4px;
    color: var(--el-color-primary);
    font-size: 12px;
    line-height: 24px;
}
.more-node-label {
    flex: 1;
    min-width: 0;
}
.more-node-count {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    min-width: 24px;
    height: 18px;
    padding: 0 7px;
    border-radius: 9px;
    color: var(--el-color-primary);
    line-height: 18px;
    font-size: 12px;
    font-weight: 500;
}
.more-node-arrow {
    margin-left: 2px;
    font-size: 12px;
}
.ellipsis-text {
    display: inline-block;
    max-width: 120px;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
}
</style>

<template>
    <DrawerPro v-model="drawerVisible" :header="$t('commons.button.upgrade')" @close="handleClose" size="large">
        <div class="panel-MdEditor">
            <div class="default-theme">
                <h2 class="inline-block">{{ versionTitle }}</h2>
            </div>
            <el-radio-group class="inline-block tag" v-model="upgradeVersion" @change="changeOption">
                <el-radio v-if="upgradeInfo.newVersion" :value="upgradeInfo.newVersion">
                    {{ upgradeInfo.newVersion }}
                </el-radio>
                <el-radio v-if="upgradeInfo.latestVersion" :value="upgradeInfo.latestVersion">
                    {{ upgradeInfo.latestVersion }}
                </el-radio>
                <el-radio v-if="upgradeInfo.testVersion" :value="upgradeInfo.testVersion">
                    {{ upgradeInfo.testVersion }}
                </el-radio>
            </el-radio-group>
            <div class="upgrade-proxy">
                <span class="upgrade-proxy__label">{{ $t('setting.upgradeDownloadVia') }}</span>
                <el-radio-group v-model="proxyMode" :disabled="running">
                    <el-radio value="direct">{{ $t('setting.upgradeViaDirect') }}</el-radio>
                    <el-radio value="panel" :disabled="!proxyOptions.panel">
                        {{ $t('setting.upgradeViaPanel') }}
                    </el-radio>
                    <el-radio value="node" :disabled="!proxyOptions.node">
                        {{ $t('setting.upgradeViaNode') }}
                    </el-radio>
                </el-radio-group>
                <div class="upgrade-proxy__hint">{{ proxyHint }}</div>
            </div>
            <div v-if="running" class="upgrade-progress">
                <div class="flx-align-center">
                    <el-icon class="is-loading mr-1"><Loading /></el-icon>
                    <span>{{ stageText }} {{ status?.version }}</span>
                </div>
                <el-progress class="mt-2" :percentage="percent" :stroke-width="16" text-inside />
                <div class="upgrade-meta mt-1">
                    <span>{{ computeSize(status?.done || 0) }} / {{ computeSize(status?.total || 0) }}</span>
                    <span v-if="status?.rate">{{ computeSize(status.rate) }}/s</span>
                </div>
            </div>
            <el-alert
                v-else-if="status?.stage === 'failed'"
                type="error"
                show-icon
                :closable="false"
                class="mb-2"
                :title="$t('setting.upgradeFailed', [status.version])"
                :description="status.error"
            />
            <div v-loading="loading">
                <MarkDownEditor :content="upgradeInfo.releaseNote" />
            </div>
        </div>
        <template #footer>
            <span class="dialog-footer">
                <el-button @click="drawerVisible = false">{{ $t('commons.button.cancel') }}</el-button>
                <el-button type="primary" :loading="running" :disabled="running" @click="onUpgrade">
                    {{ $t('setting.upgradeNow') }}
                </el-button>
            </span>
        </template>
    </DrawerPro>
</template>

<script setup lang="ts">
import MarkDownEditor from '@/components/mkdown-editor/index.vue';

import { loadReleaseNotes, loadUpgradeProxyOptions, loadUpgradeStatus, upgrade } from '@/api/modules/setting';
import { Setting } from '@/api/interface/setting';
import i18n, { loadLocaleMessages } from '@/lang';
import { MsgSuccess } from '@/utils/message';
import { computeSize } from '@/utils/size';
import { computed, onBeforeUnmount, ref } from 'vue';
import { useGlobalStore } from '@/composables/useGlobalStore';
import { ElMessageBox } from 'element-plus';

const { isLoading, isOnRestart, isIntl } = useGlobalStore();

const releaseNotesLocale = computed(() => (isIntl.value ? 'en' : 'zh'));
const versionTitle = computed(() => i18n.global.t('app.version', {}, { locale: releaseNotesLocale.value }));

const drawerVisible = ref(false);

// Download route for the package. Default: the subscription proxy when it
// runs, else the global proxy when set, else direct.
const PROXY_MODE_KEY = 'upgrade-download-proxy';
const proxyMode = ref('direct');
const proxyOptions = ref<Setting.UpgradeProxyOptions>({
    panel: false,
    panelAddr: '',
    node: false,
    nodeName: '',
    nodeError: '',
});
const proxyHint = computed(() => {
    switch (proxyMode.value) {
        case 'panel':
            return i18n.global.t('setting.upgradeViaPanelHint', [proxyOptions.value.panelAddr]);
        case 'node':
            return i18n.global.t('setting.upgradeViaNodeHint', [proxyOptions.value.nodeName || '-']);
        default:
            if (!proxyOptions.value.panel && !proxyOptions.value.node) {
                return i18n.global.t('setting.upgradeNoProxyHint');
            }
            return i18n.global.t('setting.upgradeViaDirectHint');
    }
});
const loadProxyOptions = async () => {
    try {
        const res = await loadUpgradeProxyOptions();
        proxyOptions.value = res.data;
    } catch {
        // older backend: keep direct only
    }
    const saved = localStorage.getItem(PROXY_MODE_KEY) || '';
    const available = (mode: string) =>
        mode === 'direct' ||
        (mode === 'panel' && proxyOptions.value.panel) ||
        (mode === 'node' && proxyOptions.value.node);
    if (saved && available(saved)) {
        proxyMode.value = saved;
    } else if (proxyOptions.value.node) {
        proxyMode.value = 'node';
    } else if (proxyOptions.value.panel) {
        proxyMode.value = 'panel';
    } else {
        proxyMode.value = 'direct';
    }
};
const upgradeInfo = ref();
const loading = ref();
const upgradeVersion = ref();

interface DialogProps {
    upgradeInfo: number;
    upgradeVersion: string;
}
const acceptParams = async (params: DialogProps): Promise<void> => {
    await loadLocaleMessages(releaseNotesLocale.value);
    upgradeInfo.value = params.upgradeInfo;
    upgradeVersion.value = params.upgradeVersion;
    drawerVisible.value = true;
    loadProxyOptions();
    // Re-attach to a running download or show the last failure.
    status.value = null;
    poll();
};

const emit = defineEmits(['search']);

const handleClose = () => {
    drawerVisible.value = false;
};

const changeOption = async () => {
    loading.value = true;
    await loadReleaseNotes(upgradeVersion.value)
        .then((res) => {
            loading.value = false;
            upgradeInfo.value.releaseNote = res.data;
        })
        .catch(() => {
            loading.value = false;
        });
};

// Download / verify happen while the panel stays usable: poll the status
// and show progress here; switch to the global restart overlay only once
// files are being replaced.
const status = ref<Setting.UpgradeStatus | null>(null);
const running = computed(() => ['downloading', 'verifying'].includes(status.value?.stage || ''));
const percent = computed(() => {
    const total = status.value?.total || 0;
    return total ? Math.min(100, Math.floor(((status.value?.done || 0) * 1000) / total) / 10) : 0;
});
const stageText = computed(() => {
    switch (status.value?.stage) {
        case 'downloading':
            return i18n.global.t('setting.upgradeDownloading');
        case 'verifying':
            return i18n.global.t('setting.upgradeVerifying');
        default:
            return '';
    }
});
let pollTimer: ReturnType<typeof setTimeout> | undefined;
const stopPoll = () => {
    if (pollTimer) clearTimeout(pollTimer);
    pollTimer = undefined;
};
const switchToRestart = () => {
    stopPoll();
    isLoading.value = true;
    isOnRestart.value = true;
    drawerVisible.value = false;
    emit('search');
};
const poll = async () => {
    stopPoll();
    try {
        const res = await loadUpgradeStatus();
        status.value = res.data;
    } catch {
        // Panel is restarting (installing finished) or briefly unreachable.
        if (status.value?.stage === 'installing') {
            switchToRestart();
            return;
        }
    }
    if (status.value?.stage === 'installing') {
        switchToRestart();
        return;
    }
    if (running.value) pollTimer = setTimeout(poll, 2000);
};

const onUpgrade = async () => {
    ElMessageBox.confirm(i18n.global.t('setting.upgradeHelper', i18n.global.t('commons.button.upgrade')), {
        confirmButtonText: i18n.global.t('commons.button.confirm'),
        cancelButtonText: i18n.global.t('commons.button.cancel'),
        type: 'info',
    }).then(async () => {
        localStorage.setItem(PROXY_MODE_KEY, proxyMode.value);
        await upgrade(upgradeVersion.value, proxyMode.value);
        MsgSuccess(i18n.global.t('setting.upgradeStarted'));
        status.value = { stage: 'downloading', version: upgradeVersion.value } as Setting.UpgradeStatus;
        poll();
    });
};

onBeforeUnmount(stopPoll);

defineExpose({
    acceptParams,
});
</script>

<style lang="scss" scoped>
.line-height {
    line-height: 25px;
}
.panel-MdEditor {
    height: calc(100vh - 330px);
    .tag {
        margin-top: -6px;
        margin-left: 20px;
        vertical-align: middle;
    }
    :deep(.md-editor-preview) {
        font-size: 14px;
    }
    :deep(.default-theme h2) {
        color: var(--el-color-primary);
        margin: 13px 0;
        padding: 0;
        font-size: 16px;
    }
}
.upgrade-proxy {
    padding: 8px 0 12px;
    &__label {
        margin-right: 12px;
        font-size: 14px;
    }
    &__hint {
        font-size: 12px;
        color: var(--el-text-color-secondary);
    }
}
.upgrade-progress {
    padding: 12px 16px;
    margin-bottom: 8px;
    border-radius: 6px;
    background: var(--el-fill-color-light);
}
.upgrade-meta {
    display: flex;
    gap: 16px;
    font-size: 12px;
    color: var(--el-text-color-secondary);
}
:deep(.md-editor-dark) {
    background-color: var(--panel-main-bg-color-9);
}
</style>

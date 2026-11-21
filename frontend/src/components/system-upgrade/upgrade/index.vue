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

import { loadReleaseNotes, loadUpgradeStatus, upgrade } from '@/api/modules/setting';
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
        await upgrade(upgradeVersion.value);
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

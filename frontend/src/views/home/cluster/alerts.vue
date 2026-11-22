<template>
    <CardWithHeader :header="$t('home.alertInfo')">
        <template #header-r>
            <el-tag v-if="summary.failingCount > 0" type="danger" size="small" effect="light" round>
                {{ $t('home.alertFailing', [summary.failingCount]) }}
            </el-tag>
            <el-tag v-else-if="summary.ruleCount > 0" type="success" size="small" effect="light" round>
                {{ $t('home.alertAllOk') }}
            </el-tag>
            <el-tooltip :content="$t('xpack.alert.alertRulesEntry')" placement="top">
                <el-button class="h-button-setting" link icon="Setting" @click="openRules" />
            </el-tooltip>
        </template>
        <template #body>
            <el-scrollbar height="286px">
                <div class="alert-list ml-5 mr-5">
                    <div v-if="!loaded" class="alert-empty">{{ $t('commons.msg.loading') }}</div>
                    <div v-else-if="summary.ruleCount === 0" class="alert-empty">
                        <span>{{ $t('home.alertNoRule') }}</span>
                        <el-button type="primary" link @click="openRules">{{ $t('home.alertCreateRule') }}</el-button>
                    </div>
                    <div v-else-if="summary.items.length === 0" class="alert-empty">
                        {{ $t('home.alertNoEvent', [summary.targetCount]) }}
                    </div>
                    <div v-for="item in summary.items" :key="item.alertId + item.targetLabel" class="alert-item">
                        <span class="status-dot" :class="item.status === 'fail' ? 'is-fail' : 'is-ok'" />
                        <div class="alert-item__main">
                            <div class="alert-item__title" :title="item.targetLabel">
                                {{ item.targetLabel }}
                            </div>
                            <div class="alert-item__detail" :title="item.detail">
                                {{ typeLabel(item.alertType) }} · {{ item.detail || '-' }}
                            </div>
                        </div>
                        <span class="alert-item__time">{{ formatTime(item.lastChangeAt) }}</span>
                    </div>
                </div>
            </el-scrollbar>
        </template>
    </CardWithHeader>
</template>

<script setup lang="ts">
import { onBeforeUnmount, onMounted, reactive, ref } from 'vue';
import CardWithHeader from '@/components/card-with-header/index.vue';
import { LoadClusterAlertSummary } from '@/api/modules/alert';
import { Alert } from '@/api/interface/alert';
import { useRouter } from 'vue-router';
import i18n from '@/lang';

const REFRESH_MS = 30000;

const router = useRouter();
const loaded = ref(false);
const summary = reactive<Alert.ClusterAlertSummary>({ ruleCount: 0, targetCount: 0, failingCount: 0, items: [] });
let timer: ReturnType<typeof setInterval> | undefined;

const typeLabel = (type: string) => {
    switch (type) {
        case 'clusterResource':
            return i18n.global.t('home.alertTypeResource');
        case 'siteProbe':
            return i18n.global.t('home.alertTypeSite');
        case 'modelApi':
            return i18n.global.t('home.alertTypeModel');
    }
    return type;
};

const pad = (n: number) => String(n).padStart(2, '0');
const formatTime = (sec: number) => {
    if (!sec) return '';
    const d = new Date(sec * 1000);
    const now = new Date();
    const time = `${pad(d.getHours())}:${pad(d.getMinutes())}`;
    return d.toDateString() === now.toDateString() ? time : `${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${time}`;
};

const openRules = () => {
    localStorage.setItem('alert-notice-tab', '0');
    router.push({ name: 'Alert', query: { type: 'clusterResource' } });
};

const load = async () => {
    try {
        const res = await LoadClusterAlertSummary();
        const data = res.data;
        summary.ruleCount = data?.ruleCount || 0;
        summary.targetCount = data?.targetCount || 0;
        summary.failingCount = data?.failingCount || 0;
        summary.items = data?.items || [];
    } catch {
        // keep the previous snapshot
    } finally {
        loaded.value = true;
    }
};

onMounted(() => {
    load();
    timer = setInterval(load, REFRESH_MS);
});

onBeforeUnmount(() => {
    if (timer) clearInterval(timer);
});
</script>

<style scoped lang="scss">
.alert-empty {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 6px;
    padding: 60px 0;
    color: var(--el-text-color-secondary);
    font-size: 13px;
}

.alert-item {
    display: flex;
    align-items: center;
    gap: 10px;
    padding: 9px 0;
    border-bottom: 1px dashed var(--el-border-color-lighter);

    &:last-child {
        border-bottom: none;
    }

    &__main {
        flex: 1;
        min-width: 0;
    }

    &__title {
        font-size: 13px;
        font-weight: 500;
        color: var(--panel-text-color);
        overflow: hidden;
        text-overflow: ellipsis;
        white-space: nowrap;
    }

    &__detail {
        margin-top: 2px;
        font-size: 12px;
        color: var(--el-text-color-secondary);
        overflow: hidden;
        text-overflow: ellipsis;
        white-space: nowrap;
    }

    &__time {
        flex: 0 0 auto;
        font-size: 12px;
        color: var(--el-text-color-secondary);
    }
}

.status-dot {
    flex: 0 0 auto;
    width: 8px;
    height: 8px;
    border-radius: 50%;

    &.is-ok {
        background: var(--el-color-success);
    }

    &.is-fail {
        background: var(--el-color-danger);
        box-shadow: 0 0 0 3px var(--el-color-danger-light-8);
    }
}
</style>

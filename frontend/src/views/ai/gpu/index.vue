<template>
    <div>
        <RouterButton :buttons="buttons" />
        <LayoutContent>
            <RouterViewCache />
        </LayoutContent>
    </div>
</template>

<script lang="ts" setup>
import i18n from '@/lang';
import { computed } from 'vue';
import { useGlobalStore } from '@/composables/useGlobalStore';

const { isMaster } = useGlobalStore();

const props = defineProps({
    historyDisabled: {
        type: Boolean,
        default: false,
    },
});

const buttons = computed(() => [
    {
        label: i18n.global.t('aiTools.gpu.current'),
        path: '/ai/gpu/current',
    },
    {
        label: i18n.global.t('aiTools.gpu.history'),
        path: '/ai/gpu/history',
        disabled: props.historyDisabled,
    },
    { label: i18n.global.t('website.groupSetting'), path: '/ai/gpu/groups' },
    // Fleet-wide scheduling hosts are managed on the master panel only.
    ...(isMaster.value ? [{ label: i18n.global.t('compute.host.pageTitle'), path: '/ai/gpu/scheduling' }] : []),
]);
</script>

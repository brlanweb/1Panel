<template>
    <RouterButton :buttons="buttons" />
</template>

<script lang="ts" setup>
import i18n from '@/lang';
import { computed } from 'vue';
import { useGlobalStore } from '@/composables/useGlobalStore';

const { isMaster } = useGlobalStore();

const buttons = computed<RouterButton[]>(() => [
    {
        label: i18n.global.t('vllm.modelPrepare'),
        path: '/ai/vllm/models',
    },
    {
        label: i18n.global.t('vllm.deployment'),
        path: '/ai/vllm/deployments',
    },
    // Multi-host deployment and plans are scheduled by the master panel.
    ...(isMaster.value ? [{ label: i18n.global.t('compute.inst.pageTitle'), path: '/ai/vllm/cluster' }] : []),
]);
</script>

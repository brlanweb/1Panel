<template>
    <RouterButton :buttons="buttons" />
</template>

<script lang="ts" setup>
import i18n from '@/lang';
import { computed } from 'vue';

const buttons = computed<RouterButton[]>(() => [
    {
        label: i18n.global.t('vllm.modelPrepare'),
        path: '/ai/vllm/models',
    },
    {
        label: i18n.global.t('vllm.deployment'),
        path: '/ai/vllm/deployments',
    },
]);
</script>

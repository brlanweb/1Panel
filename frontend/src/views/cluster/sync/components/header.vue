<template>
    <div>
        <RouterButton :buttons="buttons">
            <template #route-button>
                <el-button link type="primary" @click="historyRef.open()">
                    {{ $t('cluster.taskHistory') }}
                </el-button>
            </template>
        </RouterButton>
        <TaskHistory ref="historyRef" />
    </div>
</template>

<script lang="ts" setup>
import { ref } from 'vue';
import i18n from '@/lang';
import TaskHistory from './task-history.vue';

const historyRef = ref();

const buttons = [
    { label: i18n.global.t('xpack.sync.file'), path: '/cluster/sync/file' },
    { label: i18n.global.t('xpack.sync.image'), path: '/cluster/sync/image' },
    { label: i18n.global.t('xpack.sync.app'), path: '/cluster/sync/app' },
    { label: i18n.global.t('xpack.sync.ssl'), path: '/cluster/sync/ssl' },
    { label: i18n.global.t('xpack.sync.phpExtensionsTitle'), path: '/cluster/sync/php-extensions' },
];
</script>

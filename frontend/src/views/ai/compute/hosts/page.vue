<template>
    <div>
        <RouterMenu />
        <LayoutContent :title="$t('compute.host.pageTitle')">
            <template #rightToolBar>
                <span class="sync-hint">
                    <el-tag v-if="paused" type="info" size="small">{{ $t('compute.paused') }}</el-tag>
                    <span v-else-if="refreshedAt">{{ $t('compute.refreshedAt') }} {{ refreshedAt }}</span>
                </span>
                <el-button :icon="Refresh" @click="tick(true)">{{ $t('commons.button.refresh') }}</el-button>
            </template>
            <template #prompt>
                <el-alert type="info" :closable="false" :title="$t('compute.host.pageHelper')" />
            </template>
            <template #main>
                <HostsTab ref="tabRef" />
            </template>
        </LayoutContent>
    </div>
</template>

<script lang="ts" setup>
import { ref } from 'vue';
import { Refresh } from '@element-plus/icons-vue';
import RouterMenu from '@/views/ai/gpu/index.vue';
import HostsTab from './index.vue';
import { useComputePoll } from '../poll';

// GPU → 调度主机: the machines the compute pools may place replicas on
// (terminal hosts / cluster nodes / this panel), with the "参与调度"
// switch and the optical-link topology. Master panel only.
const tabRef = ref<any>();
const { paused, refreshedAt, tick } = useComputePoll(tabRef);
</script>

<style scoped>
.sync-hint {
    font-size: 12px;
    color: var(--el-text-color-secondary);
    margin-right: 12px;
}
</style>

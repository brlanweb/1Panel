<template>
    <div>
        <RouterMenu />
        <LayoutContent :title="$t('compute.inst.pageTitle')">
            <template #leftToolBar>
                <el-radio-group v-model="tab">
                    <el-radio-button value="instances">{{ $t('compute.inst.tabReplicas') }}</el-radio-button>
                    <el-radio-button value="plans">{{ $t('compute.inst.tabPlans') }}</el-radio-button>
                </el-radio-group>
            </template>
            <template #rightToolBar>
                <span class="sync-hint">
                    <el-tag v-if="paused" type="info" size="small">{{ $t('compute.paused') }}</el-tag>
                    <span v-else-if="refreshedAt">{{ $t('compute.refreshedAt') }} {{ refreshedAt }}</span>
                </span>
                <el-button :icon="Refresh" @click="tick(true)">{{ $t('commons.button.refresh') }}</el-button>
            </template>
            <template #prompt>
                <el-alert type="info" :closable="false" :title="$t('compute.inst.pageHelper')" />
            </template>
            <template #main>
                <InstancesTab v-if="tab === 'instances'" ref="tabRef" />
                <TemplatesTab v-else ref="tabRef" />
            </template>
        </LayoutContent>
    </div>
</template>

<script lang="ts" setup>
import { ref, watch } from 'vue';
import { Refresh } from '@element-plus/icons-vue';
import RouterMenu from '@/views/ai/vllm/index.vue';
import InstancesTab from './index.vue';
import TemplatesTab from '../templates/index.vue';
import { useComputePoll } from '../poll';

// vLLM → 集群部署: manual deployment across scheduling hosts (single host
// or a Ray group over the optical link) and the deployment plans
// (engine runtime + model profile) the compute pools scale with.
const tab = ref(localStorage.getItem('compute-cluster-tab') || 'instances');
const tabRef = ref<any>();
const { paused, refreshedAt, tick, kick } = useComputePoll(tabRef);

watch(tab, (v) => {
    localStorage.setItem('compute-cluster-tab', v);
    kick();
});
</script>

<style scoped>
.sync-hint {
    font-size: 12px;
    color: var(--el-text-color-secondary);
    margin-right: 12px;
}
</style>

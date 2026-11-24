<template>
    <DialogPro v-model="visible" :title="$t('vllm.sendToNode')" size="large" @close="visible = false">
        <el-alert :title="$t('vllm.sendToNodeHelper')" type="info" :closable="false" show-icon />
        <el-descriptions class="mt-3" :column="1" border>
            <el-descriptions-item :label="$t('vllm.repoId')">{{ row?.repoId }}</el-descriptions-item>
            <el-descriptions-item :label="$t('vllm.dir')">{{ row?.dir }}</el-descriptions-item>
            <el-descriptions-item :label="$t('file.size')">
                {{ row?.size ? computeSize(row.size) : '-' }}
            </el-descriptions-item>
            <el-descriptions-item :label="$t('cluster.sourceNode')">{{ sourceName }}</el-descriptions-item>
        </el-descriptions>
        <div class="mt-4 font-medium">{{ $t('xpack.sync.targetTitle') }}</div>
        <div v-loading="nodesLoading">
            <el-checkbox-group v-if="candidates.length" v-model="targets" class="target-grid mt-2">
                <el-checkbox v-for="node in candidates" :key="node.key" :value="node.key" border class="target-item">
                    <div class="target-item__body">
                        <span class="target-item__name">{{ node.name }}</span>
                        <el-tag size="small" :type="nodeStatusType(node.status)">
                            {{ nodeStatusLabel(node.status) }}
                        </el-tag>
                    </div>
                    <div v-if="node.addr" class="target-item__addr">{{ node.addr }}</div>
                </el-checkbox>
            </el-checkbox-group>
            <el-empty v-else :image-size="60" :description="$t('xpack.sync.noTargetNodes')" />
        </div>
        <template #footer>
            <el-button @click="visible = false">{{ $t('commons.button.cancel') }}</el-button>
            <el-button
                type="primary"
                :loading="submitting"
                :disabled="!targets.length || sourceId === undefined"
                @click="onSubmit"
            >
                {{ $t('commons.button.confirm') }}
            </el-button>
        </template>
    </DialogPro>
    <TaskProgress ref="progressRef" />
</template>

<script lang="ts" setup>
import { computed, ref } from 'vue';
import i18n from '@/lang';
import { Vllm } from '@/api/interface/vllm';
import { executeClusterSync } from '@/api/modules/cluster';
import { computeSize } from '@/utils/size';
import { MsgError, MsgSuccess } from '@/utils/message';
import { useGlobalStore } from '@/composables/useGlobalStore';
import TaskProgress from '@/views/cluster/sync/components/task-progress.vue';
import { LOCAL_KEY, nodeStatusLabel, nodeStatusType, useSyncNodes } from '@/views/cluster/sync/common';

const { globalStore } = useGlobalStore();
const { nodes, nodesLoading, loadNodes } = useSyncNodes();

const visible = ref(false);
const submitting = ref(false);
const row = ref<Vllm.Model>();
const targets = ref<string[]>([]);
const progressRef = ref();

// The model lives on the node this page is operating on: the master
// ('local') or a cluster node selected by name in the node switcher.
const sourceNode = computed(() => {
    const current = globalStore.currentNode || LOCAL_KEY;
    if (current === LOCAL_KEY) return nodes.value.find((n) => n.key === LOCAL_KEY);
    return nodes.value.find((n) => n.key !== LOCAL_KEY && n.name === current);
});
const sourceId = computed(() => sourceNode.value?.id);
const sourceName = computed(() => sourceNode.value?.name || globalStore.currentNode);
const candidates = computed(() => nodes.value.filter((n) => n.key !== sourceNode.value?.key && n.status !== 'offline'));

const acceptParams = async (model: Vllm.Model) => {
    row.value = model;
    targets.value = [];
    visible.value = true;
    await loadNodes();
};

const onSubmit = async () => {
    if (!row.value || sourceId.value === undefined) {
        MsgError(i18n.global.t('vllm.sendSourceUnknown'));
        return;
    }
    submitting.value = true;
    try {
        const ids = targets.value
            .map((key) => nodes.value.find((n) => n.key === key)?.id)
            .filter((id) => id !== undefined);
        const res = await executeClusterSync({
            kind: 'model',
            sourceNodeId: sourceId.value,
            targetNodeIds: ids,
            resourceKeys: [row.value.dir],
            overwriteConflicts: false,
        });
        visible.value = false;
        MsgSuccess(i18n.global.t('xpack.sync.submitSuccess'));
        progressRef.value.open(res.data.taskId);
    } finally {
        submitting.value = false;
    }
};

defineExpose({ acceptParams });
</script>

<style scoped lang="scss">
.target-grid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(240px, 1fr));
    gap: 12px;
}
.target-item {
    height: auto;
    margin-right: 0 !important;
    padding: 8px 12px;
    align-items: flex-start;
    :deep(.el-checkbox__label) {
        width: 100%;
        min-width: 0;
    }
}
.target-item__body {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 8px;
}
.target-item__name {
    font-weight: 500;
    overflow: hidden;
    text-overflow: ellipsis;
}
.target-item__addr {
    font-size: 12px;
    color: var(--el-text-color-secondary);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
}
</style>

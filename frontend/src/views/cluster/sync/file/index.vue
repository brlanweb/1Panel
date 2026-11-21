<template>
    <div>
        <SyncHeader />
        <el-row :gutter="16" class="mt-3" v-loading="nodesLoading">
            <el-col :xs="24" :md="12">
                <FilePane
                    ref="leftRef"
                    side="left"
                    v-model="leftNode"
                    :nodes="nodes"
                    :peer-node="rightNode"
                    :node-id="nodeId"
                    @transfer="(row) => onTransfer('left', row)"
                />
            </el-col>
            <el-col :xs="24" :md="12">
                <FilePane
                    ref="rightRef"
                    side="right"
                    v-model="rightNode"
                    :nodes="nodes"
                    :peer-node="leftNode"
                    :node-id="nodeId"
                    @transfer="(row) => onTransfer('right', row)"
                />
            </el-col>
        </el-row>
        <TaskProgress ref="progressRef" @finished="refreshBoth" />
    </div>
</template>

<script lang="ts" setup>
import { onMounted, ref } from 'vue';
import { ElMessageBox } from 'element-plus';
import i18n from '@/lang';
import { executeClusterSync } from '@/api/modules/cluster';
import { Cluster } from '@/api/interface/cluster';
import { MsgSuccess } from '@/utils/message';
import SyncHeader from '../components/header.vue';
import TaskProgress from '../components/task-progress.vue';
import FilePane from './file-pane.vue';
import { LOCAL_KEY, useSyncNodes } from '../common';

defineOptions({ name: 'ClusterSyncFile' });

const { nodes, nodesLoading, loadNodes, nodeName, nodeId } = useSyncNodes();
const leftNode = ref('');
const rightNode = ref('');
const leftRef = ref();
const rightRef = ref();
const progressRef = ref();

const refreshBoth = () => {
    leftRef.value?.load();
    rightRef.value?.load();
};

// The clicked row is transferred into the directory currently open in the
// other pane (file or whole directory).
const onTransfer = (side: 'left' | 'right', row: Cluster.SyncResource) => {
    const sourceKey = side === 'left' ? leftNode.value : rightNode.value;
    const targetKey = side === 'left' ? rightNode.value : leftNode.value;
    const targetPane = side === 'left' ? rightRef.value : leftRef.value;
    if (!sourceKey || !targetKey) return;
    const targetPath = targetPane?.currentPath() || '/';
    ElMessageBox.confirm(
        i18n.global.t('xpack.sync.fileSyncConfirm', [nodeName(sourceKey), row.key, nodeName(targetKey), targetPath]),
        i18n.global.t('xpack.sync.file'),
        {
            confirmButtonText: i18n.global.t('commons.button.confirm'),
            cancelButtonText: i18n.global.t('commons.button.cancel'),
            type: 'info',
        },
    ).then(async () => {
        const res = await executeClusterSync({
            kind: 'file',
            sourceNodeId: nodeId(sourceKey),
            targetNodeIds: [nodeId(targetKey)],
            resourceKeys: [row.key],
            path: row.detail || '/',
            targetPath,
            overwriteConflicts: false,
        });
        MsgSuccess(i18n.global.t('xpack.sync.submitSuccess'));
        progressRef.value.open(res.data.taskId);
    });
};

onMounted(async () => {
    await loadNodes();
    leftNode.value = LOCAL_KEY;
    const other = nodes.value.find((node) => node.key !== LOCAL_KEY && node.status !== 'offline');
    if (other) rightNode.value = other.key;
});
</script>

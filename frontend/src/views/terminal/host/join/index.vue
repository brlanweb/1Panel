<template>
    <DrawerPro
        v-model="visible"
        :header="$t('cluster.joinCluster')"
        :resource="host.name ? `${host.name} [${host.addr}]` : host.addr"
        @close="onClose"
        size="large"
    >
        <el-alert :title="$t('cluster.joinHelper')" type="info" :closable="false" show-icon class="p-mb-2" />
        <el-form ref="formRef" :model="form" label-position="top" :disabled="started">
            <el-row :gutter="20">
                <el-col :span="12">
                    <el-form-item :label="$t('cluster.nodeName')" prop="name">
                        <el-input v-model="form.name" :placeholder="host.name || host.addr" />
                    </el-form-item>
                </el-col>
                <el-col :span="12">
                    <el-form-item :label="$t('cluster.group')" prop="groupId">
                        <el-select v-model="form.groupId" class="p-w-100">
                            <el-option :label="$t('commons.table.all')" :value="0" />
                            <el-option v-for="item in groups" :key="item.id" :label="item.name" :value="item.id" />
                        </el-select>
                    </el-form-item>
                </el-col>
                <el-col :span="12">
                    <el-form-item :label="$t('cluster.joinMasterIp')" prop="masterIp">
                        <el-input v-model="form.masterIp" :placeholder="$t('cluster.joinMasterIpPlaceholder')" />
                        <span class="input-help">{{ $t('cluster.joinMasterIpHelper') }}</span>
                    </el-form-item>
                </el-col>
                <el-col :span="12">
                    <el-form-item :label="$t('cluster.joinEntryHost')" prop="entryHost">
                        <el-input v-model="form.entryHost" :placeholder="host.addr" />
                        <span class="input-help">{{ $t('cluster.joinEntryHostHelper') }}</span>
                    </el-form-item>
                </el-col>
            </el-row>
            <el-form-item>
                <el-checkbox v-model="form.installDocker">{{ $t('cluster.joinInstallDocker') }}</el-checkbox>
            </el-form-item>
        </el-form>

        <div v-if="started">
            <el-steps :active="activeStep" :process-status="status.stage === 'failed' ? 'error' : 'process'" simple>
                <el-step v-for="item in steps" :key="item" :title="$t('cluster.joinStage.' + item)" />
            </el-steps>
            <el-progress
                v-if="status.running && status.total > 0"
                class="p-mt-2"
                :percentage="Math.min(100, Math.floor((status.done / status.total) * 100))"
            />
            <el-alert
                v-if="status.stage === 'done'"
                class="p-mt-2"
                type="success"
                :closable="false"
                show-icon
                :title="$t('cluster.joinSuccess', [status.nodeName])"
            >
                <div>{{ status.entryUrl }}</div>
                <div v-if="status.installed && status.username">
                    {{ $t('cluster.joinInitialLogin', [status.username, status.password]) }}
                </div>
            </el-alert>
            <el-alert
                v-if="status.stage === 'failed'"
                class="p-mt-2"
                type="error"
                :closable="false"
                show-icon
                :title="status.error"
            />
            <div ref="logRef" class="join-log p-mt-2">
                <div v-for="(line, i) in status.logs" :key="i">{{ line }}</div>
            </div>
        </div>

        <template #footer>
            <el-button @click="visible = false">{{ $t('commons.button.cancel') }}</el-button>
            <el-button v-if="status.stage === 'done'" type="primary" @click="goNodes">
                {{ $t('cluster.joinGoNodes') }}
            </el-button>
            <el-button v-else type="primary" :loading="status.running || submitting" @click="onSubmit">
                {{ status.stage === 'failed' ? $t('commons.button.retry') : $t('cluster.joinCluster') }}
            </el-button>
        </template>
    </DrawerPro>
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, reactive, ref } from 'vue';
import { useRouter } from 'vue-router';
import { ElMessageBox } from 'element-plus';
import i18n from '@/lang';
import { Host } from '@/api/interface/host';
import { Cluster } from '@/api/interface/cluster';
import { Group } from '@/api/interface/group';
import { getGroupList } from '@/api/modules/group';
import { joinClusterNode, loadClusterNodeJoinStatus } from '@/api/modules/cluster';

const emit = defineEmits(['search']);
const router = useRouter();

const visible = ref(false);
const submitting = ref(false);
const started = ref(false);
const host = ref<Partial<Host.Host>>({});
const groups = ref<Group.GroupInfo[]>([]);
const logRef = ref<HTMLElement>();
const form = reactive({ name: '', groupId: 0, masterIp: '', entryHost: '', installDocker: false });
const emptyStatus = (): Cluster.NodeJoinStatus => ({
    hostId: 0,
    stage: '',
    running: false,
    error: '',
    done: 0,
    total: 0,
    installed: false,
    nodeId: 0,
    nodeName: '',
    entryUrl: '',
    username: '',
    password: '',
    logs: [],
});
const status = ref<Cluster.NodeJoinStatus>(emptyStatus());

// The last stage reached before a failure (the backend reports "failed").
const lastStage = ref('');
const installStages = ['download', 'upload', 'install'];
const steps = computed(() => {
    // Install steps only show when this join installs the panel.
    const showInstall = status.value.installed || installStages.includes(lastStage.value);
    return ['connect', ...(showInstall ? installStages : []), 'key', 'register', 'done'];
});
const activeStep = computed(() => {
    if (status.value.stage === 'done') {
        return steps.value.length;
    }
    const stage = status.value.stage === 'failed' ? lastStage.value : status.value.stage;
    return Math.max(0, steps.value.indexOf(stage));
});

let timer: ReturnType<typeof setTimeout> | undefined;
const stopPoll = () => {
    if (timer) {
        clearTimeout(timer);
        timer = undefined;
    }
};

const poll = async () => {
    stopPoll();
    if (!visible.value || !host.value.id) {
        return;
    }
    try {
        const res = await loadClusterNodeJoinStatus(host.value.id);
        const atBottom =
            !logRef.value || logRef.value.scrollHeight - logRef.value.scrollTop - logRef.value.clientHeight < 40;
        if (res.data.stage && res.data.stage !== 'failed') {
            lastStage.value = res.data.stage;
        }
        status.value = { ...emptyStatus(), ...res.data, logs: res.data.logs || [] };
        if (status.value.stage) {
            started.value = true;
        }
        if (atBottom) {
            nextTick(() => logRef.value && (logRef.value.scrollTop = logRef.value.scrollHeight));
        }
        if (status.value.running) {
            timer = setTimeout(poll, 1500);
        } else if (status.value.stage === 'done') {
            emit('search');
        }
    } catch {
        timer = setTimeout(poll, 3000);
    }
};

const acceptParams = async (row: Host.Host) => {
    host.value = row;
    form.name = '';
    form.groupId = 0;
    form.masterIp = '';
    form.entryHost = '';
    form.installDocker = false;
    status.value = emptyStatus();
    started.value = false;
    lastStage.value = '';
    visible.value = true;
    getGroupList('cluster_node')
        .then((res) => (groups.value = res.data || []))
        .catch(() => undefined);
    // Re-open an earlier join of this host (still running or finished).
    await poll();
};

const onSubmit = async () => {
    try {
        await ElMessageBox.confirm(i18n.global.t('cluster.joinConfirm'), i18n.global.t('cluster.joinCluster'), {
            confirmButtonText: i18n.global.t('commons.button.confirm'),
            cancelButtonText: i18n.global.t('commons.button.cancel'),
            type: 'info',
        });
    } catch {
        return;
    }
    submitting.value = true;
    try {
        await joinClusterNode({
            hostId: host.value.id,
            name: form.name.trim(),
            groupId: form.groupId,
            masterIp: form.masterIp.trim(),
            entryHost: form.entryHost.trim(),
            installDocker: form.installDocker,
        });
        started.value = true;
        lastStage.value = '';
        status.value = { ...emptyStatus(), stage: 'connect', running: true };
        timer = setTimeout(poll, 800);
    } finally {
        submitting.value = false;
    }
};

const goNodes = () => {
    visible.value = false;
    router.push({ name: 'ClusterNodes' });
};

const onClose = () => {
    stopPoll();
};

onBeforeUnmount(stopPoll);

defineExpose({ acceptParams });
</script>

<style scoped lang="scss">
.join-log {
    height: 320px;
    overflow: auto;
    padding: 8px 12px;
    font-family: Menlo, Monaco, Consolas, monospace;
    font-size: 12px;
    line-height: 1.6;
    white-space: pre-wrap;
    word-break: break-all;
    color: #d4d4d4;
    background: #1e1e1e;
    border-radius: 4px;
}
</style>

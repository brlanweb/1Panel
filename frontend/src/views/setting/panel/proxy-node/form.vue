<template>
    <div>
        <el-form ref="formRef" label-position="top" :model="form" :rules="rules" v-loading="loading">
            <el-form-item :label="$t('proxyNode.subscription')" prop="subscriptionUrl">
                <el-input
                    v-model.trim="form.subscriptionUrl"
                    type="password"
                    show-password
                    placeholder="https://example.com/api/v1/client/subscribe?token=..."
                />
                <span class="input-help">{{ $t('proxyNode.subscriptionHelper') }}</span>
            </el-form-item>

            <el-form-item :label="$t('proxyNode.node')">
                <div class="node-select-row">
                    <el-select
                        v-model="selected"
                        filterable
                        class="node-select"
                        :disabled="!status.nodes.length"
                        :placeholder="$t('proxyNode.nodePlaceholder')"
                        popper-class="proxy-node-popper"
                        @change="onSelect"
                    >
                        <el-option value="DIRECT" :label="$t('proxyNode.direct')" />
                        <el-option v-for="item in sortedNodes" :key="item.name" :value="item.name" :label="item.name">
                            <div class="node-option">
                                <span class="node-option-name">{{ item.name }}</span>
                                <span class="node-option-metrics">
                                    <span :class="delayClass(item)">{{ delayText(item) }}</span>
                                    <span class="node-option-speed">{{ speedText(item) }}</span>
                                </span>
                            </div>
                        </el-option>
                    </el-select>
                    <el-button
                        :icon="Odometer"
                        :loading="status.testing"
                        :disabled="!status.nodes.length || !status.running"
                        @click="onTest"
                    >
                        {{
                            status.testing
                                ? $t('proxyNode.testing', [status.testDone, status.testTotal])
                                : $t('proxyNode.test')
                        }}
                    </el-button>
                </div>
                <span class="input-help" v-if="status.nodes.length">
                    {{ $t('proxyNode.nodeCount', [status.nodes.length]) }}
                    <template v-if="status.testedAt">· {{ $t('proxyNode.testedAt') }} {{ status.testedAt }}</template>
                </span>
                <el-progress
                    v-if="status.testing && status.testTotal"
                    class="mt-1 w-full"
                    :percentage="Math.round((status.testDone / status.testTotal) * 100)"
                    :stroke-width="4"
                    :show-text="false"
                />
                <el-text v-if="status.message" type="danger" size="small">{{ status.message }}</el-text>
            </el-form-item>

            <el-row :gutter="12">
                <el-col :span="14">
                    <el-form-item :label="$t('proxyNode.image')" prop="image">
                        <el-input v-model.trim="form.image" placeholder="metacubex/mihomo:latest" />
                    </el-form-item>
                </el-col>
                <el-col :span="10">
                    <el-form-item :label="$t('proxyNode.port')" prop="port">
                        <el-input-number v-model="form.port" :min="1024" :max="65534" controls-position="right" />
                        <span class="input-help">{{ $t('proxyNode.portHelper') }}</span>
                    </el-form-item>
                </el-col>
            </el-row>

            <template v-if="status.subscriptionUrl">
                <el-form-item :label="$t('commons.table.status')">
                    <el-tag :type="status.running ? 'success' : 'info'">
                        {{ status.running ? $t('proxyNode.running') : $t('proxyNode.stopped') }}
                    </el-tag>
                    <el-button class="ml-2" link type="primary" @click="onOperate(status.running ? 'stop' : 'start')">
                        {{ status.running ? $t('proxyNode.stop') : $t('proxyNode.start') }}
                    </el-button>
                    <el-button link type="primary" :disabled="loading" @click="onRefresh">
                        {{ $t('proxyNode.refresh') }}
                    </el-button>
                </el-form-item>
                <el-form-item :label="$t('proxyNode.proxyAddress')">
                    <el-input :model-value="status.proxyUrl" readonly type="password" show-password>
                        <template #append><CopyButton :content="status.proxyUrl" /></template>
                    </el-input>
                    <span class="input-help">
                        {{ $t('proxyNode.containerAddress') }}: {{ status.containerName }}:7890
                    </span>
                </el-form-item>
            </template>
        </el-form>
    </div>
</template>

<script lang="ts" setup>
import { computed, onMounted, onUnmounted, reactive, ref } from 'vue';
import { FormInstance } from 'element-plus';
import { Odometer } from '@element-plus/icons-vue';
import i18n from '@/lang';
import { Rules } from '@/global/form-rules';
import { MsgSuccess } from '@/utils/message';
import { computeSize } from '@/utils/size';
import { ProxyNode } from '@/api/interface/proxy-node';
import {
    loadProxyNodeStatus,
    operateProxyNode,
    refreshProxyNode,
    saveProxyNode,
    selectProxyNode,
    testProxyNodes,
} from '@/api/modules/proxy-node';

const emit = defineEmits(['search']);

// active while the hosting drawer is open (controls status polling)
const open = ref(false);
const loading = ref(false);
const formRef = ref<FormInstance>();
const selected = ref('');
const status = reactive<ProxyNode.Status>({
    subscriptionUrl: '',
    image: '',
    port: 7897,
    selected: '',
    containerName: '',
    running: false,
    proxyUrl: '',
    nodes: [],
    testing: false,
    testDone: 0,
    testTotal: 0,
    testedAt: '',
    updatedAt: '',
    message: '',
});
const form = reactive({ subscriptionUrl: '', image: 'metacubex/mihomo:latest', port: 7897 });
const rules = reactive({
    subscriptionUrl: [Rules.requiredInput],
});

let pollTimer: ReturnType<typeof setTimeout> | undefined;

// Fastest first: tested & reachable by delay, then untested, then failed.
const sortedNodes = computed(() => {
    const rank = (item: ProxyNode.Item) => (item.delay > 0 ? 0 : item.delay === 0 ? 1 : 2);
    return [...status.nodes].sort((a, b) => rank(a) - rank(b) || (a.delay || 0) - (b.delay || 0));
});

const delayText = (item: ProxyNode.Item) => {
    if (item.delay > 0) return `${item.delay} ms`;
    if (item.delay < 0) return i18n.global.t('proxyNode.timeout');
    return status.testing ? '…' : i18n.global.t('proxyNode.untested');
};
const delayClass = (item: ProxyNode.Item) => {
    if (item.delay < 0) return 'metric-bad';
    if (item.delay === 0) return 'metric-muted';
    if (item.delay < 300) return 'metric-good';
    if (item.delay < 800) return 'metric-mid';
    return 'metric-bad';
};
const speedText = (item: ProxyNode.Item) => {
    if (item.speed > 0) return `${computeSize(Math.round(item.speed))}/s`;
    if (item.speed < 0) return '—';
    return item.delay > 0 && status.testing ? '…' : '';
};

const applyStatus = (data: ProxyNode.Status) => {
    Object.assign(status, data);
    status.nodes = data.nodes || [];
    if (!selected.value || selected.value === status.selected || !open.value) {
        selected.value = data.selected || '';
    }
};

const loadStatus = async () => {
    const res = await loadProxyNodeStatus();
    applyStatus(res.data);
    return res.data;
};

// Poll while the background speed test runs; the dropdown fills in as
// results arrive and the user can keep working meanwhile.
const schedulePoll = () => {
    clearTimeout(pollTimer);
    if (!open.value || !status.testing) return;
    pollTimer = setTimeout(async () => {
        try {
            await loadStatus();
        } catch {
            // ignore, retry next round
        }
        schedulePoll();
    }, 1500);
};

const load = async () => {
    open.value = true;
    loading.value = true;
    try {
        const data = await loadStatus();
        form.subscriptionUrl = data.subscriptionUrl || '';
        form.image = data.image || 'metacubex/mihomo:latest';
        form.port = data.port || 7897;
        selected.value = data.selected || '';
    } finally {
        loading.value = false;
    }
    schedulePoll();
};

const save = async (): Promise<boolean> => {
    if (!formRef.value) return false;
    try {
        await formRef.value.validate();
    } catch {
        return false;
    }
    loading.value = true;
    try {
        await saveProxyNode({
            subscriptionUrl: form.subscriptionUrl,
            image: form.image,
            port: form.port,
            selected: selected.value,
        });
        MsgSuccess(i18n.global.t('commons.msg.operationSuccess'));
        await loadStatus();
        emit('search');
    } finally {
        loading.value = false;
    }
    schedulePoll();
    return true;
};

const onRefresh = async () => {
    loading.value = true;
    try {
        await refreshProxyNode();
        await loadStatus();
        MsgSuccess(i18n.global.t('commons.msg.operationSuccess'));
    } finally {
        loading.value = false;
    }
    schedulePoll();
};

const onSelect = async (name: string) => {
    if (!name) return;
    try {
        await selectProxyNode(name);
        status.selected = name;
        MsgSuccess(i18n.global.t('proxyNode.selectSuccess', [name]));
    } catch {
        selected.value = status.selected;
    }
};

const onTest = async () => {
    await testProxyNodes();
    status.testing = true;
    schedulePoll();
};

const onOperate = async (operate: 'start' | 'stop') => {
    loading.value = true;
    try {
        await operateProxyNode(operate);
        await loadStatus();
        MsgSuccess(i18n.global.t('commons.msg.operationSuccess'));
    } finally {
        loading.value = false;
    }
};

const stop = () => {
    open.value = false;
    clearTimeout(pollTimer);
};

onMounted(() => {
    void load();
});
onUnmounted(() => clearTimeout(pollTimer));

defineExpose({ load, save, stop, loading });
</script>

<style scoped>
.mb-4 {
    margin-bottom: 16px;
}

.mt-1 {
    margin-top: 4px;
}

.ml-2 {
    margin-left: 8px;
}

.node-select-row {
    display: flex;
    gap: 8px;
    width: 100%;
}

.node-select {
    flex: 1;
}

.footer-help {
    margin-right: 12px;
}
</style>

<style>
.proxy-node-popper .node-option {
    display: flex;
    justify-content: space-between;
    gap: 16px;
}

.proxy-node-popper .node-option-name {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
}

.proxy-node-popper .node-option-metrics {
    display: flex;
    gap: 12px;
    font-size: 12px;
    flex-shrink: 0;
    font-variant-numeric: tabular-nums;
}

.proxy-node-popper .node-option-speed {
    min-width: 72px;
    text-align: right;
    color: var(--el-text-color-secondary);
}

.proxy-node-popper .metric-good {
    color: var(--el-color-success);
}

.proxy-node-popper .metric-mid {
    color: var(--el-color-warning);
}

.proxy-node-popper .metric-bad {
    color: var(--el-color-danger);
}

.proxy-node-popper .metric-muted {
    color: var(--el-text-color-placeholder);
}
</style>

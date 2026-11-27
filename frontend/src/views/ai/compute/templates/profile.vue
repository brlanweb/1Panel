<template>
    <DrawerPro
        v-model="visible"
        :header="$t('compute.tpl.profile')"
        :resource="form.name"
        size="large"
        @close="visible = false"
    >
        <el-steps :active="stepIndex" finish-status="success" simple class="mb-4">
            <el-step :title="$t('compute.wiz.stepModel')" />
            <el-step :title="$t('compute.wiz.stepHosts')" />
            <el-step :title="$t('compute.wiz.stepEngine')" />
        </el-steps>
        <el-form v-loading="loading" label-position="top" :model="form">
            <!-- 1. model -->
            <el-form-item :label="$t('compute.wiz.model')" required>
                <el-select
                    v-model="modelKey"
                    filterable
                    allow-create
                    default-first-option
                    class="w-full"
                    :placeholder="$t('compute.wiz.modelPlaceholder')"
                    @change="onModel"
                >
                    <el-option v-for="m in models" :key="m.repoId" :label="m.repoId" :value="m.repoId">
                        <div class="opt">
                            <span>{{ m.repoId }}</span>
                            <span class="sub">
                                {{ (m.hosts || []).map((h) => (h.hostId === 0 ? masterLabel : h.hostName)).join(', ') }}
                            </span>
                        </div>
                    </el-option>
                </el-select>
                <span class="input-help">
                    <template v-if="selected">
                        {{ $t('compute.wiz.modelOn', [hostsWithModel.length]) }}
                        <span class="mono">{{ form.modelPath }}</span>
                        <template v-if="selected.recipe">
                            · {{ $t('compute.wiz.recipe', [selected.recipe.name]) }}
                        </template>
                    </template>
                    <template v-else>{{ $t('compute.wiz.modelHelper') }}</template>
                </span>
                <el-alert v-for="e in scanErrors" :key="e" class="mt-1" type="warning" :closable="false" :title="e" />
            </el-form-item>
            <el-row :gutter="12">
                <el-col :span="12">
                    <el-form-item :label="$t('compute.servedName')" required>
                        <el-input v-model="form.servedName" placeholder="deepseek-v4-flash-0731" />
                        <span class="input-help">{{ $t('compute.wiz.servedHelper') }}</span>
                    </el-form-item>
                </el-col>
                <el-col :span="12">
                    <el-form-item :label="$t('commons.table.name')" required>
                        <el-input v-model="form.name" :placeholder="defaultName" />
                    </el-form-item>
                </el-col>
            </el-row>
            <el-form-item :label="$t('compute.tpl.modelPath')" required>
                <el-input v-model="form.modelPath" placeholder="/models/DeepSeek-V4-Flash-0731" />
                <span class="input-help">{{ $t('compute.wiz.pathHelper') }}</span>
            </el-form-item>

            <!-- 2. hosts -->
            <el-divider content-position="left">{{ $t('compute.wiz.stepHosts') }}</el-divider>
            <el-form-item :label="$t('compute.wiz.layout')">
                <el-radio-group v-model="layout" @change="onLayout">
                    <el-radio-button value="single">{{ $t('compute.wiz.layoutSingle') }}</el-radio-button>
                    <el-radio-button value="multi">{{ $t('compute.wiz.layoutMulti') }}</el-radio-button>
                </el-radio-group>
                <span class="input-help">{{ $t('compute.wiz.layoutHelper_' + layout) }}</span>
            </el-form-item>
            <el-form-item :label="$t('compute.wiz.hosts')" :required="layout === 'multi'">
                <el-table
                    ref="hostTable"
                    :data="hostRows"
                    row-key="id"
                    max-height="320"
                    size="small"
                    @selection-change="onHostSelect"
                >
                    <el-table-column type="selection" width="40" :selectable="(r: HostRow) => r.selectable" />
                    <el-table-column :label="$t('commons.table.name')" min-width="140">
                        <template #default="{ row }">
                            <span>{{ row.name }}</span>
                            <el-tag v-if="row.id === headId" size="small" type="warning" class="ml-1">
                                {{ $t('compute.wiz.head') }}
                            </el-tag>
                            <div class="sub">{{ row.domain || row.addr }}</div>
                        </template>
                    </el-table-column>
                    <el-table-column :label="$t('compute.wiz.accel')" min-width="130">
                        <template #default="{ row }">
                            <el-tag size="small" effect="plain" :type="row.kind === 'npu' ? 'warning' : 'primary'">
                                {{ row.kind.toUpperCase() }}
                            </el-tag>
                            <span class="sub ml-1">{{ row.accel }}</span>
                        </template>
                    </el-table-column>
                    <el-table-column :label="$t('compute.wiz.freeMem')" width="110">
                        <template #default="{ row }">{{ row.freeMem }}</template>
                    </el-table-column>
                    <el-table-column :label="$t('compute.wiz.model')" width="120">
                        <template #default="{ row }">
                            <el-tag v-if="row.hasModel" size="small" type="success" effect="plain">
                                {{ $t('compute.wiz.hasModel') }}
                            </el-tag>
                            <el-tag v-else-if="selected" size="small" type="info" effect="plain">
                                {{ $t('compute.wiz.willSync') }}
                            </el-tag>
                            <span v-else class="sub">—</span>
                        </template>
                    </el-table-column>
                    <el-table-column :label="$t('commons.table.status')" min-width="140">
                        <template #default="{ row }">
                            <span :class="row.selectable ? 'sub' : 'stat-danger'">{{ row.state }}</span>
                        </template>
                    </el-table-column>
                    <el-table-column v-if="layout === 'multi'" :label="$t('compute.wiz.head')" width="70">
                        <template #default="{ row }">
                            <el-radio v-model="headId" :value="row.id" :disabled="!picked.includes(row.id)" @click.stop>
                                &nbsp;
                            </el-radio>
                        </template>
                    </el-table-column>
                </el-table>
                <span class="input-help">{{ $t('compute.wiz.hostsHelper_' + layout) }}</span>
                <el-alert
                    v-if="layout === 'multi' && picked.length > 1 && !sameDomain"
                    class="mt-1"
                    type="warning"
                    :closable="false"
                    :title="$t('compute.wiz.noFabric')"
                />
            </el-form-item>
            <el-row :gutter="12">
                <el-col :span="8">
                    <el-form-item :label="$t('compute.tpl.minMem')" :required="(form.maxPerHost || 1) > 1">
                        <el-input-number v-model="form.minMemGb" :min="0" :max="2048" controls-position="right" />
                        <span class="input-help">{{ $t('compute.wiz.memHelper') }}</span>
                    </el-form-item>
                </el-col>
                <el-col v-if="layout === 'single'" :span="8">
                    <el-form-item :label="$t('compute.wiz.perHost')">
                        <el-input-number v-model="form.maxPerHost" :min="1" :max="16" controls-position="right" />
                        <span class="input-help">{{ perHostHint }}</span>
                    </el-form-item>
                </el-col>
                <el-col :span="8">
                    <el-form-item :label="$t('compute.host.port')">
                        <el-input-number v-model="form.port" :min="1" :max="65535" controls-position="right" />
                        <span class="input-help">{{ $t('compute.wiz.portHelper') }}</span>
                    </el-form-item>
                </el-col>
            </el-row>

            <!-- 3. engine -->
            <el-divider content-position="left">{{ $t('compute.wiz.stepEngine') }}</el-divider>
            <el-form-item :label="$t('compute.tpl.runtime')">
                <el-select v-model="form.runtimeId" class="w-full">
                    <el-option :value="0" :label="$t('compute.wiz.runtimeAuto', [autoRuntimeName])" />
                    <el-option v-for="r in runtimes" :key="r.id" :label="r.name" :value="r.id">
                        <span>{{ r.name }}</span>
                        <span class="sub ml-2">{{ (r.device || 'gpu').toUpperCase() }} · {{ r.backend }}</span>
                    </el-option>
                </el-select>
                <span class="input-help">{{ $t('compute.wiz.runtimeHelper') }}</span>
            </el-form-item>
            <el-form-item :label="$t('compute.tpl.image')">
                <el-input v-model="form.image" :placeholder="runtimeImage" />
            </el-form-item>
            <el-form-item>
                <el-checkbox v-model="form.needFabric" :disabled="layout === 'single'">
                    {{ $t('compute.tpl.needFabric') }}
                </el-checkbox>
                <span class="input-help">{{ $t('compute.tpl.needFabricHelper') }}</span>
            </el-form-item>
            <el-form-item :label="$t('compute.tpl.extraArgs')">
                <el-input
                    v-model="form.extraArgs"
                    type="textarea"
                    :rows="3"
                    class="code-area"
                    placeholder="--max-model-len 200000 --kv-cache-dtype fp8"
                />
                <span class="input-help">{{ $t('compute.wiz.argsHelper') }}</span>
            </el-form-item>
            <el-form-item :label="$t('compute.tpl.env')">
                <el-input v-model="form.env" type="textarea" :rows="3" class="code-area" placeholder="KEY=VALUE" />
            </el-form-item>
            <el-form-item :label="$t('compute.tpl.startTimeout')">
                <el-input-number
                    v-model="form.startTimeoutSec"
                    :min="60"
                    :max="7200"
                    :step="60"
                    controls-position="right"
                />
            </el-form-item>
            <el-alert type="info" :closable="false" show-icon :title="$t('compute.wiz.flow')" />
        </el-form>
        <template #footer>
            <el-button @click="visible = false">{{ $t('commons.button.cancel') }}</el-button>
            <el-button type="primary" :loading="saving" @click="onSave">{{ $t('commons.button.confirm') }}</el-button>
        </template>
    </DrawerPro>
</template>

<script lang="ts" setup>
import { computed, nextTick, reactive, ref } from 'vue';
import { Compute } from '@/api/interface/compute';
import { listComputeHosts, listComputeModels, saveComputeProfile } from '@/api/modules/compute';
import { MsgError, MsgSuccess } from '@/utils/message';
import { size, t } from '../helper';

interface HostRow {
    id: number;
    name: string;
    addr: string;
    domain: string;
    kind: string;
    accel: string;
    freeMem: string;
    freeBytes: number;
    hasModel: boolean;
    selectable: boolean;
    panel: boolean;
    state: string;
}

const props = defineProps<{ runtimes: Compute.Runtime[] }>();
const emit = defineEmits(['saved']);
const visible = ref(false);
const loading = ref(false);
const saving = ref(false);
const models = ref<Compute.PreparedModel[]>([]);
const scanErrors = ref<string[]>([]);
const hosts = ref<Compute.Host[]>([]);
const modelKey = ref('');
const layout = ref<'single' | 'multi'>('single');
const picked = ref<number[]>([]);
const headId = ref<number>(0);
const hostTable = ref();

const empty = (): Compute.ProfileReq => ({
    id: 0,
    name: '',
    runtimeId: 0,
    servedName: '',
    modelRepo: '',
    modelPath: '',
    hostIds: [],
    nodeCount: 1,
    maxPerHost: 1,
    needFabric: false,
    minMemGb: 0,
    port: 8000,
    image: '',
    extraArgs: '',
    env: '',
    startTimeoutSec: 1800,
    remark: '',
});
const form = reactive<Compute.ProfileReq>(empty());

// hostId 0 = the master panel itself (not a compute host): the model is
// copied from it to the picked hosts before start.
const masterLabel = computed(() => t('compute.wiz.masterHost'));
const selected = computed(() => models.value.find((m) => m.repoId === modelKey.value));
const hostsWithModel = computed(() => selected.value?.hosts || []);
const defaultName = computed(() => {
    const base = form.servedName || 'plan';
    return layout.value === 'multi' ? `${base}-${picked.value.length || 2}x` : base;
});
const stepIndex = computed(() => {
    if (!form.modelPath) return 0;
    if (layout.value === 'multi' && picked.value.length < 2) return 1;
    return 2;
});

const hostKind = (h: Compute.Host) => ((h.realtime?.gpus || []).some((g) => g.kind === 'npu') ? 'npu' : 'gpu');

const hostRows = computed<HostRow[]>(() =>
    hosts.value.map((h) => {
        const rt = h.realtime || ({} as Compute.HostRealtime);
        const gpus = rt.gpus || [];
        const kind = hostKind(h);
        const dedicated = gpus.filter((g) => !g.unified && g.memTotal > 0);
        const free = dedicated.length
            ? dedicated.reduce((s, g) => s + Math.max(0, g.memTotal - g.memUsed), 0)
            : rt.memAvailable || 0;
        const has = hostsWithModel.value.some((m) => m.hostId === h.id);
        const used = h.instanceNames || [];
        let state = t('compute.wiz.stateFree');
        let selectable = true;
        if (!rt.online) {
            state = t('compute.wiz.stateOffline');
            selectable = false;
        } else if (!h.schedulable) {
            state = t('compute.wiz.stateCordoned');
            selectable = false;
        } else if (!gpus.length) {
            state = t('compute.wiz.stateNoAccel');
            selectable = false;
        } else if (used.length) {
            state = t('compute.wiz.stateUsed', [used.join(', ')]);
            // Busy hosts stay pickable for stacked single-host replicas.
            selectable = layout.value === 'single';
        }
        return {
            id: h.id,
            name: h.name,
            addr: h.addr,
            domain: h.domain,
            kind,
            accel: gpus.length ? `${gpus[0].name} ×${gpus.length}` : '—',
            freeMem: size(free),
            freeBytes: free,
            hasModel: has,
            selectable,
            panel: h.source !== 'terminal',
            state,
        };
    }),
);

const sameDomain = computed(() => {
    const rows = hostRows.value.filter((r) => picked.value.includes(r.id));
    return rows.length > 1 && rows.every((r) => r.domain && r.domain === rows[0].domain);
});

const autoRuntimeName = computed(() => {
    const rows = hostRows.value.filter((r) => picked.value.includes(r.id));
    if (rows.some((r) => r.kind === 'npu')) return 'vllm-ascend';
    if (layout.value === 'multi') return 'vllm-ray-multinode';
    if (rows.length && rows.every((r) => r.panel)) return 'vllm-panel';
    return rows.length ? 'vllm-single' : 'vllm-panel / vllm-single';
});
const runtimeImage = computed(() => {
    const name = form.runtimeId ? props.runtimes.find((r) => r.id === form.runtimeId)?.name : autoRuntimeName.value;
    return props.runtimes.find((r) => r.name === name)?.image || '';
});
const perHostHint = computed(() => {
    const rows = hostRows.value.filter((r) => picked.value.includes(r.id));
    const host = rows[0] || hostRows.value.find((r) => r.selectable);
    if (!form.minMemGb || !host) return t('compute.wiz.perHostHelper');
    const fit = Math.floor(host.freeBytes / (form.minMemGb * 1024 ** 3));
    return t('compute.wiz.perHostFit', [host.name, fit]);
});

const onModel = (key: string) => {
    const m = models.value.find((x) => x.repoId === key);
    if (!m) {
        // Free text: a directory or a repo id that is not prepared yet.
        form.modelRepo = key.startsWith('/') ? '' : key;
        if (key.startsWith('/')) form.modelPath = key;
        return;
    }
    form.modelRepo = m.repoId;
    const dirs = (m.hosts || []).map((h) => h.dir);
    // The most common directory becomes the launch path (and sync target).
    form.modelPath = dirs.sort((a, b) => dirs.filter((d) => d === b).length - dirs.filter((d) => d === a).length)[0];
    if (!form.servedName) form.servedName = m.name.toLowerCase();
    if (m.recipe) {
        if (!form.image) form.image = m.recipe.image || '';
        if (!form.extraArgs) form.extraArgs = (m.recipe.args || '').trim();
        if (!form.env) form.env = (m.recipe.env || '').trim();
    }
    if (!picked.value.length) {
        const ids = (m.hosts || []).map((h) => h.hostId);
        nextTick(() => selectRows(layout.value === 'single' ? ids : ids.slice(0, 2)));
    }
};

const onLayout = () => {
    if (layout.value === 'single') form.needFabric = false;
    else {
        form.needFabric = true;
        form.maxPerHost = 1;
    }
    selectRows(picked.value.filter((id) => hostRows.value.find((r) => r.id === id)?.selectable));
};

const onHostSelect = (rows: HostRow[]) => {
    picked.value = rows.map((r) => r.id);
    if (!picked.value.includes(headId.value)) {
        const withModel = rows.find((r) => r.hasModel);
        headId.value = (withModel || rows[0])?.id || 0;
    }
};

const selectRows = (ids: number[]) => {
    hostTable.value?.clearSelection();
    for (const row of hostRows.value) {
        if (ids.includes(row.id) && row.selectable) hostTable.value?.toggleRowSelection(row, true);
    }
};

const open = async (row?: Compute.Profile) => {
    Object.assign(form, empty());
    modelKey.value = '';
    picked.value = [];
    headId.value = 0;
    layout.value = 'single';
    scanErrors.value = [];
    let pinned: number[] = [];
    if (row) {
        Object.assign(form, { ...row, hostIds: [] });
        form.maxPerHost = row.maxPerHost || 1;
        modelKey.value = row.modelRepo || row.modelPath;
        layout.value = row.nodeCount > 1 ? 'multi' : 'single';
        pinned = (row.hostIds || '').split(',').filter(Boolean).map(Number);
        headId.value = pinned[0] || 0;
    }
    visible.value = true;
    loading.value = true;
    try {
        const [m, h] = await Promise.all([listComputeModels(), listComputeHosts()]);
        models.value = m.data?.items || [];
        scanErrors.value = m.data?.errors || [];
        hosts.value = h.data || [];
    } finally {
        loading.value = false;
    }
    await nextTick();
    if (pinned.length) {
        selectRows(pinned);
        headId.value = pinned[0];
    }
};

const onSave = async () => {
    if (!form.modelPath || !form.servedName) {
        MsgError(t('compute.wiz.needModel'));
        return;
    }
    let ids = [...picked.value];
    if (layout.value === 'multi') {
        if (ids.length < 2) {
            MsgError(t('compute.wiz.needHosts'));
            return;
        }
        ids = [headId.value, ...ids.filter((id) => id !== headId.value)];
    }
    if ((form.maxPerHost || 1) > 1 && !form.minMemGb) {
        MsgError(t('compute.wiz.needMem'));
        return;
    }
    saving.value = true;
    try {
        await saveComputeProfile({
            ...form,
            name: form.name || defaultName.value,
            hostIds: ids,
            nodeCount: layout.value === 'multi' ? ids.length : 1,
            maxPerHost: layout.value === 'multi' ? 1 : form.maxPerHost,
        });
        MsgSuccess(t('commons.msg.operationSuccess'));
        visible.value = false;
        emit('saved');
    } finally {
        saving.value = false;
    }
};

defineExpose({ open });
</script>

<style scoped src="../style.css"></style>
<style scoped>
.w-full {
    width: 100%;
}
.opt {
    display: flex;
    justify-content: space-between;
    gap: 12px;
}
</style>

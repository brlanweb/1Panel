<template>
    <div v-loading="loading">
        <el-card shadow="never" class="mb-3">
            <template #header>
                <div class="card-head">
                    <span class="strong">{{ $t('compute.access.router') }}</span>
                    <!-- Status reflects the saved setting, not the unsaved switch. -->
                    <el-tag v-if="dirty" type="warning">{{ $t('compute.access.unsaved') }}</el-tag>
                    <el-tag v-else-if="setting.running" type="success">{{ $t('compute.access.running') }}</el-tag>
                    <el-tag v-else-if="savedEnabled" type="danger">{{ $t('compute.access.failed') }}</el-tag>
                    <el-tag v-else type="info">{{ $t('compute.access.disabled') }}</el-tag>
                </div>
            </template>
            <el-alert
                v-if="setting.error"
                type="error"
                :closable="false"
                show-icon
                class="mb-3"
                :title="setting.error"
            />
            <el-form :model="setting" label-width="140px" class="router-form">
                <el-form-item :label="$t('compute.access.enable')">
                    <el-switch v-model="setting.enabled" />
                    <span class="input-help">{{ $t('compute.access.enableHelper') }}</span>
                </el-form-item>
                <el-form-item :label="$t('compute.access.listen')">
                    <el-input v-model="setting.listen" class="p-w-250" placeholder="0.0.0.0:18080" />
                    <span class="input-help">{{ $t('compute.access.listenHelper') }}</span>
                </el-form-item>
                <el-form-item :label="$t('compute.access.retries')">
                    <el-input-number v-model="setting.maxRetries" :min="0" :max="3" controls-position="right" />
                    <span class="input-help">{{ $t('compute.access.retriesHelper') }}</span>
                </el-form-item>
                <el-form-item :label="$t('compute.access.timeout')">
                    <el-input-number
                        v-model="setting.requestTimeout"
                        :min="10"
                        :max="3600"
                        :step="30"
                        controls-position="right"
                    />
                    <span class="ml-2 sub">s</span>
                </el-form-item>
                <el-form-item :label="$t('compute.access.logRetention')">
                    <el-select v-model="setting.logRetentionHours" class="p-w-250">
                        <el-option v-for="h in [1, 6, 12, 24, 48, 72]" :key="h" :value="h" :label="retentionLabel(h)" />
                    </el-select>
                    <span class="input-help">{{ $t('compute.access.logRetentionHelper') }}</span>
                </el-form-item>
                <el-form-item>
                    <el-button type="primary" :loading="saving" @click="onSaveSetting">
                        {{ $t('commons.button.save') }}
                    </el-button>
                </el-form-item>
            </el-form>
            <div class="usage">
                <div class="strong mb-1">{{ $t('compute.access.usage') }}</div>
                <pre class="mono">{{ usageExample }}</pre>
                <div class="sub">{{ $t('compute.access.usageHelper') }}</div>
            </div>
        </el-card>

        <el-card shadow="never">
            <template #header>
                <div class="card-head">
                    <span>
                        <span class="strong">{{ $t('compute.access.consumers') }}</span>
                        <span class="sub ml-2">{{ $t('compute.access.consumersHelper') }}</span>
                    </span>
                    <el-button type="primary" @click="open()">{{ $t('compute.access.addConsumer') }}</el-button>
                </div>
            </template>
            <el-table :data="consumers" :empty-text="$t('compute.access.noConsumer')">
                <el-table-column :label="$t('commons.table.name')" min-width="150">
                    <template #default="{ row }">
                        <div class="strong">{{ row.name }}</div>
                        <div class="sub mono">{{ row.keyHint }}</div>
                    </template>
                </el-table-column>
                <el-table-column :label="$t('commons.table.status')" width="90">
                    <template #default="{ row }">
                        <el-tag size="small" :type="row.enabled ? 'success' : 'info'">
                            {{ row.enabled ? $t('compute.enabled') : $t('compute.disabledShort') }}
                        </el-tag>
                    </template>
                </el-table-column>
                <el-table-column :label="$t('compute.access.models')" min-width="160">
                    <template #default="{ row }">{{ row.allowedModels || $t('compute.access.allModels') }}</template>
                </el-table-column>
                <el-table-column label="RPM" width="80">
                    <template #default="{ row }">{{ row.rpm || '∞' }}</template>
                </el-table-column>
                <el-table-column :label="$t('compute.access.requests')" width="140">
                    <template #default="{ row }">
                        {{ num(row.requests, 1) }}
                        <span v-if="row.failures" class="stat-danger sub">({{ num(row.failures, 1) }} ✗)</span>
                    </template>
                </el-table-column>
                <el-table-column :label="$t('compute.access.tokens')" width="160">
                    <template #default="{ row }">
                        <div class="sub">in {{ num(row.promptTokens) }} · out {{ num(row.completionTokens) }}</div>
                    </template>
                </el-table-column>
                <el-table-column :label="$t('compute.access.lastUsed')" width="120">
                    <template #default="{ row }">{{ ago(row.lastUsedAt) }}</template>
                </el-table-column>
                <el-table-column :label="$t('commons.table.operate')" width="200" fixed="right">
                    <template #default="{ row }">
                        <el-button link type="primary" @click="open(row)">{{ $t('commons.button.edit') }}</el-button>
                        <el-button link type="primary" @click="onReset(row)">
                            {{ $t('compute.access.resetKey') }}
                        </el-button>
                        <el-button link type="danger" @click="onDelete(row)">
                            {{ $t('commons.button.delete') }}
                        </el-button>
                    </template>
                </el-table-column>
            </el-table>
        </el-card>

        <DialogPro v-model="visible" :title="$t('compute.access.consumer')" size="small" @close="visible = false">
            <el-form :model="form" label-position="top">
                <el-form-item :label="$t('commons.table.name')" required>
                    <el-input v-model="form.name" placeholder="new-api-us" />
                </el-form-item>
                <el-form-item :label="$t('compute.access.models')">
                    <el-select
                        v-model="modelList"
                        multiple
                        filterable
                        allow-create
                        clearable
                        class="w-full"
                        :placeholder="$t('compute.access.allModels')"
                    >
                        <el-option v-for="m in modelNames" :key="m" :label="m" :value="m" />
                    </el-select>
                </el-form-item>
                <el-form-item label="RPM">
                    <el-input-number v-model="form.rpm" :min="0" :step="60" controls-position="right" />
                    <span class="input-help">{{ $t('compute.access.rpmHelper') }}</span>
                </el-form-item>
                <el-form-item :label="$t('commons.table.status')">
                    <el-switch v-model="form.enabled" />
                </el-form-item>
                <el-form-item :label="$t('commons.table.description')">
                    <el-input v-model="form.remark" />
                </el-form-item>
            </el-form>
            <template #footer>
                <el-button @click="visible = false">{{ $t('commons.button.cancel') }}</el-button>
                <el-button type="primary" :disabled="!form.name" @click="onSave">
                    {{ $t('commons.button.confirm') }}
                </el-button>
            </template>
        </DialogPro>

        <DialogPro v-model="keyVisible" :title="$t('compute.access.keyTitle')" size="small" @close="keyVisible = false">
            <el-alert type="warning" :closable="false" show-icon class="mb-3" :title="$t('compute.access.keyOnce')" />
            <el-input :model-value="newKey" readonly>
                <template #append>
                    <el-button @click="copy(newKey)">{{ $t('commons.button.copy') }}</el-button>
                </template>
            </el-input>
            <template #footer>
                <el-button type="primary" @click="keyVisible = false">{{ $t('commons.button.confirm') }}</el-button>
            </template>
        </DialogPro>
    </div>
</template>

<script lang="ts" setup>
import { computed, onMounted, reactive, ref } from 'vue';
import { ElMessageBox } from 'element-plus';
import { Compute } from '@/api/interface/compute';
import {
    deleteComputeConsumer,
    getComputeSetting,
    listComputeConsumers,
    listComputeInstances,
    resetComputeConsumer,
    saveComputeConsumer,
    updateComputeSetting,
} from '@/api/modules/compute';
import { MsgSuccess } from '@/utils/message';
import { ago, num, t } from '../helper';

const loading = ref(false);
const saving = ref(false);
const consumers = ref<Compute.Consumer[]>([]);
const modelNames = ref<string[]>([]);
const setting = reactive<Compute.RouterSetting>({
    enabled: false,
    listen: '0.0.0.0:18080',
    maxRetries: 1,
    requestTimeout: 600,
    logRetentionHours: 24,
    running: false,
    error: '',
    schedulerPaused: false,
});
const retentionLabel = (h: number) => (h < 24 ? t('compute.ana.hours', [h]) : t('compute.ana.days', [h / 24]));
// Last saved router setting: the status tag must not read "start failed"
// while the switch was only flipped locally and not saved yet.
const saved = ref({ enabled: false, listen: '', maxRetries: 1, requestTimeout: 600, logRetentionHours: 24 });
const savedEnabled = computed(() => saved.value.enabled);
const dirty = computed(
    () =>
        setting.enabled !== saved.value.enabled ||
        setting.listen !== saved.value.listen ||
        setting.maxRetries !== saved.value.maxRetries ||
        setting.requestTimeout !== saved.value.requestTimeout ||
        setting.logRetentionHours !== saved.value.logRetentionHours,
);
const applySaved = (data: Compute.RouterSetting) => {
    Object.assign(setting, data);
    saved.value = {
        enabled: data.enabled,
        listen: data.listen,
        maxRetries: data.maxRetries,
        requestTimeout: data.requestTimeout,
        logRetentionHours: data.logRetentionHours,
    };
};
const visible = ref(false);
const keyVisible = ref(false);
const newKey = ref('');
const form = reactive({ id: 0, name: '', allowedModels: '', rpm: 0, enabled: true, remark: '' });
const modelList = computed({
    get: () => (form.allowedModels ? form.allowedModels.split(',').filter(Boolean) : []),
    set: (v: string[]) => (form.allowedModels = v.join(',')),
});

const usageExample = computed(() => {
    const port = (setting.listen || ':18080').split(':').pop();
    const model = modelNames.value[0] || 'deepseek-v4-flash-0731';
    return `curl http://<master-ip>:${port}/v1/chat/completions \\
  -H "Authorization: Bearer sk-xp-..." -H "Content-Type: application/json" \\
  -d '{"model":"${model}","messages":[{"role":"user","content":"hi"}],"stream":true}'`;
});

const load = async () => {
    loading.value = true;
    try {
        const [s, c, i] = await Promise.all([getComputeSetting(), listComputeConsumers(), listComputeInstances()]);
        applySaved(s.data);
        consumers.value = c.data || [];
        modelNames.value = [...new Set((i.data || []).map((x) => x.servedName))].sort();
    } finally {
        loading.value = false;
    }
};

const refresh = async () => {
    const c = await listComputeConsumers();
    consumers.value = c.data || [];
};

const onSaveSetting = async () => {
    saving.value = true;
    try {
        await updateComputeSetting({ ...setting });
        MsgSuccess(t('commons.msg.operationSuccess'));
        const s = await getComputeSetting();
        applySaved(s.data);
    } finally {
        saving.value = false;
    }
};

const open = (row?: Compute.Consumer) => {
    Object.assign(form, { id: 0, name: '', allowedModels: '', rpm: 0, enabled: true, remark: '' }, row || {});
    visible.value = true;
};

const onSave = async () => {
    const res = await saveComputeConsumer({ ...form });
    visible.value = false;
    if (res.data?.key) {
        newKey.value = res.data.key;
        keyVisible.value = true;
    } else {
        MsgSuccess(t('commons.msg.operationSuccess'));
    }
    refresh();
};

const onReset = async (row: Compute.Consumer) => {
    await ElMessageBox.confirm(t('compute.access.resetConfirm', [row.name]), t('compute.access.resetKey'), {
        type: 'warning',
    });
    const res = await resetComputeConsumer(row.id);
    newKey.value = res.data.key;
    keyVisible.value = true;
    refresh();
};

const onDelete = async (row: Compute.Consumer) => {
    await ElMessageBox.confirm(t('compute.access.deleteConfirm', [row.name]), t('commons.button.delete'), {
        type: 'warning',
    });
    await deleteComputeConsumer(row.id);
    refresh();
};

const copy = async (text: string) => {
    try {
        await navigator.clipboard.writeText(text);
        MsgSuccess(t('commons.msg.copySuccess'));
    } catch {
        // insecure context (http): leave the key selectable in the input
    }
};

onMounted(load);
// Usage counters change slowly: refreshed with the page poll.
defineExpose({ refresh });
</script>

<style scoped src="../style.css"></style>
<style scoped>
.w-full {
    width: 100%;
}
.router-form {
    max-width: 760px;
}
.usage pre {
    background: var(--el-fill-color-light);
    padding: 8px 12px;
    border-radius: 4px;
    white-space: pre-wrap;
    word-break: break-all;
    margin: 6px 0;
}
.mb-1 {
    margin-bottom: 4px;
}
</style>

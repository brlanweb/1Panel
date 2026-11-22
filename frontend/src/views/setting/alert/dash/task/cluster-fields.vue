<template>
    <div>
        <!-- Cluster resources -->
        <template v-if="type === 'clusterResource'">
            <el-form-item :label="$t('xpack.alert.clusterNodes')">
                <el-select
                    class="selectClass"
                    v-model="params.nodeIds"
                    multiple
                    clearable
                    collapse-tags
                    collapse-tags-tooltip
                    :placeholder="$t('xpack.alert.clusterAllNodes')"
                >
                    <el-option :value="0" :label="$t('xpack.alert.clusterMasterNode')" />
                    <el-option v-for="node in nodeOptions" :key="node.id" :value="node.id" :label="node.name" />
                </el-select>
                <span class="input-help">{{ $t('xpack.alert.clusterNodesHelper') }}</span>
            </el-form-item>
            <el-form-item :label="$t('xpack.alert.clusterMetrics')">
                <div class="metric-grid">
                    <div class="metric-row">
                        <el-checkbox v-model="metricOn.cpu">CPU</el-checkbox>
                        <el-input-number v-model="params.cpu" :min="1" :max="100" :disabled="!metricOn.cpu" />
                        <span class="input-help">%</span>
                    </div>
                    <div class="metric-row">
                        <el-checkbox v-model="metricOn.memory">{{ $t('xpack.alert.clusterMemory') }}</el-checkbox>
                        <el-input-number v-model="params.memory" :min="1" :max="100" :disabled="!metricOn.memory" />
                        <span class="input-help">%</span>
                    </div>
                    <div class="metric-row">
                        <el-checkbox v-model="metricOn.disk">{{ $t('xpack.alert.clusterDisk') }}</el-checkbox>
                        <el-input-number v-model="params.disk" :min="1" :max="100" :disabled="!metricOn.disk" />
                        <span class="input-help">%</span>
                    </div>
                    <div class="metric-row">
                        <el-checkbox v-model="params.offline">{{ $t('xpack.alert.clusterOffline') }}</el-checkbox>
                    </div>
                </div>
                <span class="input-help">{{ $t('xpack.alert.clusterMetricsHelper') }}</span>
            </el-form-item>
            <el-form-item :label="$t('xpack.alert.clusterAvgWindow')">
                <el-input-number v-model="row.cycle" :min="1" :max="30" />
                <span class="input-help ml-2">{{ $t('commons.units.minute') }}</span>
            </el-form-item>
            <el-form-item v-if="metricOn.disk" :label="$t('xpack.alert.clusterDiskExclude')">
                <el-input v-model="diskExcludeText" type="textarea" :rows="2" placeholder="/snap/*&#10;/boot" />
                <span class="input-help">{{ $t('xpack.alert.clusterDiskExcludeHelper') }}</span>
            </el-form-item>
        </template>

        <!-- Website check -->
        <template v-if="type === 'siteProbe'">
            <el-form-item :label="$t('xpack.alert.probeName')" prop="title">
                <el-input v-model.trim="row.title" maxlength="64" />
            </el-form-item>
            <el-form-item label="URL" required>
                <el-input v-model.trim="params.url" placeholder="https://example.com/health">
                    <template #prepend>
                        <el-select v-model="params.method" style="width: 100px">
                            <el-option value="GET" label="GET" />
                            <el-option value="HEAD" label="HEAD" />
                            <el-option value="POST" label="POST" />
                        </el-select>
                    </template>
                </el-input>
            </el-form-item>
            <el-form-item :label="$t('xpack.alert.probeExpectStatus')">
                <el-input v-model.trim="params.expectStatus" placeholder="200-399" />
                <span class="input-help">{{ $t('xpack.alert.probeExpectStatusHelper') }}</span>
            </el-form-item>
            <el-form-item :label="$t('xpack.alert.probeKeyword')">
                <el-input v-model="params.keyword" />
                <span class="input-help">{{ $t('xpack.alert.probeKeywordHelper') }}</span>
            </el-form-item>
            <el-form-item :label="$t('xpack.alert.probeHeaders')">
                <el-input v-model="headersText" type="textarea" :rows="2" placeholder="Authorization: Bearer xxx" />
            </el-form-item>
            <el-form-item>
                <el-checkbox v-model="params.followRedirect">{{ $t('xpack.alert.probeFollowRedirect') }}</el-checkbox>
                <el-checkbox v-model="params.verifyTLS">{{ $t('xpack.alert.probeVerifyTLS') }}</el-checkbox>
            </el-form-item>
        </template>

        <!-- Model API check -->
        <template v-if="type === 'modelApi'">
            <el-form-item :label="$t('xpack.alert.probeName')" prop="title">
                <el-input v-model.trim="row.title" maxlength="64" />
            </el-form-item>
            <el-form-item :label="$t('xpack.alert.modelProvider')" required>
                <el-radio-group v-model="params.provider">
                    <el-radio-button value="openai" label="OpenAI" />
                    <el-radio-button value="anthropic" label="Anthropic" />
                    <el-radio-button value="gemini" label="Gemini" />
                </el-radio-group>
                <span class="input-help">{{ $t('xpack.alert.modelProviderHelper') }}</span>
            </el-form-item>
            <el-form-item label="Base URL">
                <el-input v-model.trim="params.baseUrl" :placeholder="baseUrlPlaceholder" />
                <span class="input-help">{{ $t('xpack.alert.modelBaseUrlHelper') }}</span>
            </el-form-item>
            <el-form-item :label="$t('xpack.alert.modelName')" required>
                <el-input
                    v-model.trim="params.model"
                    placeholder="gpt-4o-mini / claude-3-5-haiku-latest / gemini-2.0-flash"
                />
            </el-form-item>
            <el-form-item label="API Key" :required="!params.apiKeySet">
                <el-input
                    v-model.trim="params.apiKey"
                    type="password"
                    show-password
                    autocomplete="new-password"
                    :placeholder="params.apiKeySet ? $t('xpack.alert.modelKeyKeep') : ''"
                />
            </el-form-item>
            <el-form-item>
                <el-checkbox v-model="params.strictContent">{{ $t('xpack.alert.modelStrict') }}</el-checkbox>
                <el-checkbox v-model="params.treat429AsFail">{{ $t('xpack.alert.model429') }}</el-checkbox>
            </el-form-item>
        </template>

        <!-- Common for site / model -->
        <template v-if="isProbe">
            <el-form-item :label="$t('xpack.alert.probeInterval')">
                <el-select class="selectClass" v-model="row.cycle">
                    <el-option
                        v-for="m in intervalChoices"
                        :key="m"
                        :value="m"
                        :label="m + ' ' + $t('commons.units.minute')"
                    />
                </el-select>
            </el-form-item>
            <el-form-item :label="$t('xpack.alert.probeTimeout')">
                <el-input-number v-model="params.timeoutSec" :min="1" :max="maxTimeout" />
                <span class="input-help ml-2">{{ $t('commons.units.second') }}</span>
                <span class="input-help">{{ $t('xpack.alert.probeTimeoutHelper') }}</span>
            </el-form-item>
            <el-form-item>
                <el-checkbox v-model="params.useProxy">{{ $t('xpack.alert.probeUseProxy') }}</el-checkbox>
                <span class="input-help">{{ $t('xpack.alert.probeUseProxyHelper') }}</span>
            </el-form-item>
        </template>

        <el-form-item :label="$t('xpack.alert.failThreshold')">
            <el-input-number v-model="row.count" :min="1" :max="100" />
            <span class="input-help">{{ $t('xpack.alert.failThresholdHelper') }}</span>
        </el-form-item>
        <el-form-item :label="$t('xpack.alert.cooldownHours')">
            <el-input-number v-model="params.cooldownHours" :min="1" :max="720" />
            <span class="input-help">{{ $t('xpack.alert.cooldownHoursHelper') }}</span>
        </el-form-item>

        <el-form-item v-if="states.length > 0" :label="$t('xpack.alert.targetStates')">
            <el-table :data="states" size="small" max-height="240">
                <el-table-column
                    :label="$t('xpack.alert.target')"
                    prop="targetLabel"
                    min-width="160"
                    show-overflow-tooltip
                />
                <el-table-column :label="$t('commons.table.status')" width="90">
                    <template #default="{ row: s }">
                        <el-tag :type="s.status === 'ok' ? 'success' : 'danger'" size="small">
                            {{ s.status === 'ok' ? $t('xpack.alert.stateOk') : $t('xpack.alert.stateFail') }}
                        </el-tag>
                    </template>
                </el-table-column>
                <el-table-column :label="$t('xpack.alert.stateDetail')" min-width="200" show-overflow-tooltip>
                    <template #default="{ row: s }">
                        {{ s.lastError || s.lastValue || '-' }}
                        <span v-if="s.cooldownUntil > nowSec" class="input-help">
                            · {{ $t('xpack.alert.stateCooldown', [formatTime(s.cooldownUntil)]) }}
                        </span>
                    </template>
                </el-table-column>
            </el-table>
        </el-form-item>
    </div>
</template>

<script lang="ts" setup>
import { computed, reactive, ref, watch } from 'vue';
import { Alert } from '@/api/interface/alert';
import { listClusterNodeOptions } from '@/api/modules/cluster';
import { ListClusterAlertStates } from '@/api/modules/alert';
import i18n from '@/lang';

const props = defineProps<{ row: Alert.AlertInfo; isEdit: boolean }>();
const row = props.row;
const type = computed(() => row.type);
const isProbe = computed(() => type.value === 'siteProbe' || type.value === 'modelApi');

const params = reactive<Alert.ClusterAlertParams>({});
const metricOn = reactive({ cpu: false, memory: false, disk: false });
const diskExcludeText = ref('');
const headersText = ref('');
const nodeOptions = ref<{ id: number; name: string }[]>([]);
const states = ref<Alert.ClusterTargetState[]>([]);
const nowSec = Math.floor(Date.now() / 1000);
const intervalChoices = [1, 2, 3, 5, 10, 15, 30, 60];

const maxTimeout = computed(() => Math.min(300, Math.max(1, (row.cycle || 1) * 60 - 1)));
const baseUrlPlaceholder = computed(() => {
    switch (params.provider) {
        case 'anthropic':
            return 'https://api.anthropic.com';
        case 'gemini':
            return 'https://generativelanguage.googleapis.com/v1beta';
        default:
            return 'https://api.openai.com/v1';
    }
});

const defaults = (t: string): Alert.ClusterAlertParams => {
    switch (t) {
        case 'clusterResource':
            return { nodeIds: [], cpu: 90, memory: 90, disk: 90, offline: true, diskExclude: [], cooldownHours: 24 };
        case 'siteProbe':
            return {
                method: 'GET',
                expectStatus: '200-399',
                followRedirect: true,
                verifyTLS: true,
                timeoutSec: 10,
                useProxy: false,
                cooldownHours: 24,
            };
        case 'modelApi':
            return {
                provider: 'openai',
                maxTokens: 16,
                strictContent: false,
                treat429AsFail: false,
                timeoutSec: 30,
                useProxy: false,
                cooldownHours: 24,
            };
    }
    return {};
};

const load = () => {
    Object.keys(params).forEach((k) => delete (params as any)[k]);
    let stored: Alert.ClusterAlertParams = {};
    if (props.isEdit && row.advancedParams) {
        try {
            stored = JSON.parse(row.advancedParams);
        } catch {}
    }
    Object.assign(params, defaults(row.type), stored);
    params.apiKey = '';
    metricOn.cpu = !props.isEdit || !!stored.cpu;
    metricOn.memory = !props.isEdit || !!stored.memory;
    metricOn.disk = !props.isEdit || !!stored.disk;
    if (!params.cpu) params.cpu = 90;
    if (!params.memory) params.memory = 90;
    if (!params.disk) params.disk = 90;
    diskExcludeText.value = (params.diskExclude || []).join('\n');
    headersText.value = Object.entries(params.headers || {})
        .map(([k, v]) => `${k}: ${v}`)
        .join('\n');
    if (!props.isEdit) {
        if (row.type === 'clusterResource') {
            row.cycle = 5;
            row.count = 2;
        } else {
            row.cycle = 5;
            row.count = 2;
        }
        row.sendCount = 3;
        row.project = '';
    }
    if (row.type === 'clusterResource') {
        listClusterNodeOptions()
            .then((res) => (nodeOptions.value = (res.data || []).map((n) => ({ id: n.id, name: n.name }))))
            .catch(() => {});
    }
    states.value = [];
    if (props.isEdit && row.id) {
        ListClusterAlertStates(row.id)
            .then((res) => (states.value = res.data || []))
            .catch(() => {});
    }
};

watch(() => row.type, load, { immediate: true });

const formatTime = (sec: number) => new Date(sec * 1000).toLocaleString();

// validate returns an error message or '' and writes advancedParams.
const build = (): string => {
    const t = i18n.global.t;
    const out: Alert.ClusterAlertParams = { cooldownHours: params.cooldownHours || 24 };
    if (row.type === 'clusterResource') {
        out.nodeIds = params.nodeIds || [];
        out.cpu = metricOn.cpu ? params.cpu : 0;
        out.memory = metricOn.memory ? params.memory : 0;
        out.disk = metricOn.disk ? params.disk : 0;
        out.offline = !!params.offline;
        out.diskExclude = diskExcludeText.value
            .split('\n')
            .map((s) => s.trim())
            .filter(Boolean);
        if (!out.cpu && !out.memory && !out.disk && !out.offline) return t('xpack.alert.clusterMetricRequired');
        if (!row.title) row.title = t('xpack.alert.clusterResource');
    } else {
        if (!row.title) return t('xpack.alert.probeNameRequired');
        if ((params.timeoutSec || 0) >= (row.cycle || 1) * 60) return t('xpack.alert.probeTimeoutHelper');
        out.timeoutSec = params.timeoutSec;
        out.useProxy = !!params.useProxy;
    }
    if (row.type === 'siteProbe') {
        if (!/^https?:\/\/.+/i.test(params.url || '')) return t('xpack.alert.probeUrlInvalid');
        const headers: Record<string, string> = {};
        for (const line of headersText.value.split('\n')) {
            const idx = line.indexOf(':');
            if (idx > 0) headers[line.slice(0, idx).trim()] = line.slice(idx + 1).trim();
        }
        Object.assign(out, {
            url: params.url,
            method: params.method,
            expectStatus: params.expectStatus,
            keyword: params.keyword,
            followRedirect: !!params.followRedirect,
            verifyTLS: !!params.verifyTLS,
            headers,
        });
    }
    if (row.type === 'modelApi') {
        if (!params.model) return t('xpack.alert.modelNameRequired');
        if (!params.apiKey && !params.apiKeySet) return t('xpack.alert.modelKeyRequired');
        if (params.baseUrl && !/^https?:\/\/.+/i.test(params.baseUrl)) return t('xpack.alert.probeUrlInvalid');
        Object.assign(out, {
            provider: params.provider,
            baseUrl: params.baseUrl,
            model: params.model,
            apiKey: params.apiKey || '',
            maxTokens: params.maxTokens || 16,
            strictContent: !!params.strictContent,
            treat429AsFail: !!params.treat429AsFail,
        });
    }
    row.advancedParams = JSON.stringify(out);
    return '';
};

defineExpose({ build });
</script>

<style scoped lang="scss">
.metric-grid {
    display: flex;
    flex-direction: column;
    gap: 6px;
    width: 100%;
}
.metric-row {
    display: flex;
    align-items: center;
    gap: 10px;
    .el-checkbox {
        min-width: 110px;
    }
}
</style>

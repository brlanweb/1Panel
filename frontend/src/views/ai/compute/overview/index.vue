<template>
    <div v-loading="loading">
        <el-alert
            v-if="data && data.hosts === 0"
            type="info"
            :closable="false"
            show-icon
            class="mb-3"
            :title="$t('compute.overview.emptyTitle')"
        >
            <template #default>
                <div>{{ $t('compute.overview.emptyHelper') }}</div>
                <el-button class="mt-2" type="primary" size="small" @click="emit('goto', 'hosts')">
                    {{ $t('compute.host.import') }}
                </el-button>
            </template>
        </el-alert>

        <div class="stat-cards mb-3" v-if="data">
            <el-card shadow="hover" class="stat-card" @click="emit('goto', 'hosts')">
                <div class="stat-title">{{ $t('compute.overview.hosts') }}</div>
                <div class="stat-value">
                    <span class="stat-success">{{ data.hostsOnline }}</span>
                    <span class="stat-total">/ {{ data.hosts }}</span>
                </div>
                <div class="stat-note">
                    {{ $t('compute.overview.idle') }} {{ data.hostsIdle }}
                    <span v-if="data.hostsCordoned">
                        · {{ $t('compute.overview.cordoned') }} {{ data.hostsCordoned }}
                    </span>
                </div>
            </el-card>
            <el-card shadow="hover" class="stat-card">
                <div class="stat-title">{{ $t('compute.overview.accel') }}</div>
                <div class="stat-value">
                    {{ data.gpuUtilAvg.toFixed(0) }}
                    <span class="stat-unit">%</span>
                </div>
                <div class="stat-note">
                    <template v-if="data.gpus">{{ data.gpus }} GPU ·</template>
                    <template v-if="data.npus">{{ data.npus }} NPU ·</template>
                    {{ $t('compute.overview.mem') }} {{ size(data.memUsed) }} /
                    {{ size(data.memTotal) }}
                </div>
            </el-card>
            <el-card shadow="hover" class="stat-card" @click="emit('goto', 'instances')">
                <div class="stat-title">{{ $t('compute.overview.instances') }}</div>
                <div class="stat-value">
                    <span :class="data.instHealthy < data.instances ? 'stat-warning' : 'stat-success'">
                        {{ data.instHealthy }}
                    </span>
                    <span class="stat-total">/ {{ data.instances }}</span>
                </div>
                <div class="stat-note">{{ $t('compute.overview.genTps') }} {{ num(data.genTps) }} tok/s</div>
            </el-card>
            <el-card shadow="hover" class="stat-card" @click="emit('goto', 'hosts')">
                <div class="stat-title">{{ $t('compute.overview.fabric') }}</div>
                <div class="stat-value" :class="data.fabricIssues ? 'stat-danger' : ''">{{ data.domains }}</div>
                <div class="stat-note">
                    <span v-if="data.fabricIssues" class="stat-danger">
                        {{ $t('compute.overview.fabricIssues', [data.fabricIssues]) }}
                    </span>
                    <span v-else>{{ $t('compute.overview.fabricOk') }}</span>
                </div>
            </el-card>
            <el-card shadow="hover" class="stat-card" @click="emit('goto', 'access')">
                <div class="stat-title">{{ $t('compute.overview.router') }}</div>
                <div class="stat-value">
                    <el-tag v-if="data.router.running" type="success">{{ $t('compute.access.running') }}</el-tag>
                    <el-tag v-else-if="data.router.enabled" type="danger">{{ $t('compute.access.failed') }}</el-tag>
                    <el-tag v-else type="info">{{ $t('compute.access.disabled') }}</el-tag>
                </div>
                <div class="stat-note">
                    {{ data.router.listen }}
                    <span v-if="data.router.schedulerPaused">· {{ $t('compute.overview.autoPaused') }}</span>
                </div>
            </el-card>
        </div>

        <el-row :gutter="12">
            <el-col :span="suggestions.length || warnings.length ? 16 : 24">
                <el-card shadow="never">
                    <template #header>
                        <div class="card-head">
                            <span>{{ $t('compute.overview.models') }}</span>
                            <el-button link type="primary" @click="emit('goto', 'policy')">
                                {{ $t('compute.overview.editPolicy') }}
                            </el-button>
                        </div>
                    </template>
                    <el-table :data="data?.models || []" :empty-text="$t('compute.overview.noModel')">
                        <el-table-column :label="$t('compute.servedName')" min-width="200" prop="servedName" />
                        <el-table-column :label="$t('compute.overview.healthy')" width="110">
                            <template #default="{ row }">
                                <span :class="row.healthy < Math.max(row.minInstances, 1) ? 'stat-danger' : ''">
                                    {{ row.healthy }} / {{ row.instances }}
                                </span>
                                <div class="sub" v-if="row.policyMode !== 'off'">
                                    {{ row.minInstances }}–{{ row.maxInstances }}
                                </div>
                            </template>
                        </el-table-column>
                        <el-table-column :label="$t('compute.metric.running')" width="90">
                            <template #default="{ row }">{{ num(row.running, 0) }}</template>
                        </el-table-column>
                        <el-table-column :label="$t('compute.metric.waiting')" width="90">
                            <template #default="{ row }">
                                <span :class="row.waiting > 0 ? 'stat-warning' : ''">{{ num(row.waiting, 0) }}</span>
                            </template>
                        </el-table-column>
                        <el-table-column :label="$t('compute.metric.kv')" min-width="140">
                            <template #default="{ row }">
                                <el-progress
                                    :percentage="pct(row.kvUsage * 100)"
                                    :color="barColor(row.kvUsage * 100)"
                                    :stroke-width="8"
                                />
                            </template>
                        </el-table-column>
                        <el-table-column :label="$t('compute.metric.ttft')" width="100">
                            <template #default="{ row }">{{ ms(row.ttftMs) }}</template>
                        </el-table-column>
                        <el-table-column :label="$t('compute.metric.genTps')" width="110">
                            <template #default="{ row }">{{ num(row.genTps) }}</template>
                        </el-table-column>
                        <el-table-column :label="$t('compute.policy.mode')" width="100">
                            <template #default="{ row }">
                                <el-tag size="small" :type="row.policyMode === 'auto' ? 'success' : 'info'">
                                    {{ $t('compute.policy.mode_' + row.policyMode) }}
                                </el-tag>
                            </template>
                        </el-table-column>
                    </el-table>
                </el-card>
            </el-col>
            <el-col :span="8" v-if="suggestions.length || warnings.length">
                <el-card shadow="never" class="side-card" v-if="suggestions.length">
                    <template #header>
                        <div class="card-head">
                            <span>{{ $t('compute.overview.suggestions') }}</span>
                            <el-tag size="small" type="warning">{{ suggestions.length }}</el-tag>
                        </div>
                    </template>
                    <div v-for="s in suggestions" :key="s.id" class="sug-item">
                        <div class="sug-head">
                            <el-tag size="small" :type="sevType(s.severity)">
                                {{ $t('compute.suggest.' + s.kind) }}
                            </el-tag>
                            <span class="sug-model">{{ s.servedName }}</span>
                        </div>
                        <div class="sub">{{ s.reason }}</div>
                        <div class="sug-actions">
                            <el-button
                                v-if="s.actionable"
                                size="small"
                                type="primary"
                                link
                                :loading="acting === s.id"
                                @click="onApply(s)"
                            >
                                {{ $t('compute.suggest.apply') }}
                            </el-button>
                            <el-button size="small" link @click="onDismiss(s)">
                                {{ $t('compute.suggest.dismiss') }}
                            </el-button>
                        </div>
                    </div>
                </el-card>
                <el-card shadow="never" class="side-card mt-3" v-if="warnings.length">
                    <template #header>{{ $t('compute.overview.warnings') }}</template>
                    <div v-for="w in warnings" :key="w" class="warn-item">
                        <el-icon class="stat-warning"><WarningFilled /></el-icon>
                        {{ warningText(w) }}
                    </div>
                </el-card>
            </el-col>
        </el-row>
    </div>
</template>

<script lang="ts" setup>
import { computed, ref } from 'vue';
import { WarningFilled } from '@element-plus/icons-vue';
import { ElMessageBox } from 'element-plus';
import { Compute } from '@/api/interface/compute';
import { getComputeOverview, listComputeSuggestions, operateComputeSuggestion } from '@/api/modules/compute';
import { MsgSuccess } from '@/utils/message';
import { barColor, ms, num, pct, size, t, warningText } from '../helper';

const emit = defineEmits(['goto']);
const loading = ref(false);
const data = ref<Compute.Overview>();
const suggestions = ref<Compute.Suggestion[]>([]);
const acting = ref('');
const warnings = computed(() => (data.value?.warnings || []).slice(0, 30));

const sevType = (s: string) => (s === 'critical' ? 'danger' : s === 'warning' ? 'warning' : 'info');

const refresh = async (force = false) => {
    if (force && !data.value) loading.value = true;
    try {
        const [o, s] = await Promise.all([getComputeOverview(), listComputeSuggestions()]);
        data.value = o.data;
        suggestions.value = s.data || [];
    } finally {
        loading.value = false;
    }
};

const onApply = async (s: Compute.Suggestion) => {
    await ElMessageBox.confirm(
        `${t('compute.suggest.' + s.kind)} · ${s.servedName}\n${s.reason}`,
        t('compute.suggest.applyConfirm'),
        { type: 'warning' },
    );
    acting.value = s.id;
    try {
        const res = await operateComputeSuggestion(s.id, 'apply');
        MsgSuccess(t('compute.job.started', [res.data?.jobId || '']));
        await refresh();
    } finally {
        acting.value = '';
    }
};

const onDismiss = async (s: Compute.Suggestion) => {
    await operateComputeSuggestion(s.id, 'dismiss');
    suggestions.value = suggestions.value.filter((x) => x.id !== s.id);
};

defineExpose({ refresh });
</script>

<style scoped src="../style.css"></style>

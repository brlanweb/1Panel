<template>
    <div v-loading="loading">
        <el-alert type="info" :closable="false" show-icon class="mb-3" :title="$t('compute.policy.helper')" />
        <div class="toolbar">
            <el-button type="primary" @click="open()">{{ $t('compute.policy.add') }}</el-button>
            <span class="spacer" />
            <span class="sub">{{ $t('compute.policy.autoSwitch') }}</span>
            <el-switch
                v-model="autoEnabled"
                :loading="savingSwitch"
                :active-text="$t('compute.policy.autoOn')"
                :inactive-text="$t('compute.policy.autoOff')"
                @change="onAutoSwitch"
            />
        </div>
        <el-table :data="policies" :empty-text="$t('compute.policy.empty')">
            <el-table-column :label="$t('compute.servedName')" prop="servedName" min-width="180" />
            <el-table-column :label="$t('compute.policy.mode')" width="110">
                <template #default="{ row }">
                    <el-tag
                        size="small"
                        :type="row.mode === 'auto' ? 'success' : row.mode === 'suggest' ? 'primary' : 'info'"
                    >
                        {{ $t('compute.policy.mode_' + row.mode) }}
                    </el-tag>
                </template>
            </el-table-column>
            <el-table-column :label="$t('compute.policy.range')" width="120">
                <template #default="{ row }">{{ row.minInstances }} – {{ row.maxInstances }}</template>
            </el-table-column>
            <el-table-column :label="$t('compute.policy.hotSpare')" prop="hotSpare" width="90" />
            <el-table-column :label="$t('compute.policy.priority')" width="110">
                <template #default="{ row }">
                    {{ row.priority }}
                    <el-tag v-if="row.preemptible" size="small" type="warning" effect="plain">
                        {{ $t('compute.policy.preemptible') }}
                    </el-tag>
                </template>
            </el-table-column>
            <el-table-column :label="$t('compute.policy.trigger')" min-width="220">
                <template #default="{ row }">
                    <div class="sub">
                        {{
                            $t('compute.policy.upWhen', [
                                row.queueHigh,
                                Math.round(row.kvHigh * 100),
                                row.sustainRounds,
                            ])
                        }}
                    </div>
                    <div class="sub" v-if="row.idleMinutes">{{ $t('compute.policy.downWhen', [row.idleMinutes]) }}</div>
                </template>
            </el-table-column>
            <el-table-column :label="$t('compute.tpl.profile')" min-width="140">
                <template #default="{ row }">{{ profileName(row.profileId) }}</template>
            </el-table-column>
            <el-table-column :label="$t('commons.table.operate')" width="150" fixed="right">
                <template #default="{ row }">
                    <el-button link type="primary" @click="open(row)">{{ $t('commons.button.edit') }}</el-button>
                    <el-button link type="danger" @click="onDelete(row)">{{ $t('commons.button.delete') }}</el-button>
                </template>
            </el-table-column>
        </el-table>

        <DrawerPro
            v-model="visible"
            :header="$t('compute.policy.title')"
            :resource="form.servedName"
            size="normal"
            @close="visible = false"
        >
            <el-form label-position="top" :model="form">
                <el-form-item :label="$t('compute.servedName')" required>
                    <el-select v-model="form.servedName" filterable allow-create class="w-full" :disabled="!!form.id">
                        <el-option v-for="m in modelNames" :key="m" :label="m" :value="m" />
                    </el-select>
                </el-form-item>
                <el-form-item :label="$t('compute.policy.mode')">
                    <el-radio-group v-model="form.mode">
                        <el-radio-button value="off">{{ $t('compute.policy.mode_off') }}</el-radio-button>
                        <el-radio-button value="suggest">{{ $t('compute.policy.mode_suggest') }}</el-radio-button>
                        <el-radio-button value="auto">{{ $t('compute.policy.mode_auto') }}</el-radio-button>
                    </el-radio-group>
                    <span class="input-help">{{ $t('compute.policy.modeHelper_' + form.mode) }}</span>
                </el-form-item>
                <el-form-item :label="$t('compute.policy.scaleProfile')" :required="form.mode === 'auto'">
                    <el-select v-model="form.profileId" clearable class="w-full">
                        <el-option
                            v-for="p in profiles.filter((x) => !form.servedName || x.servedName === form.servedName)"
                            :key="p.id"
                            :label="`${p.name} (${p.nodeCount}×)`"
                            :value="p.id"
                        />
                    </el-select>
                    <span class="input-help">{{ $t('compute.policy.scaleProfileHelper') }}</span>
                </el-form-item>
                <el-row :gutter="12">
                    <el-col :span="8">
                        <el-form-item :label="$t('compute.policy.min')">
                            <el-input-number v-model="form.minInstances" :min="0" :max="64" controls-position="right" />
                        </el-form-item>
                    </el-col>
                    <el-col :span="8">
                        <el-form-item :label="$t('compute.policy.max')">
                            <el-input-number v-model="form.maxInstances" :min="0" :max="64" controls-position="right" />
                        </el-form-item>
                    </el-col>
                    <el-col :span="8">
                        <el-form-item :label="$t('compute.policy.hotSpare')">
                            <el-input-number v-model="form.hotSpare" :min="0" :max="16" controls-position="right" />
                        </el-form-item>
                    </el-col>
                </el-row>
                <el-row :gutter="12">
                    <el-col :span="12">
                        <el-form-item :label="$t('compute.policy.priority')">
                            <el-slider v-model="form.priority" :min="0" :max="100" />
                        </el-form-item>
                    </el-col>
                    <el-col :span="12">
                        <el-form-item :label="$t('compute.policy.preemptible')">
                            <el-switch v-model="form.preemptible" />
                            <span class="input-help">{{ $t('compute.policy.preemptHelper') }}</span>
                        </el-form-item>
                    </el-col>
                </el-row>
                <el-divider content-position="left">{{ $t('compute.policy.thresholds') }}</el-divider>
                <el-row :gutter="12">
                    <el-col :span="8">
                        <el-form-item :label="$t('compute.policy.queueHigh')">
                            <el-input-number
                                v-model="form.queueHigh"
                                :min="0.5"
                                :step="0.5"
                                controls-position="right"
                            />
                        </el-form-item>
                    </el-col>
                    <el-col :span="8">
                        <el-form-item :label="$t('compute.policy.kvHigh')">
                            <el-input-number v-model="kvPercent" :min="50" :max="100" controls-position="right" />
                        </el-form-item>
                    </el-col>
                    <el-col :span="8">
                        <el-form-item :label="$t('compute.policy.sustain')">
                            <el-input-number
                                v-model="form.sustainRounds"
                                :min="1"
                                :max="20"
                                controls-position="right"
                            />
                        </el-form-item>
                    </el-col>
                </el-row>
                <el-row :gutter="12">
                    <el-col :span="12">
                        <el-form-item :label="$t('compute.policy.idle')">
                            <el-input-number
                                v-model="form.idleMinutes"
                                :min="0"
                                :max="1440"
                                controls-position="right"
                            />
                            <span class="input-help">{{ $t('compute.policy.idleHelper') }}</span>
                        </el-form-item>
                    </el-col>
                    <el-col :span="12">
                        <el-form-item :label="$t('compute.policy.cooldown')">
                            <el-input-number
                                v-model="form.cooldownSec"
                                :min="60"
                                :max="86400"
                                :step="60"
                                controls-position="right"
                            />
                        </el-form-item>
                    </el-col>
                </el-row>
            </el-form>
            <template #footer>
                <el-button @click="visible = false">{{ $t('commons.button.cancel') }}</el-button>
                <el-button type="primary" :disabled="!form.servedName" @click="onSave">
                    {{ $t('commons.button.confirm') }}
                </el-button>
            </template>
        </DrawerPro>
    </div>
</template>

<script lang="ts" setup>
import { computed, onMounted, reactive, ref } from 'vue';
import { ElMessageBox } from 'element-plus';
import { Compute } from '@/api/interface/compute';
import {
    deleteComputePolicy,
    getComputeSetting,
    listComputeInstances,
    listComputePolicies,
    listComputeProfiles,
    saveComputePolicy,
    updateComputeSetting,
} from '@/api/modules/compute';
import { MsgSuccess } from '@/utils/message';
import { t } from '../helper';

const loading = ref(false);
const visible = ref(false);
const policies = ref<Compute.Policy[]>([]);
const profiles = ref<Compute.Profile[]>([]);
const served = ref<string[]>([]);
const setting = ref<Compute.RouterSetting>();
const autoEnabled = ref(true);
const savingSwitch = ref(false);

const empty = (): Partial<Compute.Policy> => ({
    id: 0,
    servedName: '',
    profileId: 0,
    mode: 'suggest',
    minInstances: 1,
    maxInstances: 2,
    hotSpare: 0,
    priority: 50,
    preemptible: false,
    queueHigh: 4,
    kvHigh: 0.92,
    sustainRounds: 3,
    idleMinutes: 30,
    cooldownSec: 600,
});
const form = reactive<Partial<Compute.Policy>>(empty());
const kvPercent = computed({
    get: () => Math.round((form.kvHigh || 0) * 100),
    set: (v: number) => (form.kvHigh = v / 100),
});
const modelNames = computed(() => [...new Set([...served.value, ...profiles.value.map((p) => p.servedName)])].sort());
const profileName = (id: number) => profiles.value.find((p) => p.id === id)?.name || '—';

const load = async () => {
    loading.value = true;
    try {
        const [p, pr, i, s] = await Promise.all([
            listComputePolicies(),
            listComputeProfiles(),
            listComputeInstances(),
            getComputeSetting(),
        ]);
        policies.value = p.data || [];
        profiles.value = pr.data || [];
        served.value = (i.data || []).map((x) => x.servedName);
        setting.value = s.data;
        autoEnabled.value = !s.data.schedulerPaused;
    } finally {
        loading.value = false;
    }
};

const open = (row?: Compute.Policy) => {
    Object.assign(form, empty(), row || {});
    visible.value = true;
};

const onSave = async () => {
    if (form.mode === 'auto') {
        await ElMessageBox.confirm(t('compute.policy.autoConfirm'), t('compute.policy.mode_auto'), { type: 'warning' });
    }
    await saveComputePolicy({ ...form, profileId: form.profileId || 0 });
    MsgSuccess(t('commons.msg.operationSuccess'));
    visible.value = false;
    load();
};

const onDelete = async (row: Compute.Policy) => {
    await ElMessageBox.confirm(t('compute.tpl.deleteConfirm', [row.servedName]), t('commons.button.delete'), {
        type: 'warning',
    });
    await deleteComputePolicy(row.id);
    load();
};

const onAutoSwitch = async (v: boolean) => {
    if (!setting.value) return;
    savingSwitch.value = true;
    try {
        await updateComputeSetting({ ...setting.value, schedulerPaused: !v });
        setting.value.schedulerPaused = !v;
        MsgSuccess(t('commons.msg.operationSuccess'));
    } catch {
        autoEnabled.value = !v;
    } finally {
        savingSwitch.value = false;
    }
};

onMounted(load);
</script>

<style scoped src="../style.css"></style>
<style scoped>
.w-full {
    width: 100%;
}
</style>

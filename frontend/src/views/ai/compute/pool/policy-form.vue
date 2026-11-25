<template>
    <div>
        <el-form-item :label="$t('compute.pool.plan')" :required="form.mode === 'auto'">
            <el-select
                v-model="form.profileId"
                clearable
                class="w-full"
                :placeholder="$t('compute.pool.planPlaceholder')"
            >
                <el-option
                    v-for="p in planOptions"
                    :key="p.id"
                    :label="`${p.name} · ${$t('compute.pool.hostsPerReplica', [p.nodeCount])}`"
                    :value="p.id"
                />
            </el-select>
            <span class="input-help">
                {{ $t('compute.pool.planHelper') }}
                <template v-if="plan && plan.nodeCount > 1">
                    {{ $t('compute.pool.groupHelper', [plan.nodeCount]) }}
                </template>
            </span>
        </el-form-item>
        <el-form-item :label="$t('compute.policy.mode')">
            <el-radio-group v-model="form.mode">
                <el-radio-button value="off">{{ $t('compute.policy.mode_off') }}</el-radio-button>
                <el-radio-button value="suggest">{{ $t('compute.policy.mode_suggest') }}</el-radio-button>
                <el-radio-button value="auto">{{ $t('compute.policy.mode_auto') }}</el-radio-button>
            </el-radio-group>
            <span class="input-help">{{ $t('compute.policy.modeHelper_' + form.mode) }}</span>
        </el-form-item>
        <el-row :gutter="12">
            <el-col :span="12">
                <el-form-item :label="$t('compute.pool.replicas')">
                    <div class="range">
                        <el-input-number v-model="form.minInstances" :min="0" :max="64" controls-position="right" />
                        <span>–</span>
                        <el-input-number v-model="form.maxInstances" :min="0" :max="64" controls-position="right" />
                    </div>
                    <span v-if="plan" class="input-help">
                        {{
                            $t('compute.pool.hostsNeeded', [
                                form.minInstances * plan.nodeCount,
                                form.maxInstances * plan.nodeCount,
                            ])
                        }}
                    </span>
                </el-form-item>
            </el-col>
            <el-col :span="12">
                <el-form-item :label="$t('compute.policy.idle')">
                    <el-input-number v-model="form.idleMinutes" :min="0" :max="1440" controls-position="right" />
                    <span class="input-help">{{ $t('compute.policy.idleHelper') }}</span>
                </el-form-item>
            </el-col>
        </el-row>
        <el-divider content-position="left">{{ $t('compute.pool.scaleUpWhen') }}</el-divider>
        <el-row :gutter="12">
            <el-col :span="8">
                <el-form-item :label="$t('compute.policy.queueHigh')">
                    <el-input-number v-model="form.queueHigh" :min="0.5" :step="0.5" controls-position="right" />
                </el-form-item>
            </el-col>
            <el-col :span="8">
                <el-form-item :label="$t('compute.pool.ttftHigh')">
                    <el-input-number v-model="form.ttftHigh" :min="0" :step="500" controls-position="right" />
                    <span class="input-help">{{ $t('compute.pool.ttftHelper') }}</span>
                </el-form-item>
            </el-col>
            <el-col :span="8">
                <el-form-item :label="$t('compute.policy.kvHigh')">
                    <el-input-number v-model="kvPercent" :min="50" :max="100" controls-position="right" />
                </el-form-item>
            </el-col>
        </el-row>
        <el-collapse>
            <el-collapse-item :title="$t('compute.pool.advanced')" name="adv">
                <el-row :gutter="12">
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
                    <el-col :span="8">
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
            </el-collapse-item>
        </el-collapse>
    </div>
</template>

<script lang="ts" setup>
import { computed } from 'vue';
import { Compute } from '@/api/interface/compute';

const props = defineProps<{ form: Partial<Compute.Policy>; profiles: Compute.Profile[] }>();

const planOptions = computed(() =>
    props.profiles.filter((p) => !props.form.servedName || p.servedName === props.form.servedName),
);
const plan = computed(() => props.profiles.find((p) => p.id === props.form.profileId));
const kvPercent = computed({
    get: () => Math.round((props.form.kvHigh || 0) * 100),
    set: (v: number) => (props.form.kvHigh = v / 100),
});
</script>

<style scoped>
.w-full {
    width: 100%;
}
.range {
    display: flex;
    gap: 8px;
    align-items: center;
}
</style>

<template>
    <DrawerPro
        v-model="visible"
        :header="mode === 'deploy' ? $t('compute.inst.deploy') : $t('compute.inst.switch')"
        :resource="instance?.name || ''"
        size="normal"
        @close="visible = false"
    >
        <el-alert
            type="info"
            :closable="false"
            show-icon
            class="mb-3"
            :title="mode === 'deploy' ? $t('compute.inst.deployHelper') : $t('compute.inst.switchHelper')"
        />
        <el-form v-loading="loading" label-position="top">
            <el-form-item :label="$t('compute.tpl.profile')">
                <el-select v-model="profileId" filterable class="w-full" @change="pre = undefined">
                    <el-option v-for="p in profileOptions" :key="p.id" :label="p.name" :value="p.id">
                        <span>{{ p.name }}</span>
                        <span class="sub ml-2">{{ p.servedName }} · {{ p.nodeCount }}×</span>
                    </el-option>
                </el-select>
                <span v-if="profile" class="input-help">
                    {{ profile.modelPath }} · {{ $t('compute.tpl.nodeCount') }} {{ profile.nodeCount }}
                    <span v-if="profile.needFabric">· {{ $t('compute.tpl.needFabric') }}</span>
                    <span v-if="profile.minMemGb">
                        · ≥{{ profile.minMemGb }} GiB {{ $t('compute.tpl.memPoolHint') }}
                    </span>
                </span>
            </el-form-item>
            <template v-if="mode === 'deploy'">
                <el-form-item :label="$t('compute.inst.placement')">
                    <el-radio-group v-model="auto" @change="pre = undefined">
                        <el-radio :value="true">{{ $t('compute.inst.placeAuto') }}</el-radio>
                        <el-radio :value="false">{{ $t('compute.inst.placeManual') }}</el-radio>
                    </el-radio-group>
                </el-form-item>
                <el-form-item v-if="!auto" :label="$t('compute.inst.hosts')">
                    <el-select v-model="hostIds" multiple filterable class="w-full" @change="pre = undefined">
                        <el-option
                            v-for="h in hosts"
                            :key="h.id"
                            :label="h.name"
                            :value="h.id"
                            :disabled="!h.schedulable || !!h.instanceNames?.length"
                        >
                            <span>{{ h.name }}</span>
                            <span class="sub ml-2">
                                {{ h.domain || '' }}
                                <template v-if="h.instanceNames?.length">· {{ h.instanceNames.join(',') }}</template>
                            </span>
                        </el-option>
                    </el-select>
                    <span class="input-help">{{ $t('compute.inst.headFirst') }}</span>
                </el-form-item>
                <el-form-item :label="$t('compute.inst.name')">
                    <el-input v-model="name" :placeholder="$t('compute.inst.nameAuto')" />
                </el-form-item>
            </template>

            <div v-if="pre" class="mt-2">
                <el-alert
                    :type="pre.ok ? 'success' : 'error'"
                    :closable="false"
                    show-icon
                    :title="pre.ok ? $t('compute.inst.precheckOk') : $t('compute.inst.precheckFail')"
                />
                <div v-for="it in pre.items || []" :key="it.name" class="line pre-item">
                    <span>
                        <el-icon :class="it.ok ? 'stat-success' : 'stat-danger'">
                            <CircleCheckFilled v-if="it.ok" />
                            <CircleCloseFilled v-else />
                        </el-icon>
                        {{ it.name }}
                    </span>
                    <span class="sub">{{ it.message }}</span>
                </div>
            </div>
        </el-form>
        <template #footer>
            <el-button @click="visible = false">{{ $t('commons.button.cancel') }}</el-button>
            <el-button :disabled="loading || !profileId" @click="run(true)">
                {{ $t('compute.inst.precheck') }}
            </el-button>
            <el-button type="primary" :disabled="loading || !pre?.ok" @click="run(false)">
                {{ mode === 'deploy' ? $t('compute.inst.deployNow') : $t('compute.inst.switchNow') }}
            </el-button>
        </template>
    </DrawerPro>
</template>

<script lang="ts" setup>
import { computed, ref } from 'vue';
import { ElMessageBox } from 'element-plus';
import { CircleCheckFilled, CircleCloseFilled } from '@element-plus/icons-vue';
import { Compute } from '@/api/interface/compute';
import { computeDeploy, computeSwitch, listComputeHosts, listComputeProfiles } from '@/api/modules/compute';
import { MsgSuccess } from '@/utils/message';
import { t } from '../helper';

const emit = defineEmits(['started']);
const visible = ref(false);
const loading = ref(false);
const mode = ref<'deploy' | 'switch'>('deploy');
const instance = ref<Compute.Instance>();
const profiles = ref<Compute.Profile[]>([]);
const hosts = ref<Compute.Host[]>([]);
const profileId = ref<number>();
const auto = ref(true);
const hostIds = ref<number[]>([]);
const name = ref('');
const pre = ref<Compute.PrecheckRes>();

const profile = computed(() => profiles.value.find((p) => p.id === profileId.value));
const profileOptions = computed(() => {
    if (mode.value !== 'switch' || !instance.value) return profiles.value;
    const n = (instance.value.hostIds || '').split(',').filter(Boolean).length;
    return profiles.value.filter((p) => p.nodeCount === n && p.id !== instance.value!.profileId);
});

const open = async (m: 'deploy' | 'switch', inst?: Compute.Instance) => {
    mode.value = m;
    instance.value = inst;
    profileId.value = undefined;
    hostIds.value = [];
    auto.value = true;
    name.value = '';
    pre.value = undefined;
    visible.value = true;
    loading.value = true;
    try {
        const [p, h] = await Promise.all([listComputeProfiles(), listComputeHosts()]);
        profiles.value = p.data || [];
        hosts.value = h.data || [];
    } finally {
        loading.value = false;
    }
};

const run = async (dryRun: boolean) => {
    if (!profileId.value) return;
    if (!dryRun) {
        const msg =
            mode.value === 'switch'
                ? t('compute.inst.switchConfirm', [instance.value?.name])
                : t('compute.inst.deployConfirm');
        await ElMessageBox.confirm(msg, t('commons.button.confirm'), { type: 'warning' });
    }
    loading.value = true;
    try {
        const res =
            mode.value === 'deploy'
                ? await computeDeploy({
                      profileId: profileId.value,
                      hostIds: auto.value ? [] : hostIds.value,
                      name: name.value,
                      dryRun,
                  })
                : await computeSwitch({ instanceId: instance.value!.id, profileId: profileId.value, dryRun });
        pre.value = res.data;
        if (!dryRun && res.data.jobId) {
            MsgSuccess(t('compute.job.started', [res.data.jobId]));
            visible.value = false;
            emit('started');
        }
    } finally {
        loading.value = false;
    }
};

defineExpose({ open });
</script>

<style scoped src="../style.css"></style>
<style scoped>
.w-full {
    width: 100%;
}
.pre-item {
    padding: 4px 0;
    border-bottom: 1px dashed var(--el-border-color-lighter);
    align-items: center;
}
</style>

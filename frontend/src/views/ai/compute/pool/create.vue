<template>
    <DrawerPro v-model="visible" :header="$t('compute.pool.create')" size="large" @close="visible = false">
        <el-alert type="info" :closable="false" show-icon class="mb-3" :title="$t('compute.pool.createHelper')" />
        <el-form v-loading="loading" label-position="top" :model="form">
            <el-form-item :label="$t('compute.pool.model')" required>
                <el-select
                    v-model="form.servedName"
                    filterable
                    allow-create
                    class="w-full"
                    :placeholder="$t('compute.pool.modelPlaceholder')"
                    @change="onModel"
                >
                    <el-option v-for="m in modelOptions" :key="m.name" :label="m.name" :value="m.name">
                        <span>{{ m.name }}</span>
                        <span class="sub ml-2">{{ m.hint }}</span>
                    </el-option>
                </el-select>
                <span class="input-help">{{ $t('compute.pool.modelHelper') }}</span>
            </el-form-item>
            <el-form-item :label="$t('compute.pool.serviceName')" required>
                <el-input v-model="form.name" :placeholder="form.servedName" />
                <span class="input-help">{{ $t('compute.pool.serviceNameHelper') }}</span>
            </el-form-item>
            <el-alert
                v-if="form.servedName && !planCount"
                type="warning"
                :closable="false"
                show-icon
                class="mb-3"
                :title="$t('compute.pool.noPlan')"
            >
                <el-button link type="primary" @click="gotoPlans">{{ $t('compute.pool.gotoPlans') }}</el-button>
            </el-alert>
            <PolicyForm :form="form" :profiles="profiles" />
        </el-form>
        <template #footer>
            <el-button @click="visible = false">{{ $t('commons.button.cancel') }}</el-button>
            <el-button type="primary" :loading="loading" :disabled="!form.servedName" @click="onSubmit">
                {{ $t('commons.button.confirm') }}
            </el-button>
        </template>
    </DrawerPro>
</template>

<script lang="ts" setup>
import { computed, reactive, ref } from 'vue';
import { useRouter } from 'vue-router';
import { ElMessageBox } from 'element-plus';
import { Compute } from '@/api/interface/compute';
import { createComputePool, listComputeInstances, listComputePools, listComputeProfiles } from '@/api/modules/compute';
import { t } from '../helper';
import PolicyForm from './policy-form.vue';

const emit = defineEmits(['created']);
const router = useRouter();
const visible = ref(false);
const loading = ref(false);
const profiles = ref<Compute.Profile[]>([]);
const instances = ref<Compute.Instance[]>([]);
const taken = ref<string[]>([]);

// Defaults follow a DGX GB10 fleet: one replica = two hosts over the
// optical link, 1–2 replicas, scale up on queue or slow first token.
const empty = (): Partial<Compute.Pool> & { name: string } => ({
    id: 0,
    name: '',
    servedName: '',
    profileId: 0,
    mode: 'suggest',
    minInstances: 1,
    maxInstances: 2,
    hotSpare: 0,
    priority: 50,
    preemptible: false,
    queueHigh: 4,
    ttftHigh: 3000,
    kvHigh: 0.92,
    sustainRounds: 3,
    idleMinutes: 30,
    cooldownSec: 900,
});
const form = reactive(empty());

const modelOptions = computed(() => {
    const names = new Map<string, string>();
    for (const p of profiles.value) names.set(p.servedName, t('compute.pool.hintPlan', [p.name]));
    for (const i of instances.value) if (!names.has(i.servedName)) names.set(i.servedName, i.name);
    return [...names.entries()]
        .filter(([n]) => !taken.value.includes(n))
        .map(([name, hint]) => ({ name, hint }))
        .sort((a, b) => a.name.localeCompare(b.name));
});
const planCount = computed(() => profiles.value.filter((p) => p.servedName === form.servedName).length);

const onModel = () => {
    const plans = profiles.value.filter((p) => p.servedName === form.servedName);
    // Prefer the plan with the most hosts per replica (the GB10 pair).
    plans.sort((a, b) => b.nodeCount - a.nodeCount);
    form.profileId = plans[0]?.id || 0;
    if (!form.name) form.name = form.servedName;
};

const open = async () => {
    Object.assign(form, empty());
    visible.value = true;
    loading.value = true;
    try {
        const [p, i, pools] = await Promise.all([listComputeProfiles(), listComputeInstances(), listComputePools()]);
        profiles.value = p.data || [];
        instances.value = i.data || [];
        taken.value = (pools.data || []).map((x) => x.servedName);
    } finally {
        loading.value = false;
    }
};

const gotoPlans = () => {
    visible.value = false;
    router.push('/ai/vllm/cluster');
};

const onSubmit = async () => {
    if (!form.name) form.name = form.servedName;
    if (form.mode === 'auto') {
        await ElMessageBox.confirm(t('compute.policy.autoConfirm'), t('compute.policy.mode_auto'), { type: 'warning' });
    }
    loading.value = true;
    try {
        const res = await createComputePool({ ...form, profileId: form.profileId || 0 });
        visible.value = false;
        emit('created', res.data, { ...form });
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
</style>

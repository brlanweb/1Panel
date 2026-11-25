<template>
    <DrawerPro
        v-model="visible"
        :header="$t('compute.pool.detail')"
        :resource="pool?.name || pool?.servedName || ''"
        size="large"
        @close="close"
    >
        <template v-if="pool">
            <el-descriptions :column="2" border class="mb-3">
                <el-descriptions-item :label="$t('compute.pool.model')">
                    <span class="mono">{{ pool.servedName }}</span>
                </el-descriptions-item>
                <el-descriptions-item :label="$t('compute.pool.endpoint')">
                    <span class="mono">{{ pool.baseUrl }}</span>
                </el-descriptions-item>
                <el-descriptions-item :label="$t('compute.pool.replicas')">
                    {{ pool.healthy }} / {{ pool.instances }}
                    <span class="sub ml-2">
                        ({{ $t('compute.pool.range', [pool.minInstances, pool.maxInstances]) }})
                    </span>
                </el-descriptions-item>
                <el-descriptions-item :label="$t('compute.pool.service')">
                    <span v-if="pool.accountId && !pool.serviceMissing">{{ pool.name }}</span>
                    <el-tag v-else size="small" type="warning" effect="plain">
                        {{ $t('compute.pool.noService') }}
                    </el-tag>
                </el-descriptions-item>
            </el-descriptions>
            <el-radio-group v-model="tab" class="mb-3">
                <el-radio-button value="instances">{{ $t('compute.pool.tabInstances') }}</el-radio-button>
                <el-radio-button value="policy">{{ $t('compute.pool.tabPolicy') }}</el-radio-button>
                <el-radio-button value="jobs">{{ $t('compute.pool.tabJobs') }}</el-radio-button>
            </el-radio-group>
            <InstancesTab v-if="tab === 'instances'" ref="tabRef" :served-name="pool.servedName" />
            <JobsTab v-else-if="tab === 'jobs'" ref="tabRef" :served-name="pool.servedName" />
            <el-form v-else label-position="top" :model="form">
                <PolicyForm :form="form" :profiles="profiles" />
                <el-button type="primary" :loading="saving" @click="onSave">{{ $t('commons.button.save') }}</el-button>
            </el-form>
        </template>
    </DrawerPro>
</template>

<script lang="ts" setup>
import { nextTick, reactive, ref, watch } from 'vue';
import { ElMessageBox } from 'element-plus';
import { Compute } from '@/api/interface/compute';
import { listComputeProfiles, updateComputePool } from '@/api/modules/compute';
import { MsgSuccess } from '@/utils/message';
import { t } from '../helper';
import PolicyForm from './policy-form.vue';
import InstancesTab from '../instances/index.vue';
import JobsTab from '../jobs/index.vue';

const emit = defineEmits(['changed']);
const visible = ref(false);
const saving = ref(false);
const tab = ref('instances');
const tabRef = ref<any>();
const pool = ref<Compute.Pool>();
const profiles = ref<Compute.Profile[]>([]);
const form = reactive<Partial<Compute.Pool>>({});

const open = async (row: Compute.Pool) => {
    pool.value = row;
    Object.keys(form).forEach((k) => delete (form as any)[k]);
    Object.assign(form, row);
    tab.value = 'instances';
    visible.value = true;
    const res = await listComputeProfiles();
    profiles.value = res.data || [];
    nextTick(() => tabRef.value?.refresh?.(true));
};

const close = () => {
    visible.value = false;
    emit('changed');
};

// Called by the page poll while the drawer is open.
const refresh = async () => {
    if (tab.value !== 'policy') await tabRef.value?.refresh?.(false);
};

watch(tab, () => nextTick(() => tabRef.value?.refresh?.(true)));

const onSave = async () => {
    if (form.mode === 'auto' && pool.value?.mode !== 'auto') {
        await ElMessageBox.confirm(t('compute.policy.autoConfirm'), t('compute.policy.mode_auto'), { type: 'warning' });
    }
    saving.value = true;
    try {
        await updateComputePool({ ...form, profileId: form.profileId || 0 });
        MsgSuccess(t('commons.msg.operationSuccess'));
        Object.assign(pool.value!, form);
        emit('changed');
    } finally {
        saving.value = false;
    }
};

defineExpose({ open, refresh, visible });
</script>

<style scoped src="../style.css"></style>

<template>
    <DialogPro v-model="visible" :title="$t('compute.inst.adopt')" size="small" @close="visible = false">
        <el-alert type="info" :closable="false" show-icon class="mb-3" :title="$t('compute.inst.adoptHelper')" />
        <el-form v-loading="loading" :model="form" label-position="top">
            <el-form-item :label="$t('compute.inst.name')">
                <el-input v-model="form.name" />
            </el-form-item>
            <el-form-item :label="$t('compute.servedName')">
                <el-input v-model="form.servedName" />
                <span class="input-help">{{ $t('compute.inst.servedHelper') }}</span>
            </el-form-item>
            <el-form-item :label="$t('compute.inst.workers')">
                <el-select v-model="form.workerHostIds" multiple clearable class="w-full">
                    <el-option v-for="h in others" :key="h.id" :label="h.name" :value="h.id" />
                </el-select>
                <span class="input-help">{{ $t('compute.inst.workersHelper') }}</span>
            </el-form-item>
            <el-form-item :label="$t('compute.inst.apiKey')">
                <el-input v-model="form.apiKey" type="password" show-password />
                <span class="input-help">{{ $t('compute.inst.apiKeyHelper') }}</span>
            </el-form-item>
        </el-form>
        <template #footer>
            <el-button @click="visible = false">{{ $t('commons.button.cancel') }}</el-button>
            <el-button type="primary" :disabled="loading" @click="onSubmit">
                {{ $t('commons.button.confirm') }}
            </el-button>
        </template>
    </DialogPro>
</template>

<script lang="ts" setup>
import { computed, reactive, ref } from 'vue';
import { Compute } from '@/api/interface/compute';
import { adoptComputeInstance } from '@/api/modules/compute';
import { MsgSuccess } from '@/utils/message';
import { t } from '../helper';

const emit = defineEmits(['saved']);
const visible = ref(false);
const loading = ref(false);
const hosts = ref<Compute.Host[]>([]);
const form = reactive({ hostId: 0, port: 0, name: '', servedName: '', apiKey: '', workerHostIds: [] as number[] });
const others = computed(() => hosts.value.filter((h) => h.id !== form.hostId));

const open = (host: Compute.Host, proc: Compute.Process, all: Compute.Host[]) => {
    hosts.value = all;
    Object.assign(form, {
        hostId: host.id,
        port: proc.port,
        name: `${host.name}-${proc.port}`,
        servedName: proc.servedName,
        apiKey: '',
        workerHostIds: [],
    });
    visible.value = true;
};

const onSubmit = async () => {
    loading.value = true;
    try {
        await adoptComputeInstance({ ...form });
        MsgSuccess(t('commons.msg.operationSuccess'));
        visible.value = false;
        emit('saved');
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

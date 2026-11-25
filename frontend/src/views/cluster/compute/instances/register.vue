<template>
    <DrawerPro
        v-model="visible"
        :header="form.id ? $t('commons.button.edit') : $t('compute.inst.register')"
        :resource="form.name"
        size="small"
        @close="visible = false"
    >
        <el-alert type="info" :closable="false" show-icon class="mb-3" :title="$t('compute.inst.registerHelper')" />
        <el-form ref="formRef" v-loading="loading" :model="form" label-position="top">
            <el-form-item :label="$t('compute.inst.name')" required>
                <el-input v-model="form.name" />
            </el-form-item>
            <el-form-item :label="$t('compute.servedName')" required>
                <el-input v-model="form.servedName" />
                <span class="input-help">{{ $t('compute.inst.servedHelper') }}</span>
            </el-form-item>
            <el-form-item :label="$t('compute.inst.head')" required>
                <el-select v-model="form.headHostId" filterable class="w-full">
                    <el-option v-for="h in hosts" :key="h.id" :label="`${h.name} (${h.addr})`" :value="h.id" />
                </el-select>
            </el-form-item>
            <el-form-item :label="$t('compute.inst.workers')">
                <el-select v-model="form.hostIds" multiple filterable class="w-full">
                    <el-option
                        v-for="h in hosts.filter((x) => x.id !== form.headHostId)"
                        :key="h.id"
                        :label="h.name"
                        :value="h.id"
                    />
                </el-select>
            </el-form-item>
            <el-row :gutter="12">
                <el-col :span="12">
                    <el-form-item :label="$t('compute.host.port')" required>
                        <el-input-number v-model="form.port" :min="1" :max="65535" controls-position="right" />
                    </el-form-item>
                </el-col>
                <el-col :span="12">
                    <el-form-item :label="$t('compute.inst.weight')">
                        <el-input-number v-model="form.weight" :min="0" :max="1000" controls-position="right" />
                    </el-form-item>
                </el-col>
            </el-row>
            <el-form-item :label="$t('compute.inst.container')">
                <el-input v-model="form.containerName" />
                <span class="input-help">{{ $t('compute.inst.containerHelper') }}</span>
            </el-form-item>
            <el-form-item :label="$t('compute.inst.apiKey')">
                <el-input
                    v-model="form.apiKey"
                    type="password"
                    show-password
                    :placeholder="form.id ? $t('compute.host.keepCredential') : ''"
                />
                <span class="input-help">{{ $t('compute.inst.apiKeyHelper') }}</span>
            </el-form-item>
        </el-form>
        <template #footer>
            <el-button @click="visible = false">{{ $t('commons.button.cancel') }}</el-button>
            <el-button
                type="primary"
                :disabled="loading || !form.name || !form.servedName || !form.headHostId"
                @click="onSubmit"
            >
                {{ $t('commons.button.confirm') }}
            </el-button>
        </template>
    </DrawerPro>
</template>

<script lang="ts" setup>
import { reactive, ref } from 'vue';
import { Compute } from '@/api/interface/compute';
import { listComputeHosts, saveComputeInstance } from '@/api/modules/compute';
import { MsgSuccess } from '@/utils/message';
import { t } from '../helper';

const emit = defineEmits(['saved']);
const visible = ref(false);
const loading = ref(false);
const hosts = ref<Compute.Host[]>([]);
const empty = () => ({
    id: 0,
    name: '',
    servedName: '',
    headHostId: undefined as number | undefined,
    hostIds: [] as number[],
    port: 8000,
    weight: 100,
    containerName: '',
    apiKey: '',
});
const form = reactive(empty());

const open = async (row?: Compute.Instance) => {
    Object.assign(form, empty());
    if (row) {
        const ids = (row.hostIds || '').split(',').filter(Boolean).map(Number);
        Object.assign(form, {
            id: row.id,
            name: row.name,
            servedName: row.servedName,
            headHostId: row.headHostId,
            hostIds: ids.filter((i) => i !== row.headHostId),
            port: row.port,
            weight: row.weight,
            containerName: row.containerName,
        });
    }
    visible.value = true;
    const res = await listComputeHosts();
    hosts.value = res.data || [];
};

const onSubmit = async () => {
    loading.value = true;
    try {
        await saveComputeInstance({ ...form });
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

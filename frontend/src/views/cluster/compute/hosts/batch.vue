<template>
    <DrawerPro v-model="visible" :header="$t('compute.host.batch')" size="normal" @close="visible = false">
        <el-form v-loading="loading" :model="form" label-position="top">
            <el-form-item :label="$t('compute.host.batchLines')">
                <el-input
                    v-model="form.lines"
                    type="textarea"
                    :rows="10"
                    class="code-area"
                    :placeholder="placeholder"
                />
                <span class="input-help">{{ $t('compute.host.batchHelper') }}</span>
            </el-form-item>
            <el-row :gutter="12">
                <el-col :span="12">
                    <el-form-item :label="$t('compute.host.user')">
                        <el-input v-model="form.sshUser" />
                    </el-form-item>
                </el-col>
                <el-col :span="12">
                    <el-form-item :label="$t('compute.host.port')">
                        <el-input-number v-model="form.sshPort" :min="1" :max="65535" controls-position="right" />
                    </el-form-item>
                </el-col>
            </el-row>
            <el-form-item :label="$t('compute.host.authMode')">
                <el-radio-group v-model="form.authMode">
                    <el-radio value="password">{{ $t('compute.host.password') }}</el-radio>
                    <el-radio value="key">{{ $t('compute.host.key') }}</el-radio>
                </el-radio-group>
            </el-form-item>
            <el-form-item
                :label="form.authMode === 'key' ? $t('compute.host.privateKey') : $t('compute.host.password')"
            >
                <el-input
                    v-if="form.authMode === 'key'"
                    v-model="form.credential"
                    type="textarea"
                    :rows="5"
                    class="code-area"
                />
                <el-input v-else v-model="form.credential" type="password" show-password />
            </el-form-item>
            <el-form-item v-if="form.authMode === 'key'" :label="$t('compute.host.passPhrase')">
                <el-input v-model="form.passPhrase" type="password" show-password />
            </el-form-item>
            <el-form-item :label="$t('compute.host.labels')">
                <el-input v-model="form.labels" placeholder="gb10" />
            </el-form-item>
            <el-alert
                v-if="result"
                :type="result.error ? 'warning' : 'success'"
                :closable="false"
                show-icon
                :title="$t('compute.host.batchResult', [result.created])"
                :description="result.error"
            />
        </el-form>
        <template #footer>
            <el-button @click="visible = false">{{ $t('commons.button.cancel') }}</el-button>
            <el-button type="primary" :disabled="loading || !form.lines || !form.credential" @click="onSubmit">
                {{ $t('commons.button.confirm') }}
            </el-button>
        </template>
    </DrawerPro>
</template>

<script lang="ts" setup>
import { reactive, ref } from 'vue';
import { batchComputeHosts } from '@/api/modules/compute';
import { Compute } from '@/api/interface/compute';

const emit = defineEmits(['saved']);
const visible = ref(false);
const loading = ref(false);
const result = ref<{ created: number; error: string }>();
const placeholder = 'DGX-A-121,10.15.9.121\nDGX-A-122,10.15.9.122\nDGX-B-211,10.15.9.211,22,root';
const form = reactive<Compute.HostBatchReq>({
    lines: '',
    sshUser: 'root',
    sshPort: 22,
    authMode: 'password',
    credential: '',
    passPhrase: '',
    labels: '',
});

const open = () => {
    result.value = undefined;
    form.lines = '';
    form.credential = '';
    form.passPhrase = '';
    visible.value = true;
};

const onSubmit = async () => {
    loading.value = true;
    try {
        const res = await batchComputeHosts({ ...form });
        result.value = res.data;
        emit('saved');
        if (!res.data.error) visible.value = false;
    } finally {
        loading.value = false;
    }
};

defineExpose({ open });
</script>

<style scoped src="../style.css"></style>

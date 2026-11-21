<template>
    <DrawerPro v-model="open" :header="$t('commons.button.set')" @close="open = false" size="normal">
        <el-form ref="formRef" label-position="top" :model="form" :rules="rules" v-loading="loading">
            <el-form-item :label="$t('vllm.modelDir')" prop="modelDir">
                <el-input v-model.trim="form.modelDir" />
            </el-form-item>
            <el-form-item :label="$t('vllm.tokenizerDir')" prop="tokenizerDir">
                <el-input v-model.trim="form.tokenizerDir" />
            </el-form-item>
            <el-divider content-position="left">Hugging Face</el-divider>
            <el-form-item :label="$t('vllm.hfEndpoint')" prop="hfEndpoint">
                <el-input v-model.trim="form.hfEndpoint" placeholder="https://hf-mirror.com" />
                <span class="input-help">{{ $t('vllm.hfEndpointHelper') }}</span>
            </el-form-item>
            <el-form-item label="HF Token" prop="hfToken">
                <el-input v-model.trim="form.hfToken" type="password" show-password />
            </el-form-item>
            <el-divider content-position="left">ModelScope</el-divider>
            <el-form-item :label="$t('vllm.modelScopeUrl')" prop="modelScopeUrl">
                <el-input v-model.trim="form.modelScopeUrl" placeholder="https://www.modelscope.cn" />
            </el-form-item>
            <el-form-item label="ModelScope Token" prop="modelScopeToken">
                <el-input v-model.trim="form.modelScopeToken" type="password" show-password />
            </el-form-item>
            <el-divider content-position="left">{{ $t('vllm.proxy') }}</el-divider>
            <el-form-item :label="$t('vllm.proxyMode')" prop="proxyMode">
                <el-radio-group v-model="form.proxyMode">
                    <el-radio-button value="none">{{ $t('vllm.proxyNone') }}</el-radio-button>
                    <el-radio-button value="node">{{ $t('vllm.proxyNode') }}</el-radio-button>
                    <el-radio-button value="custom">{{ $t('vllm.proxyCustom') }}</el-radio-button>
                </el-radio-group>
                <span class="input-help" v-if="form.proxyMode === 'node'">{{ $t('vllm.proxyNodeHelper') }}</span>
            </el-form-item>
            <el-form-item v-if="form.proxyMode === 'custom'" :label="$t('vllm.proxyUrl')" prop="proxyUrl">
                <el-input v-model.trim="form.proxyUrl" placeholder="http://127.0.0.1:7890" />
                <span class="input-help">{{ $t('vllm.proxyUrlHelper') }}</span>
            </el-form-item>
            <el-form-item v-if="form.proxyMode !== 'none'" prop="proxyDeploy">
                <el-checkbox v-model="form.proxyDeploy">{{ $t('vllm.proxyDeploy') }}</el-checkbox>
                <span class="input-help">{{ $t('vllm.proxyDeployHelper') }}</span>
            </el-form-item>
        </el-form>
        <template #footer>
            <el-button @click="open = false" :disabled="loading">{{ $t('commons.button.cancel') }}</el-button>
            <el-button type="primary" @click="submit" :disabled="loading">
                {{ $t('commons.button.save') }}
            </el-button>
        </template>
    </DrawerPro>
</template>

<script lang="ts" setup>
import { reactive, ref } from 'vue';
import { FormInstance } from 'element-plus';
import { Rules } from '@/global/form-rules';
import { Vllm } from '@/api/interface/vllm';
import { loadVllmSettings, updateVllmSettings } from '@/api/modules/vllm';
import { MsgSuccess } from '@/utils/message';
import i18n from '@/lang';

const open = ref(false);
const loading = ref(false);
const formRef = ref<FormInstance>();
const form = reactive<Vllm.Settings>({
    modelDir: '',
    tokenizerDir: '',
    hfEndpoint: '',
    hfToken: '',
    modelScopeUrl: '',
    modelScopeToken: '',
    proxyMode: 'none',
    proxyUrl: '',
    proxyDeploy: false,
});
const rules = reactive({
    modelDir: [Rules.requiredInput],
    tokenizerDir: [Rules.requiredInput],
    proxyUrl: [Rules.requiredInput],
});

const acceptParams = async () => {
    open.value = true;
    loading.value = true;
    try {
        const res = await loadVllmSettings();
        Object.assign(form, res.data);
        if (!form.proxyMode) form.proxyMode = 'none';
    } finally {
        loading.value = false;
    }
};

const submit = async () => {
    if (!formRef.value) return;
    await formRef.value.validate(async (valid) => {
        if (!valid) return;
        loading.value = true;
        try {
            await updateVllmSettings({ ...form });
            MsgSuccess(i18n.global.t('commons.msg.operationSuccess'));
            open.value = false;
        } finally {
            loading.value = false;
        }
    });
};

defineExpose({ acceptParams });
</script>

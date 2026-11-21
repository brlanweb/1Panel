<template>
    <DrawerPro v-model="open" :header="$t('vllm.downloadModel')" @close="open = false" size="normal">
        <el-form ref="formRef" label-position="top" :model="form" :rules="rules" v-loading="loading">
            <el-form-item :label="$t('vllm.source')" prop="source">
                <el-radio-group v-model="form.source">
                    <el-radio-button value="modelscope">ModelScope</el-radio-button>
                    <el-radio-button value="huggingface">Hugging Face</el-radio-button>
                </el-radio-group>
            </el-form-item>
            <el-form-item :label="$t('vllm.repoId')" prop="repoId">
                <el-input v-model.trim="form.repoId" placeholder="Qwen/Qwen3-8B" />
                <span class="input-help">{{ $t('vllm.repoIdHelper') }}</span>
            </el-form-item>
            <el-form-item :label="$t('commons.table.type')" prop="kind">
                <el-radio-group v-model="form.kind">
                    <el-radio value="model">{{ $t('vllm.kindModel') }}</el-radio>
                    <el-radio value="tokenizer">{{ $t('vllm.kindTokenizer') }}</el-radio>
                </el-radio-group>
            </el-form-item>
            <el-form-item :label="$t('vllm.revision')" prop="revision">
                <el-input v-model.trim="form.revision" placeholder="main / master" />
            </el-form-item>
            <el-alert type="info" :closable="false" show-icon :title="$t('vllm.downloadHelper')" />
        </el-form>
        <template #footer>
            <el-button @click="open = false" :disabled="loading">{{ $t('commons.button.cancel') }}</el-button>
            <el-button type="primary" @click="submit" :disabled="loading">
                {{ $t('commons.button.confirm') }}
            </el-button>
        </template>
    </DrawerPro>
</template>

<script lang="ts" setup>
import { reactive, ref } from 'vue';
import { FormInstance } from 'element-plus';
import { Rules } from '@/global/form-rules';
import { downloadVllmModel } from '@/api/modules/vllm';
import { newUUID } from '@/utils/id';
import { MsgSuccess } from '@/utils/message';
import i18n from '@/lang';

const em = defineEmits(['search', 'task']);
const open = ref(false);
const loading = ref(false);
const formRef = ref<FormInstance>();
const form = reactive({ source: 'modelscope', repoId: '', kind: 'model', revision: '' });

const checkRepo = (_rule: any, value: string, callback: any) => {
    if (!/^[A-Za-z0-9][A-Za-z0-9_.-]*\/[A-Za-z0-9][A-Za-z0-9_.-]*$/.test(value || '')) {
        callback(new Error(i18n.global.t('vllm.repoIdHelper')));
        return;
    }
    callback();
};
const rules = reactive({
    source: [Rules.requiredSelect],
    repoId: [Rules.requiredInput, { validator: checkRepo, trigger: 'blur' }],
    kind: [Rules.requiredSelect],
});

const acceptParams = (params?: { repoId?: string; source?: string }) => {
    form.source = params?.source || 'modelscope';
    form.repoId = params?.repoId || '';
    form.kind = 'model';
    form.revision = '';
    open.value = true;
};

const submit = async () => {
    if (!formRef.value) return;
    await formRef.value.validate(async (valid) => {
        if (!valid) return;
        loading.value = true;
        const taskID = newUUID();
        try {
            await downloadVllmModel({ ...form, taskID });
            MsgSuccess(i18n.global.t('vllm.downloadStarted'));
            open.value = false;
            em('search');
            em('task', taskID);
        } finally {
            loading.value = false;
        }
    });
};

defineExpose({ acceptParams });
</script>

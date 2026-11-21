<template>
    <DrawerPro v-model="open" :header="$t('vllm.importLocal')" @close="open = false" size="normal">
        <el-form ref="formRef" label-position="top" :model="form" :rules="rules" v-loading="loading">
            <el-form-item :label="$t('vllm.modelName')" prop="repoId">
                <el-input v-model.trim="form.repoId" placeholder="Qwen/Qwen3-8B" />
            </el-form-item>
            <el-form-item :label="$t('commons.table.type')" prop="kind">
                <el-radio-group v-model="form.kind">
                    <el-radio value="model">{{ $t('vllm.kindModel') }}</el-radio>
                    <el-radio value="tokenizer">{{ $t('vllm.kindTokenizer') }}</el-radio>
                </el-radio-group>
            </el-form-item>
            <el-form-item :label="$t('vllm.dir')" prop="dir">
                <el-input v-model.trim="form.dir" placeholder="/data/models/Qwen3-8B">
                    <template #prepend>
                        <el-button icon="Folder" @click="fileRef.acceptParams({ path: form.dir, dir: true })" />
                    </template>
                </el-input>
                <span class="input-help">{{ $t('vllm.importHelper') }}</span>
            </el-form-item>
        </el-form>
        <template #footer>
            <el-button @click="open = false" :disabled="loading">{{ $t('commons.button.cancel') }}</el-button>
            <el-button type="primary" @click="submit" :disabled="loading">
                {{ $t('commons.button.confirm') }}
            </el-button>
        </template>
        <FileList ref="fileRef" @choose="(p: string) => (form.dir = p)" />
    </DrawerPro>
</template>

<script lang="ts" setup>
import { reactive, ref } from 'vue';
import { FormInstance } from 'element-plus';
import { Rules } from '@/global/form-rules';
import { importVllmModel } from '@/api/modules/vllm';
import { MsgSuccess } from '@/utils/message';
import FileList from '@/components/file-list/index.vue';
import i18n from '@/lang';

const em = defineEmits(['search']);
const open = ref(false);
const loading = ref(false);
const formRef = ref<FormInstance>();
const fileRef = ref();
const form = reactive({ repoId: '', kind: 'model', dir: '' });
const rules = reactive({
    repoId: [Rules.requiredInput],
    kind: [Rules.requiredSelect],
    dir: [Rules.requiredInput],
});

const acceptParams = () => {
    form.repoId = '';
    form.kind = 'model';
    form.dir = '';
    open.value = true;
};

const submit = async () => {
    if (!formRef.value) return;
    await formRef.value.validate(async (valid) => {
        if (!valid) return;
        loading.value = true;
        try {
            await importVllmModel({ ...form });
            MsgSuccess(i18n.global.t('commons.msg.operationSuccess'));
            open.value = false;
            em('search');
        } finally {
            loading.value = false;
        }
    });
};

defineExpose({ acceptParams });
</script>

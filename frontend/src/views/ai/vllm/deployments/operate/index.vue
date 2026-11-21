<template>
    <DrawerPro v-model="open" :header="title" :resource="form.id ? form.name : ''" @close="open = false" size="large">
        <el-form ref="formRef" label-position="top" :model="form" :rules="rules" v-loading="loading">
            <el-form-item :label="$t('commons.table.name')" prop="name">
                <el-input v-model.trim="form.name" :disabled="!!form.id" />
            </el-form-item>

            <template v-if="form.external">
                <el-form-item :label="$t('vllm.baseUrl')" prop="baseUrl">
                    <el-input v-model.trim="form.baseUrl" placeholder="http://192.168.1.10:8000/v1" />
                    <span class="input-help">{{ $t('vllm.baseUrlHelper') }}</span>
                </el-form-item>
                <el-form-item :label="$t('vllm.servedName')" prop="servedName">
                    <el-input v-model.trim="form.servedName" />
                </el-form-item>
                <el-form-item label="API Key" prop="apiKey">
                    <el-input v-model.trim="form.apiKey" type="password" show-password />
                </el-form-item>
            </template>

            <template v-else>
                <el-form-item label="Recipe">
                    <el-select v-model="form.recipeId" clearable filterable @change="applyRecipe">
                        <el-option :label="$t('vllm.noRecipe')" :value="0" />
                        <el-option v-for="r in recipes" :key="r.id" :label="r.name" :value="r.id" />
                    </el-select>
                    <span class="input-help">{{ $t('vllm.recipeHelper') }}</span>
                </el-form-item>
                <el-form-item :label="$t('vllm.kindModel')" prop="modelId">
                    <el-select v-model="form.modelId" filterable @change="onModelChange">
                        <el-option v-for="m in models" :key="m.id" :label="m.repoId" :value="m.id" />
                    </el-select>
                    <span class="input-help" v-if="models.length === 0">{{ $t('vllm.noModelHelper') }}</span>
                </el-form-item>
                <el-form-item :label="$t('vllm.servedName')" prop="servedName">
                    <el-input v-model.trim="form.servedName" />
                    <span class="input-help">{{ $t('vllm.servedNameHelper') }}</span>
                </el-form-item>
                <el-form-item :label="$t('vllm.image')" prop="image">
                    <el-input v-model.trim="form.image" placeholder="vllm/vllm-openai:latest" />
                </el-form-item>
                <el-row :gutter="20">
                    <el-col :span="12">
                        <el-form-item :label="$t('vllm.hostIP')" prop="hostIP">
                            <el-select v-model="form.hostIP" allow-create filterable>
                                <el-option label="127.0.0.1" value="127.0.0.1" />
                                <el-option label="0.0.0.0" value="0.0.0.0" />
                            </el-select>
                        </el-form-item>
                    </el-col>
                    <el-col :span="12">
                        <el-form-item :label="$t('commons.table.port')" prop="port">
                            <el-input-number v-model="form.port" :min="1" :max="65535" class="!w-full" />
                        </el-form-item>
                    </el-col>
                </el-row>
                <el-row :gutter="20">
                    <el-col :span="12">
                        <el-form-item label="GPU" prop="gpuType">
                            <el-select v-model="form.gpuType">
                                <el-option label="NVIDIA" value="nvidia" />
                                <el-option label="CPU" value="cpu" />
                                <el-option label="Ascend" value="ascend" />
                                <el-option label="Intel" value="intel" />
                            </el-select>
                        </el-form-item>
                    </el-col>
                    <el-col :span="12">
                        <el-form-item :label="$t('vllm.gpuDevices')" prop="gpuDevices">
                            <el-input v-model.trim="form.gpuDevices" placeholder="all / 0,1" />
                        </el-form-item>
                    </el-col>
                </el-row>
                <el-row :gutter="20">
                    <el-col :span="8">
                        <el-form-item label="Tensor Parallel">
                            <el-input-number v-model="form.tensorParallel" :min="1" :max="64" class="!w-full" />
                        </el-form-item>
                    </el-col>
                    <el-col :span="8">
                        <el-form-item label="Max Model Len">
                            <el-input-number v-model="form.maxModelLen" :min="0" :step="1024" class="!w-full" />
                        </el-form-item>
                    </el-col>
                    <el-col :span="8">
                        <el-form-item label="GPU Memory Util" prop="gpuMemoryUtil">
                            <el-input v-model.trim="form.gpuMemoryUtil" placeholder="0.9" />
                        </el-form-item>
                    </el-col>
                </el-row>
                <el-form-item label="shm_size">
                    <el-input v-model.trim="form.shmSize" placeholder="16g" />
                </el-form-item>
                <el-form-item label="API Key" prop="apiKey">
                    <el-input v-model.trim="form.apiKey" type="password" show-password />
                    <span class="input-help">{{ $t('vllm.apiKeyHelper') }}</span>
                </el-form-item>
                <el-form-item :label="$t('vllm.engineArgs')" prop="args">
                    <el-input
                        v-model="form.args"
                        type="textarea"
                        :rows="5"
                        placeholder="--enable-prefix-caching&#10;--max-num-seqs 64"
                    />
                    <span class="input-help">{{ $t('vllm.engineArgsHelper') }}</span>
                </el-form-item>
                <el-form-item :label="$t('vllm.env')" prop="env">
                    <el-input v-model="form.env" type="textarea" :rows="3" placeholder="KEY=VALUE" />
                </el-form-item>
            </template>
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
import { computed, reactive, ref } from 'vue';
import { FormInstance } from 'element-plus';
import { Rules } from '@/global/form-rules';
import { Vllm } from '@/api/interface/vllm';
import { pageVllmModels, pageVllmRecipes, saveVllmDeployment } from '@/api/modules/vllm';
import { newUUID } from '@/utils/id';
import { MsgSuccess } from '@/utils/message';
import i18n from '@/lang';

const em = defineEmits(['search', 'task']);
const open = ref(false);
const loading = ref(false);
const formRef = ref<FormInstance>();
const models = ref<Vllm.Model[]>([]);
const recipes = ref<Vllm.Recipe[]>([]);

const empty = (external: boolean): Vllm.Deployment => ({
    id: 0,
    name: '',
    external,
    baseUrl: '',
    apiKey: '',
    image: 'vllm/vllm-openai:latest',
    hostIP: '127.0.0.1',
    port: 8000,
    modelId: undefined as any,
    servedName: '',
    gpuType: 'nvidia',
    gpuDevices: 'all',
    tensorParallel: 1,
    maxModelLen: 0,
    gpuMemoryUtil: '0.9',
    shmSize: '16g',
    args: '',
    env: '',
    recipeId: 0,
    taskID: '',
});
const form = reactive<Vllm.Deployment>(empty(false));

const title = computed(() => {
    if (form.id) return i18n.global.t('commons.button.edit');
    return form.external ? i18n.global.t('vllm.createExternal') : i18n.global.t('vllm.createInstance');
});

const checkName = (_r: any, v: string, cb: any) => {
    if (!/^[a-zA-Z0-9][a-zA-Z0-9_.-]{0,62}$/.test(v || '')) {
        cb(new Error(i18n.global.t('vllm.nameHelper')));
        return;
    }
    cb();
};
const checkUrl = (_r: any, v: string, cb: any) => {
    if (!/^https?:\/\/[^\s/]+/.test(v || '')) {
        cb(new Error(i18n.global.t('vllm.baseUrlHelper')));
        return;
    }
    cb();
};
const checkUtil = (_r: any, v: string, cb: any) => {
    if (!v) return cb();
    const f = Number(v);
    if (isNaN(f) || f <= 0 || f > 1) {
        cb(new Error('0 < value <= 1'));
        return;
    }
    cb();
};
const rules = reactive({
    name: [Rules.requiredInput, { validator: checkName, trigger: 'blur' }],
    baseUrl: [Rules.requiredInput, { validator: checkUrl, trigger: 'blur' }],
    modelId: [Rules.requiredSelect],
    port: [Rules.requiredInput, Rules.port],
    gpuMemoryUtil: [{ validator: checkUtil, trigger: 'blur' }],
});

const loadOptions = async () => {
    const [m, r] = await Promise.all([
        pageVllmModels({ page: 1, pageSize: 500, info: '', kind: 'model' }),
        pageVllmRecipes({ page: 1, pageSize: 500, info: '' }),
    ]);
    models.value = (m.data.items || []).filter((i) => i.status === 'Success');
    recipes.value = r.data.items || [];
};

const acceptParams = async (params: Partial<Vllm.Deployment>) => {
    Object.assign(form, empty(!!params.external), params.id ? params : { external: !!params.external });
    if (!form.recipeId) form.recipeId = 0;
    open.value = true;
    if (!form.external) {
        loading.value = true;
        try {
            await loadOptions();
        } finally {
            loading.value = false;
        }
    }
};

const onModelChange = (id: number) => {
    const m = models.value.find((i) => i.id === id);
    if (m && !form.servedName) {
        form.servedName = m.repoId.split('/').pop() || '';
    }
};

const applyRecipe = (id: number) => {
    const r = recipes.value.find((i) => i.id === id);
    if (!r) return;
    if (r.image) form.image = r.image;
    form.gpuType = r.gpuType || form.gpuType;
    form.args = r.args || '';
    form.env = r.env || '';
    if (r.modelRepo && !form.modelId) {
        const m = models.value.find((i) => i.repoId === r.modelRepo);
        if (m) {
            form.modelId = m.id;
            onModelChange(m.id);
        }
    }
};

const submit = async () => {
    if (!formRef.value) return;
    await formRef.value.validate(async (valid) => {
        if (!valid) return;
        loading.value = true;
        const taskID = newUUID();
        try {
            await saveVllmDeployment({ ...form, taskID });
            MsgSuccess(i18n.global.t('commons.msg.operationSuccess'));
            open.value = false;
            em('search');
            if (!form.external) em('task', taskID);
        } finally {
            loading.value = false;
        }
    });
};

defineExpose({ acceptParams });
</script>

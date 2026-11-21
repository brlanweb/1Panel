<template>
    <DrawerPro v-model="open" header="vLLM Recipes" @close="open = false" size="large">
        <div class="flex justify-between mb-3">
            <el-button type="primary" @click="openEdit()">{{ $t('commons.button.create') }}</el-button>
            <TableSearch @search="search()" v-model:searchName="info" />
        </div>
        <el-table :data="items" v-loading="loading">
            <el-table-column :label="$t('commons.table.name')" prop="name" min-width="160" show-overflow-tooltip>
                <template #default="{ row }">
                    {{ row.name }}
                    <el-tag v-if="row.builtin" size="small" class="ml-1">{{ $t('vllm.builtin') }}</el-tag>
                </template>
            </el-table-column>
            <el-table-column label="GPU" prop="gpuType" width="90" />
            <el-table-column :label="$t('vllm.repoId')" prop="modelRepo" min-width="160" show-overflow-tooltip />
            <el-table-column
                :label="$t('commons.table.description')"
                prop="description"
                min-width="200"
                show-overflow-tooltip
            />
            <el-table-column :label="$t('commons.table.operate')" width="150" fixed="right">
                <template #default="{ row }">
                    <el-button link type="primary" @click="openEdit(row)">{{ $t('commons.button.edit') }}</el-button>
                    <el-button link type="danger" @click="onDelete(row)">
                        {{ $t('commons.button.delete') }}
                    </el-button>
                </template>
            </el-table-column>
        </el-table>
        <el-pagination
            class="mt-3 float-right"
            layout="total, prev, pager, next"
            :total="total"
            :page-size="pageSize"
            v-model:current-page="page"
            @current-change="search"
        />

        <DialogPro v-model="editOpen" :title="form.id ? $t('commons.button.edit') : $t('commons.button.create')">
            <el-form ref="formRef" label-position="top" :model="form" :rules="rules">
                <el-form-item :label="$t('commons.table.name')" prop="name">
                    <el-input v-model.trim="form.name" />
                </el-form-item>
                <el-form-item label="GPU" prop="gpuType">
                    <el-select v-model="form.gpuType">
                        <el-option label="NVIDIA" value="nvidia" />
                        <el-option label="CPU" value="cpu" />
                        <el-option label="Ascend" value="ascend" />
                        <el-option label="Intel" value="intel" />
                    </el-select>
                </el-form-item>
                <el-form-item :label="$t('vllm.repoId')" prop="modelRepo">
                    <el-input v-model.trim="form.modelRepo" placeholder="Qwen/Qwen3-8B" />
                </el-form-item>
                <el-form-item :label="$t('vllm.image')" prop="image">
                    <el-input v-model.trim="form.image" placeholder="vllm/vllm-openai:latest" />
                </el-form-item>
                <el-form-item :label="$t('vllm.engineArgs')" prop="args">
                    <el-input v-model="form.args" type="textarea" :rows="5" placeholder="--max-num-seqs 64" />
                    <span class="input-help">{{ $t('vllm.engineArgsHelper') }}</span>
                </el-form-item>
                <el-form-item :label="$t('vllm.env')" prop="env">
                    <el-input v-model="form.env" type="textarea" :rows="3" placeholder="KEY=VALUE" />
                </el-form-item>
                <el-form-item :label="$t('commons.table.description')" prop="description">
                    <el-input v-model="form.description" type="textarea" :rows="2" />
                </el-form-item>
            </el-form>
            <template #footer>
                <el-button @click="editOpen = false">{{ $t('commons.button.cancel') }}</el-button>
                <el-button type="primary" @click="submit">{{ $t('commons.button.confirm') }}</el-button>
            </template>
        </DialogPro>
    </DrawerPro>
</template>

<script lang="ts" setup>
import { reactive, ref } from 'vue';
import { ElMessageBox, FormInstance } from 'element-plus';
import { Rules } from '@/global/form-rules';
import { Vllm } from '@/api/interface/vllm';
import { deleteVllmRecipe, pageVllmRecipes, saveVllmRecipe } from '@/api/modules/vllm';
import { MsgSuccess } from '@/utils/message';
import i18n from '@/lang';

const open = ref(false);
const loading = ref(false);
const items = ref<Vllm.Recipe[]>([]);
const info = ref('');
const page = ref(1);
const pageSize = 10;
const total = ref(0);
const editOpen = ref(false);
const formRef = ref<FormInstance>();
const empty = (): Vllm.Recipe => ({
    id: 0,
    name: '',
    gpuType: 'nvidia',
    modelRepo: '',
    image: '',
    args: '',
    env: '',
    description: '',
});
const form = reactive<Vllm.Recipe>(empty());
const rules = reactive({ name: [Rules.requiredInput], gpuType: [Rules.requiredSelect] });

const acceptParams = () => {
    open.value = true;
    page.value = 1;
    search();
};

const search = async () => {
    loading.value = true;
    try {
        const res = await pageVllmRecipes({ page: page.value, pageSize, info: info.value });
        items.value = res.data.items || [];
        total.value = res.data.total;
    } finally {
        loading.value = false;
    }
};

const openEdit = (row?: Vllm.Recipe) => {
    Object.assign(form, row ? { ...row } : empty());
    editOpen.value = true;
};

const submit = async () => {
    if (!formRef.value) return;
    await formRef.value.validate(async (valid) => {
        if (!valid) return;
        await saveVllmRecipe({ ...form });
        MsgSuccess(i18n.global.t('commons.msg.operationSuccess'));
        editOpen.value = false;
        search();
    });
};

const onDelete = (row: Vllm.Recipe) => {
    ElMessageBox.confirm(i18n.global.t('vllm.deleteRecipeHelper', [row.name]), i18n.global.t('commons.button.delete'), {
        type: 'warning',
    }).then(async () => {
        await deleteVllmRecipe({ id: row.id });
        MsgSuccess(i18n.global.t('commons.msg.deleteSuccess'));
        search();
    });
};

defineExpose({ acceptParams });
</script>

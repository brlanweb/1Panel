<template>
    <DrawerPro
        v-model="visible"
        :header="$t('compute.host.edit')"
        :resource="form.name"
        size="small"
        @close="visible = false"
    >
        <el-alert type="info" :closable="false" show-icon class="mb-3">
            <template #title>
                {{ $t('compute.host.accessManagedIn', [sourceLabel]) }}
                <el-button link type="primary" @click="goSource">{{ $t('compute.host.openSource') }}</el-button>
            </template>
        </el-alert>
        <el-form ref="formRef" :model="form" :rules="rules" label-position="top">
            <el-form-item :label="$t('commons.table.name')" prop="name">
                <el-input v-model="form.name" />
            </el-form-item>
            <el-form-item :label="$t('compute.host.addr')" prop="addr">
                <el-input v-model="form.addr" />
                <span class="input-help">{{ $t('compute.host.addrHelper') }}</span>
            </el-form-item>
            <el-form-item :label="$t('compute.host.labels')">
                <el-input v-model="form.labels" placeholder="gb10,rack-a" />
            </el-form-item>
            <el-form-item :label="$t('commons.table.description')">
                <el-input v-model="form.remark" type="textarea" :rows="2" />
            </el-form-item>
            <el-form-item>
                <el-checkbox v-model="form.schedulable">{{ $t('compute.host.schedulable') }}</el-checkbox>
            </el-form-item>
        </el-form>
        <template #footer>
            <el-button @click="visible = false">{{ $t('commons.button.cancel') }}</el-button>
            <el-button type="primary" :loading="saving" @click="onSave">{{ $t('commons.button.confirm') }}</el-button>
        </template>
    </DrawerPro>
</template>

<script lang="ts" setup>
import { computed, reactive, ref } from 'vue';
import { useRouter } from 'vue-router';
import type { FormInstance } from 'element-plus';
import { Compute } from '@/api/interface/compute';
import { updateComputeHost } from '@/api/modules/compute';
import { Rules } from '@/global/form-rules';
import { MsgSuccess } from '@/utils/message';
import { t } from '../helper';

const emit = defineEmits(['saved']);
const router = useRouter();
const visible = ref(false);
const saving = ref(false);
const formRef = ref<FormInstance>();
const source = ref<Compute.Host['source']>('terminal');
const form = reactive<Compute.HostReq>({ id: 0, name: '', addr: '', labels: '', remark: '', schedulable: true });
const rules = reactive({ name: [Rules.requiredInput] });

const sourceLabel = computed(() => t('compute.host.source_' + source.value));

const open = (row: Compute.Host) => {
    source.value = row.source;
    Object.assign(form, {
        id: row.id,
        name: row.name,
        addr: row.addr,
        labels: row.labels,
        remark: row.remark,
        schedulable: row.schedulable,
    });
    visible.value = true;
};

const goSource = () => {
    visible.value = false;
    router.push(source.value === 'terminal' ? '/terminal' : '/cluster/nodes');
};

const onSave = async () => {
    if (!(await formRef.value?.validate().catch(() => false))) return;
    saving.value = true;
    try {
        await updateComputeHost({ ...form });
        MsgSuccess(t('commons.msg.operationSuccess'));
        visible.value = false;
        emit('saved');
    } finally {
        saving.value = false;
    }
};

defineExpose({ open });
</script>

<template>
    <DrawerPro
        v-model="visible"
        :header="isEdit ? $t('compute.host.edit') : $t('compute.host.add')"
        :resource="form.name"
        size="small"
        @close="visible = false"
    >
        <el-alert type="info" :closable="false" show-icon class="mb-3" :title="$t('compute.host.agentlessHelper')" />
        <el-form ref="formRef" v-loading="loading" :model="form" :rules="rules" label-position="top">
            <el-form-item :label="$t('commons.table.name')" prop="name">
                <el-input v-model="form.name" placeholder="DGX-A-121" />
            </el-form-item>
            <el-row :gutter="12">
                <el-col :span="16">
                    <el-form-item :label="$t('compute.host.addr')" prop="addr">
                        <el-input v-model="form.addr" placeholder="10.15.9.121" />
                    </el-form-item>
                </el-col>
                <el-col :span="8">
                    <el-form-item :label="$t('compute.host.port')" prop="sshPort">
                        <el-input-number
                            v-model="form.sshPort"
                            :min="1"
                            :max="65535"
                            controls-position="right"
                            class="w-full"
                        />
                    </el-form-item>
                </el-col>
            </el-row>
            <el-form-item :label="$t('compute.host.user')" prop="sshUser">
                <el-input v-model="form.sshUser" />
                <span class="input-help">{{ $t('compute.host.userHelper') }}</span>
            </el-form-item>
            <el-form-item :label="$t('compute.host.authMode')">
                <el-radio-group v-model="form.authMode">
                    <el-radio value="password">{{ $t('compute.host.password') }}</el-radio>
                    <el-radio value="key">{{ $t('compute.host.key') }}</el-radio>
                </el-radio-group>
            </el-form-item>
            <el-form-item
                :label="form.authMode === 'key' ? $t('compute.host.privateKey') : $t('compute.host.password')"
                prop="credential"
            >
                <el-input
                    v-if="form.authMode === 'key'"
                    v-model="form.credential"
                    type="textarea"
                    :rows="5"
                    class="code-area"
                    :placeholder="
                        isEdit && hasCredential
                            ? $t('compute.host.keepCredential')
                            : '-----BEGIN OPENSSH PRIVATE KEY-----'
                    "
                />
                <el-input
                    v-else
                    v-model="form.credential"
                    type="password"
                    show-password
                    :placeholder="isEdit && hasCredential ? $t('compute.host.keepCredential') : ''"
                />
            </el-form-item>
            <el-form-item v-if="form.authMode === 'key'" :label="$t('compute.host.passPhrase')">
                <el-input v-model="form.passPhrase" type="password" show-password />
            </el-form-item>
            <el-form-item :label="$t('compute.host.labels')">
                <el-input v-model="form.labels" placeholder="gb10,rack-a" />
            </el-form-item>
            <el-form-item :label="$t('commons.table.description')">
                <el-input v-model="form.remark" type="textarea" :rows="2" />
            </el-form-item>
            <el-alert
                v-if="testRes"
                :type="testRes.ok ? 'success' : 'error'"
                :closable="false"
                show-icon
                :title="testRes.ok ? $t('compute.host.testOk', [testRes.latencyMs]) : $t('compute.host.testFail')"
            >
                <template #default>
                    <div v-if="testRes.ok">
                        {{ testRes.hostname }} · {{ testRes.os }} · {{ testRes.arch }}
                        <br />
                        {{ testRes.gpuModel || $t('compute.host.noGpu') }}
                        <span v-if="testRes.driver">· {{ $t('compute.host.driver') }} {{ testRes.driver }}</span>
                        <div v-if="!testRes.sudo" class="stat-warning">{{ $t('compute.host.noSudo') }}</div>
                    </div>
                    <div v-else>{{ testRes.message }}</div>
                </template>
            </el-alert>
        </el-form>
        <template #footer>
            <el-button :disabled="loading" @click="visible = false">{{ $t('commons.button.cancel') }}</el-button>
            <el-button :disabled="loading" @click="onTest">{{ $t('compute.host.test') }}</el-button>
            <el-button :disabled="loading" type="primary" @click="onSubmit">
                {{ $t('commons.button.confirm') }}
            </el-button>
        </template>
    </DrawerPro>
</template>

<script lang="ts" setup>
import { computed, reactive, ref } from 'vue';
import type { FormInstance, FormRules } from 'element-plus';
import { Compute } from '@/api/interface/compute';
import { createComputeHost, testComputeHost, updateComputeHost } from '@/api/modules/compute';
import { MsgSuccess } from '@/utils/message';
import { t } from '../helper';

const emit = defineEmits(['saved']);
const visible = ref(false);
const loading = ref(false);
const formRef = ref<FormInstance>();
const hasCredential = ref(false);
const testRes = ref<Compute.HostTestRes>();

const empty = (): Compute.HostReq => ({
    id: 0,
    name: '',
    addr: '',
    sshPort: 22,
    sshUser: 'root',
    authMode: 'password',
    credential: '',
    passPhrase: '',
    labels: '',
    remark: '',
});
const form = reactive<Compute.HostReq>(empty());
const isEdit = computed(() => !!form.id);

const rules = computed<FormRules>(() => ({
    name: [{ required: true, message: t('commons.rule.requiredInput'), trigger: 'blur' }],
    addr: [{ required: true, message: t('commons.rule.requiredInput'), trigger: 'blur' }],
    credential: [
        {
            required: !(isEdit.value && hasCredential.value),
            message: t('commons.rule.requiredInput'),
            trigger: 'blur',
        },
    ],
}));

const open = (row?: Compute.Host) => {
    Object.assign(form, empty());
    testRes.value = undefined;
    hasCredential.value = false;
    if (row) {
        Object.assign(form, {
            id: row.id,
            name: row.name,
            addr: row.addr,
            sshPort: row.sshPort,
            sshUser: row.sshUser,
            authMode: row.authMode || 'password',
            labels: row.labels,
            remark: row.remark,
            schedulable: row.schedulable,
        });
        hasCredential.value = row.hasCredential;
    }
    visible.value = true;
};

const onTest = async () => {
    loading.value = true;
    testRes.value = undefined;
    try {
        const res = await testComputeHost({ ...form });
        testRes.value = res.data;
    } finally {
        loading.value = false;
    }
};

const onSubmit = async () => {
    if (!formRef.value) return;
    await formRef.value.validate();
    loading.value = true;
    try {
        if (isEdit.value) await updateComputeHost({ ...form });
        else await createComputeHost({ ...form });
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

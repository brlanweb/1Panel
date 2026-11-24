<template>
    <div>
        <DrawerPro
            v-model="operateVisible"
            :header="isEdit ? $t('cluster.editNode') : $t('cluster.createNode')"
            :resource="form.name"
            @close="handleClose"
            size="small"
        >
            <el-form v-loading="loading" ref="formRef" :model="form" :rules="rules" label-position="top">
                <el-form-item :label="$t('cluster.nodeName')" prop="name">
                    <el-input v-model="form.name" />
                </el-form-item>
                <el-form-item :label="$t('cluster.entryUrl')" prop="entryUrl">
                    <el-input v-model="form.entryUrl" :placeholder="$t('cluster.entryUrlPlaceholder')" />
                    <span class="input-help">{{ $t('cluster.entryUrlHelper') }}</span>
                    <span class="input-help">{{ $t('cluster.apiInterfaceHelper') }}</span>
                </el-form-item>
                <el-form-item :label="$t('cluster.apiKey')" prop="apiKey">
                    <el-input
                        type="password"
                        clearable
                        show-password
                        v-model="form.apiKey"
                        :placeholder="isEdit ? $t('cluster.apiKeyKeepPlaceholder') : $t('cluster.apiKeyPlaceholder')"
                    />
                    <span class="input-help">{{ $t('cluster.apiKeyHelper') }}</span>
                </el-form-item>
                <el-form-item :label="$t('cluster.group')" prop="groupId">
                    <el-select v-model="form.groupId" class="p-w-100" clearable>
                        <el-option :label="$t('commons.table.all')" :value="0"></el-option>
                        <el-option v-for="item in groups" :key="item.id" :label="item.name" :value="item.id" />
                    </el-select>
                </el-form-item>
                <el-form-item :label="$t('cluster.remark')" prop="remark">
                    <el-input type="textarea" v-model="form.remark" />
                </el-form-item>
            </el-form>
            <template #footer>
                <span class="dialog-footer">
                    <el-button :disabled="loading" @click="operateVisible = false">
                        {{ $t('commons.button.cancel') }}
                    </el-button>
                    <el-button :disabled="loading || !isEdit" @click="onTest">
                        {{ $t('cluster.testConnection') }}
                    </el-button>
                    <el-button :disabled="loading" type="primary" @click="onSubmit(formRef)">
                        {{ $t('commons.button.confirm') }}
                    </el-button>
                </span>
            </template>
        </DrawerPro>
    </div>
</template>
<script lang="ts" setup>
import { reactive, ref } from 'vue';
import i18n from '@/lang';
import { ElForm } from 'element-plus';
import { createClusterNode, testClusterNode, updateClusterNode } from '@/api/modules/cluster';
import { getGroupList } from '@/api/modules/group';
import { Group } from '@/api/interface/group';
import { Cluster } from '@/api/interface/cluster';
import { Rules } from '@/global/form-rules';
import { MsgError, MsgSuccess } from '@/utils/message';
import bus from '@/global/bus';

const loading = ref(false);
const operateVisible = ref(false);
const isEdit = ref(false);
type FormInstance = InstanceType<typeof ElForm>;
const formRef = ref<FormInstance>();
const groups = ref<Group.GroupInfo[]>([]);

const form = reactive({
    id: 0,
    name: '',
    entryUrl: '',
    remark: '',
    groupId: 0,
    hostId: 0,
    apiKey: '',
});

const urlRule = (_rule: any, value: string, callback: (error?: Error) => void) => {
    if (!value) {
        callback(new Error(i18n.global.t('commons.rule.requiredInput')));
        return;
    }
    if (!/^https?:\/\/.+/.test(value)) {
        callback(new Error(i18n.global.t('cluster.entryUrlInvalid')));
        return;
    }
    callback();
};

const apiKeyRule = (_rule: any, value: string, callback: (error?: Error) => void) => {
    if (!isEdit.value && !value) {
        callback(new Error(i18n.global.t('commons.rule.requiredInput')));
        return;
    }
    callback();
};

const rules = reactive({
    name: [Rules.requiredInput],
    entryUrl: [{ validator: urlRule, trigger: 'blur' }],
    apiKey: [{ validator: apiKeyRule, trigger: 'blur' }],
});

const listGroups = async () => {
    try {
        const res = await getGroupList('cluster_node');
        groups.value = res.data;
    } catch {
        // The request interceptor already reports the error.
    }
};

interface DialogProps {
    row?: Cluster.NodeInfo;
    isSelf?: boolean;
    prefill?: {
        name?: string;
        entryUrl?: string;
        hostId?: number;
        remark?: string;
    };
}
const acceptParams = (params: DialogProps): void => {
    listGroups();
    if (params.row) {
        isEdit.value = true;
        form.id = params.row.id;
        form.name = params.row.name;
        form.entryUrl = params.row.entryUrl;
        form.remark = params.row.remark || '';
        form.groupId = params.row.groupId || 0;
        form.hostId = params.row.hostId || 0;
        form.apiKey = '';
    } else {
        isEdit.value = false;
        form.id = 0;
        form.name = params.prefill?.name || '';
        form.entryUrl = params.isSelf ? window.location.origin : params.prefill?.entryUrl || '';
        form.remark = params.prefill?.remark || '';
        form.groupId = 0;
        form.hostId = params.prefill?.hostId || 0;
        form.apiKey = '';
    }
    operateVisible.value = true;
};
const emit = defineEmits<{ (e: 'search'): void }>();

const handleClose = () => {
    operateVisible.value = false;
};

const onTest = async () => {
    if (!form.id) {
        return;
    }
    loading.value = true;
    await testClusterNode(form.id)
        .then(() => {
            MsgSuccess(i18n.global.t('cluster.testSuccess'));
            emit('search');
        })
        .catch(() => {
            MsgError(i18n.global.t('cluster.testFailed'));
        })
        .finally(() => {
            loading.value = false;
        });
};

const onSubmit = async (formEl: FormInstance | undefined) => {
    if (!formEl) return;
    formEl.validate(async (valid) => {
        if (!valid) return;
        loading.value = true;
        const api = isEdit.value ? updateClusterNode : createClusterNode;
        const req: Cluster.NodeCreate | Cluster.NodeUpdate = isEdit.value
            ? {
                  id: form.id,
                  name: form.name,
                  entryUrl: form.entryUrl,
                  remark: form.remark,
                  groupId: form.groupId,
                  apiKey: form.apiKey,
              }
            : {
                  name: form.name,
                  entryUrl: form.entryUrl,
                  remark: form.remark,
                  groupId: form.groupId,
                  hostId: form.hostId || undefined,
                  apiKey: form.apiKey,
              };
        await api(req as any)
            .then(() => {
                loading.value = false;
                emit('search');
                bus.emit('refreshClusterNodes');
                operateVisible.value = false;
                MsgSuccess(i18n.global.t('commons.msg.operationSuccess'));
            })
            .catch(() => {
                loading.value = false;
            });
    });
};

defineExpose({
    acceptParams,
});
</script>

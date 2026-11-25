<template>
    <DialogPro v-model="visible" :title="$t('compute.pool.keyTitle')" size="normal" @close="visible = false">
        <el-alert type="success" :closable="false" show-icon class="mb-3" :title="$t('compute.pool.keyHelper')" />
        <el-form label-position="top">
            <el-form-item :label="$t('compute.pool.endpoint')">
                <el-input :model-value="baseUrl" readonly>
                    <template #append>
                        <el-button @click="copy(baseUrl)">{{ $t('commons.button.copy') }}</el-button>
                    </template>
                </el-input>
            </el-form-item>
            <el-form-item label="API Key">
                <el-input :model-value="key" readonly>
                    <template #append>
                        <el-button @click="copy(key)">{{ $t('commons.button.copy') }}</el-button>
                    </template>
                </el-input>
                <span class="input-help">{{ $t('compute.access.keyOnce') }}</span>
            </el-form-item>
        </el-form>
        <template #footer>
            <el-button type="primary" @click="visible = false">{{ $t('commons.button.confirm') }}</el-button>
        </template>
    </DialogPro>
</template>

<script lang="ts" setup>
import { ref } from 'vue';
import { listComputePools } from '@/api/modules/compute';
import { MsgSuccess } from '@/utils/message';
import { t } from '../helper';

const visible = ref(false);
const key = ref('');
const baseUrl = ref('');

const open = async (k: string, row?: { baseUrl?: string }) => {
    key.value = k;
    baseUrl.value = row?.baseUrl || '';
    visible.value = true;
    if (!baseUrl.value) {
        const res = await listComputePools();
        baseUrl.value = res.data?.[0]?.baseUrl || '';
    }
};

const copy = async (text: string) => {
    try {
        await navigator.clipboard.writeText(text);
        MsgSuccess(t('commons.msg.copySuccess'));
    } catch {
        // insecure context (http): the value stays selectable
    }
};

defineExpose({ open });
</script>

<style scoped>
.mb-3 {
    margin-bottom: 12px;
}
</style>

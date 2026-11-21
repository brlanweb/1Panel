<template>
    <DialogPro v-model="visible" :title="title" size="large" @close="visible = false">
        <el-alert :title="summary" type="info" :closable="false" show-icon />
        <el-alert v-if="hint" :title="hint" type="warning" :closable="false" class="mt-2" />
        <div class="confirm-rows">
            <div v-for="target in targets" :key="target.key" class="confirm-row">
                <div class="confirm-row__resources">
                    <el-tag
                        v-for="res in resources"
                        :key="res.key"
                        :closable="resources.length > 1"
                        @close="removeResource(res.key)"
                    >
                        {{ res.label }}
                    </el-tag>
                </div>
                <el-icon class="confirm-row__arrow"><Right /></el-icon>
                <el-tag type="success" :closable="targets.length > 1" @close="removeTarget(target.key)">
                    {{ target.label }}
                </el-tag>
            </div>
        </div>
        <template #footer>
            <el-button @click="visible = false">{{ $t('commons.button.cancel') }}</el-button>
            <el-button
                type="primary"
                :loading="submitting"
                :disabled="!resources.length || !targets.length"
                @click="onConfirm"
            >
                {{ $t('commons.button.confirm') }}
            </el-button>
        </template>
    </DialogPro>
</template>

<script lang="ts" setup>
import { computed, ref } from 'vue';
import { Right } from '@element-plus/icons-vue';
import i18n from '@/lang';

interface Item {
    key: string;
    label: string;
}

const props = defineProps({
    title: { type: String, default: '' },
    // i18n key with {0} resources, {1} targets, {2} tasks
    summaryKey: { type: String, required: true },
    hint: { type: String, default: '' },
    submitting: { type: Boolean, default: false },
});
const emit = defineEmits(['confirm']);

const visible = ref(false);
const resources = ref<Item[]>([]);
const targets = ref<Item[]>([]);

const summary = computed(() =>
    i18n.global.t(props.summaryKey, [
        resources.value.length,
        targets.value.length,
        resources.value.length * targets.value.length,
    ]),
);

const removeResource = (key: string) => {
    resources.value = resources.value.filter((item) => item.key !== key);
};
const removeTarget = (key: string) => {
    targets.value = targets.value.filter((item) => item.key !== key);
};

const open = (res: Item[], tgt: Item[]) => {
    resources.value = [...res];
    targets.value = [...tgt];
    visible.value = true;
};
const close = () => {
    visible.value = false;
};

const onConfirm = () => {
    emit(
        'confirm',
        resources.value.map((item) => item.key),
        targets.value.map((item) => item.key),
    );
};

defineExpose({ open, close });
</script>

<style scoped lang="scss">
.confirm-rows {
    margin-top: 12px;
    max-height: 50vh;
    overflow-y: auto;
}
.confirm-row {
    display: flex;
    align-items: center;
    gap: 12px;
    padding: 8px 0;
    border-bottom: 1px dashed var(--el-border-color-lighter);
}
.confirm-row__resources {
    flex: 1;
    display: flex;
    flex-wrap: wrap;
    gap: 6px;
}
.confirm-row__arrow {
    color: var(--el-color-primary);
}
</style>

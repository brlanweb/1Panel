<template>
    <el-card shadow="never" class="target-card">
        <template #header>
            <div class="flex flex-wrap items-center justify-between gap-2">
                <div>
                    <div class="font-medium">{{ $t('xpack.sync.targetTitle') }}</div>
                    <div class="target-desc">{{ $t('xpack.sync.targetDesc') }}</div>
                </div>
                <div class="flex items-center gap-3">
                    <el-input
                        v-if="searchable"
                        v-model="keyword"
                        clearable
                        class="p-w-200"
                        :placeholder="$t('xpack.sync.targetSearchPlaceholder')"
                    />
                    <el-checkbox
                        :model-value="allChecked"
                        :indeterminate="indeterminate"
                        :disabled="!filtered.length || disabled"
                        @change="onCheckAll"
                    >
                        {{ $t('commons.button.selectAll') }}
                    </el-checkbox>
                </div>
            </div>
        </template>
        <slot name="selected"></slot>
        <el-checkbox-group v-if="filtered.length" v-model="selected" :disabled="disabled" class="target-grid">
            <el-checkbox v-for="node in filtered" :key="node.key" :value="node.key" border class="target-item">
                <div class="target-item__body">
                    <span class="target-item__name">{{ node.name }}</span>
                    <el-tag size="small" :type="nodeStatusType(node.status)">
                        {{ nodeStatusLabel(node.status) }}
                    </el-tag>
                </div>
                <div v-if="node.addr" class="target-item__addr">{{ node.addr }}</div>
            </el-checkbox>
        </el-checkbox-group>
        <el-empty
            v-else
            :image-size="60"
            :description="source ? $t('xpack.sync.noTargetNodes') : $t('xpack.sync.noSourceNode')"
        />
        <div class="flex justify-end mt-4">
            <el-button type="primary" :disabled="!canHandle" :loading="submitting" @click="emit('handle')">
                {{ $t('commons.button.handle') }}
            </el-button>
        </div>
    </el-card>
</template>

<script lang="ts" setup>
import { computed, ref, watch } from 'vue';
import { nodeStatusLabel, nodeStatusType, SyncNode } from '../common';

const props = defineProps({
    nodes: { type: Array as () => SyncNode[], default: () => [] },
    // key of the source node; it is excluded from the targets
    source: { type: String, default: '' },
    modelValue: { type: Array as () => string[], default: () => [] },
    hasResource: { type: Boolean, default: false },
    searchable: { type: Boolean, default: true },
    disabled: { type: Boolean, default: false },
    submitting: { type: Boolean, default: false },
});
const emit = defineEmits(['update:modelValue', 'handle']);

const keyword = ref('');

const candidates = computed(() =>
    props.source ? props.nodes.filter((node) => node.key !== props.source && node.status !== 'offline') : [],
);
const filtered = computed(() => {
    const kw = keyword.value.trim().toLowerCase();
    if (!kw) return candidates.value;
    return candidates.value.filter(
        (node) =>
            node.name.toLowerCase().includes(kw) ||
            node.addr.toLowerCase().includes(kw) ||
            node.status.toLowerCase().includes(kw),
    );
});

const selected = computed({
    get: () => props.modelValue,
    set: (value: string[]) => emit('update:modelValue', value),
});

// Drop targets that are no longer candidates (source changed).
watch(candidates, (list) => {
    const keys = new Set(list.map((node) => node.key));
    const kept = props.modelValue.filter((key) => keys.has(key));
    if (kept.length !== props.modelValue.length) emit('update:modelValue', kept);
});

const allChecked = computed(
    () => filtered.value.length > 0 && filtered.value.every((node) => props.modelValue.includes(node.key)),
);
const indeterminate = computed(
    () => !allChecked.value && filtered.value.some((node) => props.modelValue.includes(node.key)),
);
const onCheckAll = (checked: boolean) => {
    const keys = filtered.value.map((node) => node.key);
    if (checked) {
        emit('update:modelValue', Array.from(new Set([...props.modelValue, ...keys])));
    } else {
        emit(
            'update:modelValue',
            props.modelValue.filter((key) => !keys.includes(key)),
        );
    }
};

const canHandle = computed(() => props.hasResource && props.modelValue.length > 0 && !props.disabled);
</script>

<style scoped lang="scss">
.target-card {
    margin-top: 16px;
}
.target-desc {
    font-size: 12px;
    color: var(--el-text-color-secondary);
    margin-top: 4px;
}
.target-grid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(240px, 1fr));
    gap: 12px;
}
.target-item {
    height: auto;
    margin-right: 0 !important;
    padding: 8px 12px;
    align-items: flex-start;
    :deep(.el-checkbox__label) {
        width: 100%;
        min-width: 0;
    }
}
.target-item__body {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 8px;
}
.target-item__name {
    font-weight: 500;
    overflow: hidden;
    text-overflow: ellipsis;
}
.target-item__addr {
    font-size: 12px;
    color: var(--el-text-color-secondary);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
}
</style>

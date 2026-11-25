<template>
    <DrawerPro v-model="visible" :header="$t('compute.host.import')" size="large" @close="visible = false">
        <el-alert type="info" :closable="false" show-icon class="mb-3" :title="$t('compute.host.importHelper')" />
        <el-radio-group v-model="source" class="mb-3" @change="load">
            <el-radio-button value="node">{{ $t('compute.host.source_node') }}</el-radio-button>
            <el-radio-button value="terminal">{{ $t('compute.host.source_terminal') }}</el-radio-button>
            <el-radio-button value="local">{{ $t('compute.host.source_local') }}</el-radio-button>
        </el-radio-group>
        <div class="sub mb-2">{{ $t('compute.host.sourceHelper_' + source) }}</div>
        <el-table
            ref="tableRef"
            v-loading="loading"
            :data="rows"
            row-key="refId"
            :empty-text="$t('compute.host.noCandidate_' + source)"
            @selection-change="(v: Compute.HostCandidate[]) => (selected = v)"
        >
            <el-table-column type="selection" width="45" :selectable="(r: Compute.HostCandidate) => !r.imported" />
            <el-table-column :label="$t('commons.table.name')" prop="name" min-width="150" />
            <el-table-column :label="$t('compute.host.addr')" prop="addr" min-width="130" />
            <el-table-column :label="$t('compute.host.access')" prop="detail" min-width="200" show-overflow-tooltip />
            <el-table-column :label="$t('commons.table.status')" width="110">
                <template #default="{ row }">
                    <el-tag v-if="row.imported" size="small" type="success">{{ $t('compute.host.imported') }}</el-tag>
                    <span v-else class="sub">{{ row.status || '—' }}</span>
                </template>
            </el-table-column>
        </el-table>
        <el-form label-position="top" class="mt-3">
            <el-form-item :label="$t('compute.host.labels')">
                <el-input v-model="labels" placeholder="gb10,rack-a" />
            </el-form-item>
        </el-form>
        <div class="sub">
            {{ $t('compute.host.notListed') }}
            <el-button link type="primary" @click="goSource">{{ $t('compute.host.openSource') }}</el-button>
        </div>
        <template #footer>
            <el-button @click="visible = false">{{ $t('commons.button.cancel') }}</el-button>
            <el-button type="primary" :loading="saving" :disabled="!canImport" @click="onImport">
                {{ $t('compute.host.import') }}
            </el-button>
        </template>
    </DrawerPro>
</template>

<script lang="ts" setup>
import { computed, ref } from 'vue';
import { useRouter } from 'vue-router';
import { Compute } from '@/api/interface/compute';
import { importComputeHosts, listComputeHostCandidates } from '@/api/modules/compute';
import { MsgError, MsgSuccess } from '@/utils/message';
import { t } from '../helper';

const emit = defineEmits(['saved']);
const router = useRouter();
const visible = ref(false);
const loading = ref(false);
const saving = ref(false);
const source = ref<'node' | 'terminal' | 'local'>('node');
const rows = ref<Compute.HostCandidate[]>([]);
const selected = ref<Compute.HostCandidate[]>([]);
const labels = ref('');

const canImport = computed(() =>
    source.value === 'local' ? rows.value.some((r) => !r.imported) : selected.value.length > 0,
);

const load = async () => {
    loading.value = true;
    selected.value = [];
    try {
        const res = await listComputeHostCandidates(source.value);
        rows.value = res.data || [];
    } finally {
        loading.value = false;
    }
};

const open = () => {
    labels.value = '';
    visible.value = true;
    load();
};

const goSource = () => {
    visible.value = false;
    router.push(source.value === 'terminal' ? '/terminal' : '/cluster/nodes');
};

const onImport = async () => {
    saving.value = true;
    try {
        const res = await importComputeHosts({
            source: source.value,
            refIds: selected.value.map((r) => r.refId),
            labels: labels.value,
        });
        if (res.data.error) MsgError(res.data.error);
        MsgSuccess(t('compute.host.importResult', [res.data.imported]));
        visible.value = false;
        emit('saved');
    } finally {
        saving.value = false;
    }
};

defineExpose({ open });
</script>

<style scoped src="../style.css"></style>

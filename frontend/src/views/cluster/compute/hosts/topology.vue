<template>
    <div v-loading="loading">
        <el-alert type="info" :closable="false" show-icon class="mb-3" :title="$t('compute.topo.helper')" />
        <div class="toolbar">
            <el-select v-model="link.hostAId" class="p-w-150" :placeholder="$t('compute.topo.hostA')" filterable>
                <el-option v-for="h in hosts" :key="h.id" :label="h.name" :value="h.id" />
            </el-select>
            <el-input v-model="link.ifaceA" class="p-w-150" :placeholder="$t('compute.topo.iface')" />
            <span>⇄</span>
            <el-select v-model="link.hostBId" class="p-w-150" :placeholder="$t('compute.topo.hostB')" filterable>
                <el-option v-for="h in hosts" :key="h.id" :label="h.name" :value="h.id" />
            </el-select>
            <el-input v-model="link.ifaceB" class="p-w-150" :placeholder="$t('compute.topo.iface')" />
            <el-button :disabled="!link.hostAId || !link.hostBId" @click="onAdd">
                {{ $t('compute.topo.addLink') }}
            </el-button>
        </div>
        <el-empty v-if="!domains.length" :description="$t('compute.topo.empty')" />
        <el-card v-for="d in domains" :key="d.name" shadow="never" class="domain-card">
            <template #header>
                <div class="card-head">
                    <span>
                        <span class="strong">{{ d.name }}</span>
                        <span class="sub ml-2">{{ d.hostIds.map(name).join(' · ') }}</span>
                    </span>
                    <el-tag size="small" :type="d.healthy ? 'success' : 'danger'">
                        {{ d.healthy ? $t('compute.topo.healthy') : $t('compute.topo.unhealthy') }}
                    </el-tag>
                </div>
            </template>
            <div v-for="l in d.links" :key="l.id" class="link-line">
                <el-tag size="small" :type="l.state === 'ACTIVE' ? 'success' : 'danger'" effect="plain">
                    {{ l.state || '—' }}
                </el-tag>
                <span class="strong">{{ name(l.hostAId) }}</span>
                <span class="mono">{{ l.ifaceA }} {{ l.ipA }} {{ l.rdmaA }}</span>
                <span>⇄</span>
                <span class="strong">{{ name(l.hostBId) }}</span>
                <span class="mono">{{ l.ifaceB }} {{ l.ipB }} {{ l.rdmaB }}</span>
                <span v-if="l.rateGbps">{{ l.rateGbps }} Gb/s</span>
                <el-tag size="small" effect="plain" type="info">{{ $t('compute.topo.source_' + l.source) }}</el-tag>
                <el-button link type="danger" size="small" @click="onDelete(l)">
                    {{ $t('commons.button.delete') }}
                </el-button>
            </div>
        </el-card>
    </div>
</template>

<script lang="ts" setup>
import { onMounted, reactive, ref } from 'vue';
import { ElMessageBox } from 'element-plus';
import { Compute } from '@/api/interface/compute';
import { createComputeLink, deleteComputeLink, getComputeTopology } from '@/api/modules/compute';
import { MsgSuccess } from '@/utils/message';
import { t } from '../helper';

const props = defineProps<{ hosts: Compute.Host[] }>();
const loading = ref(false);
const domains = ref<Compute.Domain[]>([]);
const link = reactive({
    hostAId: undefined as number | undefined,
    hostBId: undefined as number | undefined,
    ifaceA: '',
    ifaceB: '',
});

const name = (id: number) => props.hosts.find((h) => h.id === id)?.name || `#${id}`;

const load = async () => {
    loading.value = true;
    try {
        const res = await getComputeTopology();
        domains.value = res.data || [];
    } finally {
        loading.value = false;
    }
};

const onAdd = async () => {
    await createComputeLink({ ...link });
    MsgSuccess(t('commons.msg.operationSuccess'));
    link.ifaceA = link.ifaceB = '';
    load();
};

const onDelete = async (l: Compute.Link) => {
    await ElMessageBox.confirm(t('compute.topo.deleteHelper'), t('commons.button.delete'), { type: 'warning' });
    await deleteComputeLink(l.id);
    load();
};

onMounted(load);
</script>

<style scoped src="../style.css"></style>

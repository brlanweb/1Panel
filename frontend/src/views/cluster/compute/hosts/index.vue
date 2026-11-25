<template>
    <div v-loading="loading">
        <div class="toolbar">
            <el-button type="primary" @click="hostRef.open()">{{ $t('compute.host.add') }}</el-button>
            <el-button @click="batchRef.open()">{{ $t('compute.host.batch') }}</el-button>
            <el-button @click="topoVisible = true">{{ $t('compute.topo.title') }}</el-button>
            <span class="spacer" />
            <el-input v-model="keyword" class="p-w-250" clearable :placeholder="$t('compute.host.search')" />
        </div>
        <el-table :data="rows" row-key="id" :empty-text="$t('compute.host.empty')">
            <el-table-column type="expand">
                <template #default="{ row }">
                    <HostDetail :host="row" @adopt="onAdopt" />
                </template>
            </el-table-column>
            <el-table-column :label="$t('commons.table.name')" min-width="170" fixed="left">
                <template #default="{ row }">
                    <div class="strong">{{ row.name }}</div>
                    <div class="sub">
                        {{ row.addr }}
                        <span v-if="row.hostname">· {{ row.hostname }}</span>
                    </div>
                    <div class="sub" v-if="row.labels">
                        <el-tag v-for="l in row.labels.split(',')" :key="l" size="small" effect="plain" class="mr-1">
                            {{ l }}
                        </el-tag>
                    </div>
                </template>
            </el-table-column>
            <el-table-column :label="$t('commons.table.status')" width="120">
                <template #default="{ row }">
                    <el-tooltip :disabled="!errorOf(row)" :content="errorOf(row)" placement="top">
                        <el-tag
                            size="small"
                            :type="onlineOf(row) ? 'success' : row.status === 'unknown' ? 'info' : 'danger'"
                        >
                            {{ $t('compute.host.status_' + (onlineOf(row) ? 'online' : row.status)) }}
                        </el-tag>
                    </el-tooltip>
                    <div class="sub">
                        <el-tag v-if="!row.schedulable" size="small" type="warning" effect="plain">
                            {{ $t('compute.host.cordoned') }}
                        </el-tag>
                        <span v-else-if="row.realtime.sampledAt">{{ ago(row.realtime.sampledAt) }}</span>
                    </div>
                </template>
            </el-table-column>
            <el-table-column label="GPU" min-width="170">
                <template #default="{ row }">
                    <template v-if="row.realtime.gpus?.length">
                        <el-progress
                            :percentage="pct(maxGpu(row).util)"
                            :stroke-width="8"
                            :color="'var(--el-color-primary)'"
                        />
                        <div class="sub">
                            {{ row.realtime.gpus.length }}× {{ shortGpu(row.realtime.gpus[0].name) }}
                            <span :style="{ color: tempColor(maxGpu(row).temp) }">· {{ maxGpu(row).temp }}°C</span>
                            <span v-if="maxGpu(row).power">· {{ maxGpu(row).power.toFixed(0) }}W</span>
                        </div>
                    </template>
                    <span v-else class="sub">{{ row.gpuModel || '—' }}</span>
                </template>
            </el-table-column>
            <el-table-column :label="$t('compute.host.memory')" min-width="170">
                <template #default="{ row }">
                    <template v-if="row.realtime.memTotal">
                        <el-progress
                            :percentage="pct(row.realtime.memUsedPercent)"
                            :color="barColor(row.realtime.memUsedPercent)"
                            :stroke-width="8"
                        />
                        <div class="sub">
                            {{ $t('compute.host.available') }} {{ size(row.realtime.memAvailable) }} /
                            {{ size(row.realtime.memTotal) }}
                            <el-tag v-if="row.unifiedMem" size="small" effect="plain">
                                {{ $t('compute.host.unified') }}
                            </el-tag>
                        </div>
                    </template>
                    <span v-else>—</span>
                </template>
            </el-table-column>
            <el-table-column label="CPU" width="120">
                <template #default="{ row }">
                    <template v-if="onlineOf(row)">
                        <div>{{ row.realtime.cpuPercent.toFixed(0) }}%</div>
                        <div class="sub">load {{ row.realtime.load1.toFixed(1) }}</div>
                    </template>
                    <span v-else>—</span>
                </template>
            </el-table-column>
            <el-table-column :label="$t('compute.host.fabric')" min-width="170">
                <template #default="{ row }">
                    <template v-if="activeRdma(row).length">
                        <div v-for="r in activeRdma(row)" :key="r.dev" class="sub">
                            <el-tag size="small" :type="r.errDelta ? 'danger' : 'success'" effect="plain">
                                {{ r.rateGbps }}G
                            </el-tag>
                            ↓{{ rate(r.rxRate) }} ↑{{ rate(r.txRate) }}
                        </div>
                    </template>
                    <span v-else class="sub">—</span>
                    <div class="sub" v-if="row.domain">{{ row.domain }}</div>
                </template>
            </el-table-column>
            <el-table-column :label="$t('compute.host.workload')" min-width="170">
                <template #default="{ row }">
                    <div v-for="n in row.instanceNames || []" :key="n">
                        <el-tag size="small">{{ n }}</el-tag>
                    </div>
                    <div v-if="unregistered(row).length" class="sub">
                        <el-tag size="small" type="warning" effect="plain">
                            {{ $t('compute.host.unregistered', [unregistered(row).length]) }}
                        </el-tag>
                    </div>
                    <span v-if="!row.instanceNames?.length && !unregistered(row).length" class="sub">
                        {{ $t('compute.host.idle') }}
                    </span>
                </template>
            </el-table-column>
            <el-table-column :label="$t('commons.table.operate')" width="200" fixed="right">
                <template #default="{ row }">
                    <el-button link type="primary" @click="onRefresh(row)">{{ $t('compute.host.collect') }}</el-button>
                    <el-button link type="primary" @click="hostRef.open(row)">
                        {{ $t('commons.button.edit') }}
                    </el-button>
                    <el-dropdown trigger="click" class="ml-2">
                        <el-button link type="primary">{{ $t('compute.more') }}</el-button>
                        <template #dropdown>
                            <el-dropdown-menu>
                                <el-dropdown-item @click="onCordon(row)">
                                    {{ row.schedulable ? $t('compute.host.cordon') : $t('compute.host.uncordon') }}
                                </el-dropdown-item>
                                <el-dropdown-item divided @click="onDelete(row)">
                                    <span class="stat-danger">{{ $t('commons.button.delete') }}</span>
                                </el-dropdown-item>
                            </el-dropdown-menu>
                        </template>
                    </el-dropdown>
                </template>
            </el-table-column>
        </el-table>

        <HostOperate ref="hostRef" @saved="refresh(true)" />
        <HostBatch ref="batchRef" @saved="refresh(true)" />
        <AdoptDialog ref="adoptRef" @saved="refresh(true)" />
        <DrawerPro v-model="topoVisible" :header="$t('compute.topo.title')" size="large" @close="topoVisible = false">
            <Topology v-if="topoVisible" :hosts="hosts" />
        </DrawerPro>
    </div>
</template>

<script lang="ts" setup>
import { computed, ref } from 'vue';
import { ElMessageBox } from 'element-plus';
import { Compute } from '@/api/interface/compute';
import { deleteComputeHost, listComputeHosts, refreshComputeHost, setComputeSchedulable } from '@/api/modules/compute';
import { MsgSuccess } from '@/utils/message';
import { ago, barColor, pct, rate, size, t, tempColor } from '../helper';
import HostOperate from './operate.vue';
import HostBatch from './batch.vue';
import HostDetail from './detail.vue';
import AdoptDialog from './adopt.vue';
import Topology from './topology.vue';

const loading = ref(false);
const hosts = ref<Compute.Host[]>([]);
const keyword = ref('');
const hostRef = ref();
const batchRef = ref();
const adoptRef = ref();
const topoVisible = ref(false);

const rows = computed(() => {
    const kw = keyword.value.trim().toLowerCase();
    if (!kw) return hosts.value;
    return hosts.value.filter((h) =>
        [h.name, h.addr, h.hostname, h.labels, h.domain].join(' ').toLowerCase().includes(kw),
    );
});

const onlineOf = (h: Compute.Host) => (h.realtime.sampledAt ? h.realtime.online : h.status === 'online');
const errorOf = (h: Compute.Host) => (!onlineOf(h) ? h.realtime.error || h.lastError : '');
const maxGpu = (h: Compute.Host) =>
    (h.realtime.gpus || []).reduce((a, b) => (b.util > a.util ? b : a), h.realtime.gpus![0]);
const shortGpu = (name: string) => name.replace(/^NVIDIA\s+/i, '');
const activeRdma = (h: Compute.Host) => (h.realtime.rdma || []).filter((r) => r.state === 'ACTIVE' || r.errDelta);
const unregistered = (h: Compute.Host) => (h.realtime.processes || []).filter((p) => !p.instanceId);

const refresh = async (force = false) => {
    if (force && !hosts.value.length) loading.value = true;
    try {
        const res = await listComputeHosts();
        hosts.value = res.data || [];
    } finally {
        loading.value = false;
    }
};

const onRefresh = async (row: Compute.Host) => {
    await refreshComputeHost(row.id);
    MsgSuccess(t('compute.host.collectQueued'));
    setTimeout(() => refresh(), 3000);
};

const onCordon = async (row: Compute.Host) => {
    if (row.schedulable) {
        await ElMessageBox.confirm(t('compute.host.cordonHelper'), t('compute.host.cordon'), { type: 'warning' });
    }
    await setComputeSchedulable(row.id, !row.schedulable);
    MsgSuccess(t('commons.msg.operationSuccess'));
    refresh();
};

const onDelete = async (row: Compute.Host) => {
    await ElMessageBox.confirm(t('compute.host.deleteHelper', [row.name]), t('commons.button.delete'), {
        type: 'warning',
    });
    await deleteComputeHost(row.id);
    MsgSuccess(t('commons.msg.deleteSuccess'));
    refresh();
};

const onAdopt = (host: Compute.Host, proc: Compute.Process) => adoptRef.value.open(host, proc, hosts.value);

defineExpose({ refresh });
</script>

<style scoped src="../style.css"></style>
<style scoped>
.mr-1 {
    margin-right: 4px;
}
</style>

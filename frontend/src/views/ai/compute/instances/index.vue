<template>
    <div v-loading="loading">
        <div class="toolbar">
            <el-button type="primary" @click="deployRef.open('deploy', undefined, servedName)">
                {{ servedName ? $t('compute.pool.addReplica') : $t('compute.inst.deploy') }}
            </el-button>
            <el-button @click="registerRef.open(undefined, servedName)">{{ $t('compute.inst.register') }}</el-button>
            <span class="spacer" />
            <el-select
                v-if="!servedName"
                v-model="modelFilter"
                clearable
                class="p-w-200"
                :placeholder="$t('compute.servedName')"
            >
                <el-option v-for="m in models" :key="m" :label="m" :value="m" />
            </el-select>
        </div>
        <el-table :data="rows" row-key="id" :empty-text="$t('compute.inst.empty')">
            <el-table-column :label="$t('compute.inst.name')" min-width="190" fixed="left">
                <template #default="{ row }">
                    <div class="strong">{{ row.name }}</div>
                    <div class="sub">{{ row.servedName }}</div>
                    <div class="sub">
                        <el-tag v-if="row.adopted" size="small" type="info" effect="plain">
                            {{ $t('compute.inst.adopted') }}
                        </el-tag>
                        <span v-else-if="row.profileName">{{ row.profileName }}</span>
                    </div>
                </template>
            </el-table-column>
            <el-table-column :label="$t('commons.table.status')" width="120">
                <template #default="{ row }">
                    <el-tooltip :disabled="!errOf(row)" :content="errOf(row)" placement="top">
                        <el-tag size="small" :type="instStatusType(row.status)">
                            {{ $t('compute.inst.status_' + row.status) }}
                        </el-tag>
                    </el-tooltip>
                    <div class="sub" v-if="!row.routeEnabled">
                        <el-tag size="small" type="warning" effect="plain">{{ $t('compute.inst.unrouted') }}</el-tag>
                    </div>
                </template>
            </el-table-column>
            <el-table-column :label="$t('compute.inst.hosts')" min-width="150">
                <template #default="{ row }">
                    <div v-for="(h, i) in row.hostNames || []" :key="h" class="sub">
                        {{ h }}
                        <span v-if="i === 0 && (row.hostNames || []).length > 1">(head)</span>
                    </div>
                    <div class="sub mono">:{{ row.port }}</div>
                </template>
            </el-table-column>
            <el-table-column :label="$t('compute.metric.load')" min-width="130">
                <template #default="{ row }">
                    <template v-if="row.metrics.sampledAt && row.metrics.healthy">
                        <div>
                            {{ $t('compute.metric.running') }} {{ row.metrics.running }} ·
                            <span :class="row.metrics.waiting ? 'stat-warning' : ''">
                                {{ $t('compute.metric.waiting') }} {{ row.metrics.waiting }}
                            </span>
                        </div>
                        <div class="sub">{{ $t('compute.metric.inflight') }} {{ row.metrics.inflight }}</div>
                    </template>
                    <span v-else>—</span>
                </template>
            </el-table-column>
            <el-table-column :label="$t('compute.metric.kv')" min-width="130">
                <template #default="{ row }">
                    <el-progress
                        v-if="row.metrics.healthy"
                        :percentage="pct(row.metrics.kvUsage * 100)"
                        :color="barColor(row.metrics.kvUsage * 100)"
                        :stroke-width="8"
                    />
                    <span v-else>—</span>
                </template>
            </el-table-column>
            <el-table-column :label="$t('compute.metric.latency')" width="140">
                <template #default="{ row }">
                    <template v-if="row.metrics.healthy">
                        <div>TTFT {{ ms(row.metrics.ttftMs) }}</div>
                        <div class="sub">{{ $t('compute.metric.queue') }} {{ ms(row.metrics.queueMs) }}</div>
                    </template>
                    <span v-else>—</span>
                </template>
            </el-table-column>
            <el-table-column :label="$t('compute.metric.throughput')" width="140">
                <template #default="{ row }">
                    <template v-if="row.metrics.healthy">
                        <div>{{ num(row.metrics.genTps) }} tok/s</div>
                        <div class="sub">prefill {{ num(row.metrics.promptTps) }}</div>
                    </template>
                    <span v-else>—</span>
                </template>
            </el-table-column>
            <el-table-column :label="$t('compute.inst.weight')" width="100">
                <template #default="{ row }">
                    <el-input-number
                        v-model="row.weight"
                        :min="0"
                        :max="1000"
                        :step="10"
                        size="small"
                        controls-position="right"
                        class="w-full"
                        @change="onWeight(row)"
                    />
                </template>
            </el-table-column>
            <el-table-column :label="$t('commons.table.operate')" width="220" fixed="right">
                <template #default="{ row }">
                    <el-button link type="primary" @click="deployRef.open('switch', row)">
                        {{ $t('compute.inst.switch') }}
                    </el-button>
                    <el-button link type="primary" @click="onOp(row, row.routeEnabled ? 'drain' : 'undrain')">
                        {{ row.routeEnabled ? $t('compute.inst.drain') : $t('compute.inst.undrain') }}
                    </el-button>
                    <el-dropdown trigger="click" class="ml-2">
                        <el-button link type="primary">{{ $t('compute.more') }}</el-button>
                        <template #dropdown>
                            <el-dropdown-menu>
                                <el-dropdown-item @click="registerRef.open(row)">
                                    {{ $t('commons.button.edit') }}
                                </el-dropdown-item>
                                <el-dropdown-item :disabled="row.status === 'stopped'" @click="onOp(row, 'stop')">
                                    {{ $t('commons.button.stop') }}
                                </el-dropdown-item>
                                <el-dropdown-item :disabled="!row.profileId" @click="onOp(row, 'start')">
                                    {{ $t('commons.button.start') }}
                                </el-dropdown-item>
                                <el-dropdown-item :disabled="!row.profileId" @click="onOp(row, 'restart')">
                                    {{ $t('commons.button.restart') }}
                                </el-dropdown-item>
                                <el-dropdown-item divided @click="onOp(row, 'remove')">
                                    <span class="stat-danger">
                                        {{ row.adopted ? $t('compute.inst.unregister') : $t('commons.button.delete') }}
                                    </span>
                                </el-dropdown-item>
                            </el-dropdown-menu>
                        </template>
                    </el-dropdown>
                </template>
            </el-table-column>
        </el-table>
        <DeployDrawer ref="deployRef" @started="refresh(true)" />
        <RegisterDrawer ref="registerRef" @saved="refresh(true)" />
    </div>
</template>

<script lang="ts" setup>
import { computed, ref } from 'vue';
import { ElMessageBox } from 'element-plus';
import { Compute } from '@/api/interface/compute';
import { listComputeInstances, operateComputeInstance, setComputeRoute } from '@/api/modules/compute';
import { MsgSuccess } from '@/utils/message';
import { barColor, instStatusType, ms, num, pct, t } from '../helper';
import DeployDrawer from './deploy.vue';
import RegisterDrawer from './register.vue';

// servedName scopes the list to one model (the instances of a pool).
const props = defineProps({ servedName: { type: String, default: '' } });
const loading = ref(false);
const list = ref<Compute.Instance[]>([]);
const modelFilter = ref(props.servedName);
const deployRef = ref();
const registerRef = ref();
// Edits in the weight box must not be overwritten by the poll.
let editingUntil = 0;

const models = computed(() => [...new Set(list.value.map((i) => i.servedName))].sort());
const rows = computed(() =>
    modelFilter.value ? list.value.filter((i) => i.servedName === modelFilter.value) : list.value,
);
const errOf = (row: Compute.Instance) =>
    row.lastError || (row.metrics.sampledAt && !row.metrics.healthy ? row.metrics.error : '');

const refresh = async (force = false) => {
    if (!force && Date.now() < editingUntil) return;
    if (force && !list.value.length) loading.value = true;
    try {
        const res = await listComputeInstances();
        list.value = res.data || [];
    } finally {
        loading.value = false;
    }
};

const onWeight = async (row: Compute.Instance) => {
    editingUntil = Date.now() + 3000;
    await setComputeRoute(row.id, row.routeEnabled, row.weight || 0);
};

const opNeedsConfirm = ['stop', 'restart', 'remove', 'drain'];
const onOp = async (row: Compute.Instance, op: string) => {
    let force = false;
    if (opNeedsConfirm.includes(op)) {
        let msg = t('compute.inst.opConfirm_' + op, [row.name]);
        if (row.adopted && op !== 'drain' && op !== 'remove') {
            msg += '\n' + t('compute.inst.adoptedForce');
            force = true;
        }
        await ElMessageBox.confirm(msg, t('compute.inst.op_' + op), { type: 'warning' });
    }
    const res = await operateComputeInstance(row.id, op, force);
    if (res.data?.jobId) MsgSuccess(t('compute.job.started', [res.data.jobId]));
    else MsgSuccess(t('commons.msg.operationSuccess'));
    refresh(true);
};

defineExpose({ refresh });
</script>

<style scoped src="../style.css"></style>
<style scoped>
.w-full {
    width: 100%;
}
</style>

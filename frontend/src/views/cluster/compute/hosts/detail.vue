<template>
    <div class="detail">
        <el-row :gutter="16">
            <el-col :span="8">
                <div class="sec-title">{{ $t('compute.host.asset') }}</div>
                <div class="kv">
                    <span class="k">{{ $t('compute.host.os') }}</span>
                    <span>{{ host.os || '—' }} · {{ host.arch || '—' }}</span>
                    <span class="k">{{ $t('compute.host.driver') }}</span>
                    <span>{{ host.driver || '—' }}</span>
                    <span class="k">GPU</span>
                    <span>{{ host.gpuModel || '—' }} × {{ host.gpuCount }}</span>
                    <span class="k">CPU</span>
                    <span>{{ host.cpuCores }} {{ $t('compute.host.cores') }}</span>
                    <span class="k">{{ $t('compute.host.disk') }}</span>
                    <span v-if="rt.diskTotal">{{ size(rt.diskUsed) }} / {{ size(rt.diskTotal) }}</span>
                    <span v-else>—</span>
                    <span class="k">{{ $t('compute.host.access') }}</span>
                    <span>
                        {{ host.sshUser }}@{{ host.addr }}:{{ host.sshPort }} ·
                        {{ $t('compute.host.mode_' + host.mode) }}
                    </span>
                    <span class="k">{{ $t('compute.host.collectCost') }}</span>
                    <span>{{ rt.collectMs ? rt.collectMs + ' ms' : '—' }}</span>
                </div>
                <div v-if="rt.warnings?.length" class="mt-2">
                    <el-tag v-for="w in rt.warnings" :key="w" type="warning" size="small" class="mr-1">
                        {{ warningText(w) }}
                    </el-tag>
                </div>
            </el-col>
            <el-col :span="8">
                <div class="sec-title">{{ $t('compute.host.rdma') }}</div>
                <div v-if="!rt.rdma?.length" class="sub">{{ $t('compute.host.noRdma') }}</div>
                <div v-for="r in rt.rdma || []" :key="r.dev + r.port" class="rdma">
                    <div class="line">
                        <span>
                            <el-tag size="small" :type="r.state === 'ACTIVE' ? 'success' : 'info'" effect="plain">
                                {{ r.state }}
                            </el-tag>
                            {{ r.dev }}
                            <span class="sub">({{ r.netDev || '-' }})</span>
                        </span>
                        <span>{{ r.rateGbps }} Gb/s</span>
                    </div>
                    <div class="line sub" v-if="r.state === 'ACTIVE'">
                        <span>↓ {{ rate(r.rxRate) }} · ↑ {{ rate(r.txRate) }}</span>
                        <span :class="r.errDelta ? 'stat-danger' : ''">
                            err {{ r.rcvErrors + r.symbolErrors }} · down {{ r.linkDowned }}
                            <span v-if="r.errDelta">(+{{ r.errDelta }})</span>
                        </span>
                    </div>
                </div>
            </el-col>
            <el-col :span="8">
                <div class="sec-title">{{ $t('compute.host.engines') }}</div>
                <div v-if="!rt.processes?.length" class="sub">{{ $t('compute.host.noEngine') }}</div>
                <div v-for="p in rt.processes || []" :key="p.pid" class="proc">
                    <div class="line">
                        <span>
                            <el-tag size="small" effect="plain">{{ p.engine }}</el-tag>
                            :{{ p.port }}
                            <span class="strong">{{ p.servedName }}</span>
                        </span>
                        <el-tag v-if="p.instanceId" size="small" type="success">
                            {{ $t('compute.host.registered') }}
                        </el-tag>
                        <el-button v-else size="small" type="primary" link @click="emit('adopt', host, p)">
                            {{ $t('compute.inst.adopt') }}
                        </el-button>
                    </div>
                    <el-tooltip :content="p.cmd" placement="top" :show-after="300">
                        <div class="sub ellipsis mono">pid {{ p.pid }} · {{ p.cmd }}</div>
                    </el-tooltip>
                </div>
            </el-col>
        </el-row>
    </div>
</template>

<script lang="ts" setup>
import { computed } from 'vue';
import { Compute } from '@/api/interface/compute';
import { rate, size, warningText } from '../helper';

const props = defineProps<{ host: Compute.Host }>();
const emit = defineEmits(['adopt']);
const rt = computed(() => props.host.realtime);
</script>

<style scoped src="../style.css"></style>
<style scoped>
.detail {
    padding: 4px 48px 12px;
}
.sec-title {
    font-weight: 600;
    margin-bottom: 8px;
}
.rdma,
.proc {
    padding: 4px 0;
    border-bottom: 1px dashed var(--el-border-color-lighter);
}
.ellipsis {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
}
.mr-1 {
    margin-right: 4px;
}
</style>

<template>
    <div v-loading="loading">
        <el-card shadow="never" class="mb-3">
            <template #header>
                <div class="card-head">
                    <span>
                        <span class="strong">{{ $t('compute.tpl.profiles') }}</span>
                        <span class="sub ml-2">{{ $t('compute.tpl.profilesHelper') }}</span>
                    </span>
                    <el-button type="primary" :disabled="!runtimes.length" @click="openProfile()">
                        {{ $t('compute.tpl.addProfile') }}
                    </el-button>
                </div>
            </template>
            <el-table :data="profiles" :empty-text="$t('compute.tpl.noProfile')">
                <el-table-column :label="$t('commons.table.name')" prop="name" min-width="150" />
                <el-table-column :label="$t('compute.servedName')" prop="servedName" min-width="170" />
                <el-table-column :label="$t('compute.tpl.runtime')" min-width="150">
                    <template #default="{ row }">{{ runtimeName(row.runtimeId) }}</template>
                </el-table-column>
                <el-table-column :label="$t('compute.wiz.model')" min-width="220">
                    <template #default="{ row }">
                        <div v-if="row.modelRepo">{{ row.modelRepo }}</div>
                        <span class="mono sub">{{ row.modelPath }}</span>
                    </template>
                </el-table-column>
                <el-table-column :label="$t('compute.tpl.resources')" min-width="200">
                    <template #default="{ row }">
                        <template v-if="row.nodeCount > 1">
                            {{ $t('compute.wiz.multiN', [row.nodeCount]) }}
                            <el-tag v-if="row.needFabric" size="small" effect="plain">fabric</el-tag>
                        </template>
                        <template v-else>
                            {{ $t('compute.wiz.singleN', [row.maxPerHost || 1]) }}
                        </template>
                        <div class="sub">
                            {{ hostNames(row) }}
                        </div>
                        <div class="sub">
                            :{{ row.port }}+
                            <span v-if="row.minMemGb">· {{ row.minMemGb }} GiB / {{ $t('compute.wiz.replica') }}</span>
                        </div>
                    </template>
                </el-table-column>
                <el-table-column :label="$t('commons.table.operate')" width="150" fixed="right">
                    <template #default="{ row }">
                        <el-button link type="primary" @click="openProfile(row)">
                            {{ $t('commons.button.edit') }}
                        </el-button>
                        <el-button link type="danger" @click="onDelProfile(row)">
                            {{ $t('commons.button.delete') }}
                        </el-button>
                    </template>
                </el-table-column>
            </el-table>
        </el-card>

        <el-card shadow="never">
            <template #header>
                <div class="card-head">
                    <span>
                        <span class="strong">{{ $t('compute.tpl.runtimes') }}</span>
                        <span class="sub ml-2">{{ $t('compute.tpl.runtimesHelper') }}</span>
                    </span>
                    <el-button @click="openRuntime()">{{ $t('compute.tpl.addRuntime') }}</el-button>
                </div>
            </template>
            <el-table :data="runtimes">
                <el-table-column :label="$t('commons.table.name')" min-width="170">
                    <template #default="{ row }">
                        {{ row.name }}
                        <el-tag v-if="row.builtin" size="small" type="info" effect="plain">
                            {{ $t('compute.tpl.builtin') }}
                        </el-tag>
                    </template>
                </el-table-column>
                <el-table-column :label="$t('compute.tpl.engine')" prop="engine" width="100" />
                <el-table-column :label="$t('compute.tpl.backend')" width="130">
                    <template #default="{ row }">
                        <el-tag size="small" effect="plain" :type="row.backend === 'vllm' ? 'success' : 'info'">
                            {{ $t('compute.tpl.backend_' + (row.backend || 'script')) }}
                        </el-tag>
                    </template>
                </el-table-column>
                <el-table-column :label="$t('compute.tpl.device')" width="90">
                    <template #default="{ row }">
                        <el-tag size="small" effect="plain" :type="row.device === 'npu' ? 'warning' : 'primary'">
                            {{ (row.device || 'gpu').toUpperCase() }}
                        </el-tag>
                    </template>
                </el-table-column>
                <el-table-column :label="$t('compute.tpl.image')" min-width="220">
                    <template #default="{ row }">
                        <span class="mono">{{ row.image }}</span>
                    </template>
                </el-table-column>
                <el-table-column :label="$t('compute.tpl.multiNode')" width="100">
                    <template #default="{ row }">{{ row.multiNode ? '✓' : '—' }}</template>
                </el-table-column>
                <el-table-column :label="$t('commons.table.description')" prop="remark" min-width="200" />
                <el-table-column :label="$t('commons.table.operate')" width="150" fixed="right">
                    <template #default="{ row }">
                        <el-button link type="primary" @click="openRuntime(row)">
                            {{ $t('commons.button.edit') }}
                        </el-button>
                        <el-button link type="danger" @click="onDelRuntime(row)">
                            {{ $t('commons.button.delete') }}
                        </el-button>
                    </template>
                </el-table-column>
            </el-table>
        </el-card>

        <ProfileWizard ref="wizardRef" :runtimes="runtimes" @saved="load" />

        <DrawerPro
            v-model="runtimeVisible"
            :header="$t('compute.tpl.runtime')"
            :resource="runtimeForm.name"
            size="large"
            @close="runtimeVisible = false"
        >
            <el-alert
                v-if="runtimeForm.builtin"
                type="warning"
                :closable="false"
                show-icon
                class="mb-3"
                :title="$t('compute.tpl.builtinHelper')"
            />
            <el-form label-position="top" :model="runtimeForm">
                <el-row :gutter="12">
                    <el-col :span="9">
                        <el-form-item :label="$t('commons.table.name')" required>
                            <el-input v-model="runtimeForm.name" />
                        </el-form-item>
                    </el-col>
                    <el-col :span="4">
                        <el-form-item :label="$t('compute.tpl.device')">
                            <el-select v-model="runtimeForm.device">
                                <el-option value="gpu" label="GPU" />
                                <el-option value="npu" label="NPU" />
                            </el-select>
                        </el-form-item>
                    </el-col>
                    <el-col :span="5">
                        <el-form-item :label="$t('compute.tpl.engine')">
                            <el-select v-model="runtimeForm.engine">
                                <el-option value="vllm" label="vLLM" />
                                <el-option value="sglang" label="SGLang" />
                                <el-option value="other" label="Other" />
                            </el-select>
                        </el-form-item>
                    </el-col>
                    <el-col :span="6">
                        <el-form-item :label="$t('compute.tpl.multiNode')">
                            <el-switch v-model="runtimeForm.multiNode" :disabled="isVllmBackend" />
                        </el-form-item>
                    </el-col>
                </el-row>
                <el-form-item :label="$t('compute.tpl.backend')">
                    <el-radio-group v-model="runtimeForm.backend">
                        <el-radio value="script">{{ $t('compute.tpl.backend_script') }}</el-radio>
                        <el-radio value="vllm">{{ $t('compute.tpl.backend_vllm') }}</el-radio>
                    </el-radio-group>
                    <span class="input-help">
                        {{ $t('compute.tpl.backendHelper_' + (runtimeForm.backend || 'script')) }}
                    </span>
                </el-form-item>
                <el-form-item :label="$t('compute.tpl.image')">
                    <el-input v-model="runtimeForm.image" />
                </el-form-item>
                <el-row :gutter="12">
                    <el-col :span="12">
                        <el-form-item :label="$t('compute.tpl.healthPath')">
                            <el-input v-model="runtimeForm.healthPath" />
                        </el-form-item>
                    </el-col>
                    <el-col :span="12">
                        <el-form-item :label="$t('compute.tpl.metricsPath')">
                            <el-input v-model="runtimeForm.metricsPath" />
                        </el-form-item>
                    </el-col>
                </el-row>
                <template v-if="!isVllmBackend">
                    <el-form-item :label="$t('compute.tpl.launch')" required>
                        <el-input v-model="runtimeForm.launchTemplate" type="textarea" :rows="14" class="code-area" />
                        <span class="input-help">{{ $t('compute.tpl.varsHelper') }}</span>
                    </el-form-item>
                    <el-form-item :label="$t('compute.tpl.stop')" required>
                        <el-input v-model="runtimeForm.stopTemplate" type="textarea" :rows="3" class="code-area" />
                    </el-form-item>
                </template>
                <el-form-item :label="$t('commons.table.description')">
                    <el-input v-model="runtimeForm.remark" />
                </el-form-item>
            </el-form>
            <template #footer>
                <el-button @click="runtimeVisible = false">{{ $t('commons.button.cancel') }}</el-button>
                <el-button type="primary" @click="onSaveRuntime">{{ $t('commons.button.confirm') }}</el-button>
            </template>
        </DrawerPro>
    </div>
</template>

<script lang="ts" setup>
import { computed, onMounted, reactive, ref } from 'vue';
import { ElMessageBox } from 'element-plus';
import { Compute } from '@/api/interface/compute';
import {
    deleteComputeProfile,
    deleteComputeRuntime,
    listComputeHosts,
    listComputeProfiles,
    listComputeRuntimes,
    saveComputeRuntime,
} from '@/api/modules/compute';
import ProfileWizard from './profile.vue';
import { MsgSuccess } from '@/utils/message';
import { t } from '../helper';

const loading = ref(false);
const runtimes = ref<Compute.Runtime[]>([]);
const profiles = ref<Compute.Profile[]>([]);
const hosts = ref<Compute.Host[]>([]);
const runtimeVisible = ref(false);
const wizardRef = ref();
const emptyRuntime = (): Partial<Compute.Runtime> => ({
    id: 0,
    name: '',
    engine: 'vllm',
    device: 'gpu',
    backend: 'script',
    image: '',
    multiNode: false,
    launchTemplate: '',
    stopTemplate: 'docker rm -f {{q .ContainerName}} >/dev/null 2>&1 || true\n',
    healthPath: '/health',
    metricsPath: '/metrics',
    remark: '',
    builtin: false,
});
const runtimeForm = reactive<Partial<Compute.Runtime>>(emptyRuntime());
const isVllmBackend = computed(() => runtimeForm.backend === 'vllm');

const runtimeName = (id: number) => runtimes.value.find((r) => r.id === id)?.name || `#${id}`;
const hostNames = (row: Compute.Profile) => {
    const ids = (row.hostIds || '').split(',').filter(Boolean).map(Number);
    if (!ids.length) return t('compute.wiz.anyHost');
    return ids
        .map((id, i) => {
            const name = hosts.value.find((h) => h.id === id)?.name || `#${id}`;
            return i === 0 && row.nodeCount > 1 ? `${name} (${t('compute.wiz.head')})` : name;
        })
        .join(', ');
};

const load = async () => {
    loading.value = true;
    try {
        const [r, p, h] = await Promise.all([listComputeRuntimes(), listComputeProfiles(), listComputeHosts()]);
        runtimes.value = r.data || [];
        profiles.value = p.data || [];
        hosts.value = h.data || [];
    } finally {
        loading.value = false;
    }
};

const openProfile = (row?: Compute.Profile) => wizardRef.value.open(row);
const openRuntime = (row?: Compute.Runtime) => {
    Object.assign(runtimeForm, emptyRuntime(), row || {});
    runtimeVisible.value = true;
};

const onSaveRuntime = async () => {
    await saveComputeRuntime({ ...runtimeForm });
    MsgSuccess(t('commons.msg.operationSuccess'));
    runtimeVisible.value = false;
    load();
};
const onDelProfile = async (row: Compute.Profile) => {
    await ElMessageBox.confirm(t('compute.tpl.deleteConfirm', [row.name]), t('commons.button.delete'), {
        type: 'warning',
    });
    await deleteComputeProfile(row.id);
    load();
};
const onDelRuntime = async (row: Compute.Runtime) => {
    await ElMessageBox.confirm(t('compute.tpl.deleteConfirm', [row.name]), t('commons.button.delete'), {
        type: 'warning',
    });
    await deleteComputeRuntime(row.id);
    load();
};

// Configuration tab: loaded once, not polled.
onMounted(load);
</script>

<style scoped src="../style.css"></style>
<style scoped>
.w-full {
    width: 100%;
}
</style>

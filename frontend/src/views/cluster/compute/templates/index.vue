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
                <el-table-column :label="$t('compute.tpl.modelPath')" min-width="200">
                    <template #default="{ row }">
                        <span class="mono">{{ row.modelPath }}</span>
                    </template>
                </el-table-column>
                <el-table-column :label="$t('compute.tpl.resources')" min-width="160">
                    <template #default="{ row }">
                        {{ row.nodeCount }}× {{ $t('compute.tpl.hosts') }}
                        <el-tag v-if="row.needFabric" size="small" effect="plain">fabric</el-tag>
                        <div class="sub">
                            :{{ row.port }}
                            <span v-if="row.minMemGb">· ≥{{ row.minMemGb }} GiB</span>
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

        <DrawerPro
            v-model="profileVisible"
            :header="$t('compute.tpl.profile')"
            :resource="profileForm.name"
            size="normal"
            @close="profileVisible = false"
        >
            <el-form label-position="top" :model="profileForm">
                <el-row :gutter="12">
                    <el-col :span="12">
                        <el-form-item :label="$t('commons.table.name')" required>
                            <el-input v-model="profileForm.name" placeholder="deepseek-v4-2x" />
                        </el-form-item>
                    </el-col>
                    <el-col :span="12">
                        <el-form-item :label="$t('compute.servedName')" required>
                            <el-input v-model="profileForm.servedName" placeholder="deepseek-v4-flash-0731" />
                        </el-form-item>
                    </el-col>
                </el-row>
                <el-form-item :label="$t('compute.tpl.runtime')" required>
                    <el-select v-model="profileForm.runtimeId" class="w-full">
                        <el-option v-for="r in runtimes" :key="r.id" :label="r.name" :value="r.id" />
                    </el-select>
                </el-form-item>
                <el-form-item :label="$t('compute.tpl.modelPath')" required>
                    <el-input v-model="profileForm.modelPath" placeholder="/models/DeepSeek-V4-Flash-0731" />
                    <span class="input-help">{{ $t('compute.tpl.modelPathHelper') }}</span>
                </el-form-item>
                <el-row :gutter="12">
                    <el-col :span="8">
                        <el-form-item :label="$t('compute.tpl.nodeCount')">
                            <el-input-number
                                v-model="profileForm.nodeCount"
                                :min="1"
                                :max="64"
                                controls-position="right"
                            />
                        </el-form-item>
                    </el-col>
                    <el-col :span="8">
                        <el-form-item :label="$t('compute.host.port')">
                            <el-input-number
                                v-model="profileForm.port"
                                :min="1"
                                :max="65535"
                                controls-position="right"
                            />
                        </el-form-item>
                    </el-col>
                    <el-col :span="8">
                        <el-form-item :label="$t('compute.tpl.minMem')">
                            <el-input-number
                                v-model="profileForm.minMemGb"
                                :min="0"
                                :max="2048"
                                controls-position="right"
                            />
                        </el-form-item>
                    </el-col>
                </el-row>
                <el-form-item>
                    <el-checkbox v-model="profileForm.needFabric">{{ $t('compute.tpl.needFabric') }}</el-checkbox>
                    <span class="input-help">{{ $t('compute.tpl.needFabricHelper') }}</span>
                </el-form-item>
                <el-form-item :label="$t('compute.tpl.extraArgs')">
                    <el-input
                        v-model="profileForm.extraArgs"
                        type="textarea"
                        :rows="3"
                        class="code-area"
                        placeholder="--max-model-len 200000 --gpu-memory-utilization 0.8 --kv-cache-dtype fp8"
                    />
                </el-form-item>
                <el-form-item :label="$t('compute.tpl.env')">
                    <el-input
                        v-model="profileForm.env"
                        type="textarea"
                        :rows="3"
                        class="code-area"
                        placeholder="KEY=VALUE"
                    />
                </el-form-item>
                <el-form-item :label="$t('compute.tpl.startTimeout')">
                    <el-input-number
                        v-model="profileForm.startTimeoutSec"
                        :min="60"
                        :max="7200"
                        :step="60"
                        controls-position="right"
                    />
                </el-form-item>
            </el-form>
            <template #footer>
                <el-button @click="profileVisible = false">{{ $t('commons.button.cancel') }}</el-button>
                <el-button type="primary" @click="onSaveProfile">{{ $t('commons.button.confirm') }}</el-button>
            </template>
        </DrawerPro>

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
                            <el-switch v-model="runtimeForm.multiNode" />
                        </el-form-item>
                    </el-col>
                </el-row>
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
                <el-form-item :label="$t('compute.tpl.launch')" required>
                    <el-input v-model="runtimeForm.launchTemplate" type="textarea" :rows="14" class="code-area" />
                    <span class="input-help">{{ $t('compute.tpl.varsHelper') }}</span>
                </el-form-item>
                <el-form-item :label="$t('compute.tpl.stop')" required>
                    <el-input v-model="runtimeForm.stopTemplate" type="textarea" :rows="3" class="code-area" />
                </el-form-item>
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
import { onMounted, reactive, ref } from 'vue';
import { ElMessageBox } from 'element-plus';
import { Compute } from '@/api/interface/compute';
import {
    deleteComputeProfile,
    deleteComputeRuntime,
    listComputeProfiles,
    listComputeRuntimes,
    saveComputeProfile,
    saveComputeRuntime,
} from '@/api/modules/compute';
import { MsgSuccess } from '@/utils/message';
import { t } from '../helper';

const loading = ref(false);
const runtimes = ref<Compute.Runtime[]>([]);
const profiles = ref<Compute.Profile[]>([]);
const profileVisible = ref(false);
const runtimeVisible = ref(false);

const emptyProfile = (): Partial<Compute.Profile> => ({
    id: 0,
    name: '',
    runtimeId: undefined,
    servedName: '',
    modelPath: '',
    nodeCount: 1,
    needFabric: false,
    minMemGb: 0,
    port: 8000,
    extraArgs: '',
    env: '',
    startTimeoutSec: 1800,
    remark: '',
});
const emptyRuntime = (): Partial<Compute.Runtime> => ({
    id: 0,
    name: '',
    engine: 'vllm',
    device: 'gpu',
    image: '',
    multiNode: false,
    launchTemplate: '',
    stopTemplate: 'docker rm -f {{q .ContainerName}} >/dev/null 2>&1 || true\n',
    healthPath: '/health',
    metricsPath: '/metrics',
    remark: '',
    builtin: false,
});
const profileForm = reactive<Partial<Compute.Profile>>(emptyProfile());
const runtimeForm = reactive<Partial<Compute.Runtime>>(emptyRuntime());

const runtimeName = (id: number) => runtimes.value.find((r) => r.id === id)?.name || `#${id}`;

const load = async () => {
    loading.value = true;
    try {
        const [r, p] = await Promise.all([listComputeRuntimes(), listComputeProfiles()]);
        runtimes.value = r.data || [];
        profiles.value = p.data || [];
    } finally {
        loading.value = false;
    }
};

const openProfile = (row?: Compute.Profile) => {
    Object.assign(profileForm, emptyProfile(), row || {});
    if (!row) profileForm.runtimeId = runtimes.value[0]?.id;
    profileVisible.value = true;
};
const openRuntime = (row?: Compute.Runtime) => {
    Object.assign(runtimeForm, emptyRuntime(), row || {});
    runtimeVisible.value = true;
};

const onSaveProfile = async () => {
    await saveComputeProfile({ ...profileForm });
    MsgSuccess(t('commons.msg.operationSuccess'));
    profileVisible.value = false;
    load();
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

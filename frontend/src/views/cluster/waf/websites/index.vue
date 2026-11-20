<template>
    <div>
        <LayoutContent :title="$t('menu.clusterWafWebsites')" v-loading="loading" back-path="/cluster/waf">
            <template #rightToolBar>
                <el-select class="p-w-200" v-model="req.nodeId" @change="search()" clearable>
                    <template #prefix>{{ $t('cluster.nodeName') }}</template>
                    <el-option :label="$t('commons.table.all')" :value="0"></el-option>
                    <el-option v-for="item in nodes" :key="item.id" :label="item.name" :value="item.id" />
                </el-select>
                <TableSearch @search="search()" v-model:searchName="req.info" />
                <TableRefresh @search="search()" />
            </template>
            <template #main>
                <el-alert
                    :title="$t('cluster.schemaPendingAlert')"
                    type="info"
                    show-icon
                    :closable="false"
                    class="mb-4"
                />
                <ComplexTable
                    :pagination-config="paginationConfig"
                    v-model:selects="selects"
                    :data="data"
                    @search="search()"
                    :columns="columns"
                    localKey="clusterWafWebsiteColumn"
                    row-key="domain"
                >
                    <el-table-column :label="$t('cluster.nodeName')" prop="nodeName" min-width="140px" fix />
                    <el-table-column
                        :label="$t('cluster.domain')"
                        prop="domain"
                        min-width="200px"
                        show-overflow-tooltip
                    />
                    <el-table-column :label="$t('cluster.type')" prop="type" width="110px" />
                    <el-table-column :label="$t('cluster.status')" prop="status" width="110px" align="center">
                        <template #default="{ row }">
                            <el-tag
                                :type="row.status === 'Running' ? 'success' : 'info'"
                                size="small"
                                v-if="row.status"
                            >
                                {{ row.status }}
                            </el-tag>
                            <span v-else>—</span>
                        </template>
                    </el-table-column>
                    <el-table-column :label="$t('cluster.protection')" width="130px" align="center">
                        <template #default="{ row }">
                            <el-switch :model-value="row.enabled === true" disabled />
                            <span class="ml-1" v-if="row.enabled !== true">{{ $t('cluster.disabled') }}</span>
                        </template>
                    </el-table-column>
                    <el-table-column :label="$t('cluster.mode')" width="110px" align="center">
                        <template #default="{ row }">
                            {{ row.mode || '—' }}
                        </template>
                    </el-table-column>
                    <el-table-column :label="$t('cluster.strength')" width="110px" align="center">
                        <template #default="{ row }">
                            {{ row.strength || '—' }}
                        </template>
                    </el-table-column>
                    <el-table-column :label="$t('cluster.rateLimit')" width="110px" align="center">
                        <template #default="{ row }">
                            {{ row.rateLimit === null || row.rateLimit === undefined ? '—' : row.rateLimit }}
                        </template>
                    </el-table-column>
                    <fu-table-operations
                        width="160px"
                        :buttons="buttons"
                        :label="$t('commons.table.operate')"
                        fix
                    />
                </ComplexTable>
            </template>
        </LayoutContent>
    </div>
</template>

<script lang="ts" setup>
import { onMounted, ref } from 'vue';
import i18n from '@/lang';
import { searchClusterNodes, searchClusterWafWebsites } from '@/api/modules/cluster';
import { Cluster } from '@/api/interface/cluster';
import { usePageState } from '@/composables/usePageState';

const loading = ref(false);
const data = ref<Cluster.WafWebsite[]>([]);
const selects = ref([]);
const columns = ref([]);
const nodes = ref<Cluster.NodeInfo[]>([]);

const pageState = usePageState(() => ({
    paginationConfig: {
        currentPage: 1,
        pageSize: 20,
        total: 0,
    },
    req: {
        page: 1,
        pageSize: 20,
        info: '',
        nodeId: 0,
    } as Cluster.WafWebsiteSearch & { info: string },
}));
const paginationConfig = pageState.paginationConfig;
const req = pageState.req;

const search = async () => {
    req.page = paginationConfig.currentPage;
    req.pageSize = paginationConfig.pageSize;
    loading.value = true;
    await searchClusterWafWebsites(req)
        .then((res) => {
            data.value = res.data.items;
            paginationConfig.total = res.data.total;
        })
        .finally(() => {
            loading.value = false;
        });
};

const loadNodes = async () => {
    try {
        const res = await searchClusterNodes({ page: 1, pageSize: 500 });
        nodes.value = res.data.items;
    } catch {
        // The request interceptor already reports the error.
    }
};

const buttons = [
    {
        label: i18n.global.t('cluster.jumpToRemote'),
        click: (row: Cluster.WafWebsite) => {
            if (row.jumpUrl) {
                window.open(row.jumpUrl, '_blank');
            }
        },
    },
];

onMounted(() => {
    loadNodes();
    search();
});
</script>

<style scoped>
.mb-4 {
    margin-bottom: 16px;
}
</style>

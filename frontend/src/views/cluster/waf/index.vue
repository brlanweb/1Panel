<template>
    <div>
        <LayoutContent :title="$t('menu.clusterWaf')" v-loading="loading">
            <template #rightToolBar>
                <TableRefresh @search="loadOverview()" />
            </template>
            <template #main>
                <el-alert
                    :title="$t('cluster.wafNotImplemented')"
                    type="info"
                    show-icon
                    :closable="false"
                    class="mb-4"
                />
                <el-row :gutter="12" class="mb-4">
                    <el-col :span="8">
                        <el-card shadow="hover">
                            <el-statistic :title="$t('cluster.totalNodes')" :value="overview.totalNodes" />
                        </el-card>
                    </el-col>
                    <el-col :span="8">
                        <el-card shadow="hover">
                            <el-statistic :title="$t('cluster.wafTotalWebsites')" :value="overview.totalWebsites" />
                        </el-card>
                    </el-col>
                    <el-col :span="8">
                        <el-card shadow="hover">
                            <el-button type="primary" @click="goWebsites">
                                {{ $t('cluster.goWebsiteSettings') }}
                            </el-button>
                        </el-card>
                    </el-col>
                </el-row>
                <el-card shadow="never">
                    <template #header>
                        <span>{{ $t('cluster.wafPerNode') }}</span>
                    </template>
                    <el-table :data="overview.nodes" style="width: 100%">
                        <el-table-column :label="$t('commons.table.name')" prop="name" min-width="160px" />
                        <el-table-column
                            :label="$t('cluster.websiteCount')"
                            prop="websiteCount"
                            width="140px"
                            align="center"
                        />
                        <el-table-column :label="$t('cluster.fetchError')" prop="fetchError" min-width="200px">
                            <template #default="{ row }">
                                <el-text v-if="row.fetchError" type="danger" size="small">
                                    {{ row.fetchError }}
                                </el-text>
                                <span v-else>—</span>
                            </template>
                        </el-table-column>
                    </el-table>
                </el-card>
            </template>
        </LayoutContent>
    </div>
</template>

<script lang="ts" setup>
import { onMounted, reactive, ref } from 'vue';
import { useRouter } from 'vue-router';
import { getClusterWafOverview } from '@/api/modules/cluster';
import { Cluster } from '@/api/interface/cluster';

const router = useRouter();
const loading = ref(false);
const overview = reactive<Cluster.WafOverview>({
    totalNodes: 0,
    totalWebsites: 0,
    nodes: [],
});

const goWebsites = () => {
    router.push('/cluster/waf/websites');
};

const loadOverview = async () => {
    loading.value = true;
    await getClusterWafOverview()
        .then((res) => {
            overview.totalNodes = res.data.totalNodes;
            overview.totalWebsites = res.data.totalWebsites;
            overview.nodes = res.data.nodes || [];
        })
        .finally(() => {
            loading.value = false;
        });
};

onMounted(() => {
    loadOverview();
});
</script>

<style scoped>
.mb-4 {
    margin-bottom: 16px;
}
</style>

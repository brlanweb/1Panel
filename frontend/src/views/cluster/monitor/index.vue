<template>
    <div>
        <LayoutContent :title="$t('menu.clusterMonitor')" v-loading="loading">
            <template #main>
                <el-card shadow="never" class="mb-4">
                    <el-form :inline="true">
                        <el-form-item :label="$t('cluster.nodeName')">
                            <el-select v-model="nodeName" class="p-w-200" @change="onNodeChange">
                                <el-option :label="masterAlias" value="local" />
                                <el-option
                                    v-for="item in onlineNodes"
                                    :key="item.id"
                                    :label="item.name"
                                    :value="item.name"
                                />
                            </el-select>
                        </el-form-item>
                        <el-form-item :label="$t('menu.website')">
                            <el-select
                                v-model="websiteId"
                                class="p-w-300"
                                :placeholder="$t('cluster.selectWebsite')"
                                @change="loadSite"
                            >
                                <el-option
                                    v-for="item in websites"
                                    :key="item.id"
                                    :label="item.primaryDomain"
                                    :value="item.id"
                                />
                            </el-select>
                        </el-form-item>
                        <el-form-item>
                            <el-button type="primary" :disabled="!websiteId" @click="loadSite">
                                {{ $t('commons.button.refresh') }}
                            </el-button>
                        </el-form-item>
                    </el-form>
                    <el-alert
                        v-if="websiteId && !logEnabled"
                        type="warning"
                        :closable="false"
                        show-icon
                        :title="$t('cluster.monitorNotEnabled')"
                    >
                        <template #default>
                            <el-switch v-model="logEnabledSwitch" @change="onToggleLog" />
                            <span class="ml-2">{{ $t('cluster.monitorEnableNow') }}</span>
                        </template>
                    </el-alert>
                    <el-text v-if="parsedLines" type="info" size="small">
                        {{ $t('cluster.monitorDataWindow', [parsedLines, totalLines]) }}
                    </el-text>
                </el-card>

                <el-tabs v-model="activeTab">
                    <el-tab-pane :label="$t('cluster.monitorOverview')" name="overview">
                        <el-row :gutter="12" class="mb-4">
                            <el-col :span="6" v-for="stat in todayStats" :key="stat.label">
                                <el-card shadow="hover">
                                    <el-statistic :title="stat.label" :value="stat.value" />
                                </el-card>
                            </el-col>
                        </el-row>
                        <el-card shadow="never" class="mb-4">
                            <template #header>
                                <div class="card-header-with-select">
                                    <span>{{ $t('cluster.visitorTrend') }}</span>
                                    <el-select v-model="trendRange" class="p-w-150" @change="buildTrend">
                                        <el-option :label="$t('cluster.today')" value="today" />
                                        <el-option :label="$t('cluster.last7Days')" value="7d" />
                                        <el-option :label="$t('cluster.last30Days')" value="30d" />
                                    </el-select>
                                </div>
                            </template>
                            <v-charts type="line" :option="trendOption" height="280px" />
                        </el-card>
                    </el-tab-pane>

                    <el-tab-pane :label="$t('cluster.monitorVisits')" name="visits">
                        <el-table :data="dailyRows" style="width: 100%" :default-sort="{ prop: 'date', order: 'descending' }">
                            <el-table-column :label="$t('commons.table.date')" prop="date" width="130px" sortable />
                            <el-table-column :label="$t('cluster.monitorPv')" prop="pv" sortable align="right" />
                            <el-table-column :label="$t('cluster.monitorUv')" prop="uv" sortable align="right" />
                            <el-table-column :label="$t('cluster.monitorIps')" prop="ips" sortable align="right" />
                            <el-table-column :label="$t('cluster.monitorTraffic')" prop="traffic" sortable align="right">
                                <template #default="{ row }">{{ formatBytes(row.trafficBytes) }}</template>
                            </el-table-column>
                            <el-table-column :label="$t('cluster.monitorSpiders')" prop="spiders" sortable align="right" />
                            <el-table-column :label="$t('cluster.monitorReqs')" prop="reqs" sortable align="right" />
                            <el-table-column :label="$t('cluster.monitor4xx')" prop="e4xx" sortable align="right" />
                            <el-table-column :label="$t('cluster.monitor5xx')" prop="e5xx" sortable align="right" />
                        </el-table>
                        <el-empty v-if="!dailyRows.length" :description="$t('commons.msg.noneData')" />
                    </el-tab-pane>

                    <el-tab-pane :label="$t('cluster.monitorTrends')" name="trends">
                        <el-table :data="hourlyRows" style="width: 100%">
                            <el-table-column :label="$t('commons.table.date')" prop="label" width="160px" />
                            <el-table-column :label="$t('cluster.monitorPv')" prop="pv" align="right" />
                            <el-table-column :label="$t('cluster.monitorUv')" prop="uv" align="right" />
                            <el-table-column :label="$t('cluster.monitorTraffic')" align="right">
                                <template #default="{ row }">{{ formatBytes(row.trafficBytes) }}</template>
                            </el-table-column>
                        </el-table>
                        <el-empty v-if="!hourlyRows.length" :description="$t('commons.msg.noneData')" />
                    </el-tab-pane>

                    <el-tab-pane :label="$t('cluster.monitorReqLog')" name="reqlog">
                        <el-form :inline="true" class="mb-4">
                            <el-form-item>
                                <el-select v-model="logFilterStatus" class="p-w-150" clearable :placeholder="$t('cluster.monitorStatus')">
                                    <el-option label="2xx" value="2" />
                                    <el-option label="3xx" value="3" />
                                    <el-option label="4xx" value="4" />
                                    <el-option label="5xx" value="5" />
                                </el-select>
                            </el-form-item>
                            <el-form-item>
                                <el-select v-model="logFilterSpider" class="p-w-150" clearable :placeholder="$t('cluster.monitorSpiders')">
                                    <el-option :label="$t('cluster.monitorSpiderOnly')" :value="true" />
                                    <el-option :label="$t('cluster.monitorNonSpider')" :value="false" />
                                </el-select>
                            </el-form-item>
                            <el-form-item>
                                <el-input v-model="logFilterUrl" class="p-w-200" clearable :placeholder="$t('cluster.monitorUrlSearch')" />
                            </el-form-item>
                            <el-form-item>
                                <el-input v-model="logFilterIp" class="p-w-150" clearable placeholder="IP" />
                            </el-form-item>
                            <el-form-item>
                                <el-button @click="clearSiteLog">{{ $t('commons.button.clean') }}</el-button>
                                <el-button @click="exportLogCsv">{{ $t('commons.button.export') }}</el-button>
                            </el-form-item>
                        </el-form>
                        <el-table :data="filteredReqRows" style="width: 100%" max-height="560">
                            <el-table-column label="IP" prop="ip" width="140px" />
                            <el-table-column :label="$t('website.domain')" prop="domain" width="170px" />
                            <el-table-column label="URL" prop="url" min-width="220px" show-overflow-tooltip />
                            <el-table-column :label="$t('cluster.monitorTraffic')" width="110px" align="right">
                                <template #default="{ row }">{{ formatBytes(row.bytes) }}</template>
                            </el-table-column>
                            <el-table-column :label="$t('cluster.monitorRespTime')" width="110px" align="right">
                                <template #default>—</template>
                            </el-table-column>
                            <el-table-column :label="$t('cluster.monitorMethod')" prop="method" width="80px" />
                            <el-table-column :label="$t('cluster.monitorStatus')" prop="status" width="80px" />
                            <el-table-column :label="$t('commons.table.date')" prop="timeLabel" width="170px" />
                        </el-table>
                        <el-empty v-if="!filteredReqRows.length" :description="$t('commons.msg.noneData')" />
                    </el-tab-pane>

                    <el-tab-pane :label="$t('cluster.monitorSites')" name="sites">
                        <el-table :data="siteRows" style="width: 100%" v-loading="sitesLoading">
                            <el-table-column :label="$t('menu.website')" prop="domain" min-width="180px" />
                            <el-table-column :label="$t('cluster.monitorPv')" prop="pv" sortable align="right" />
                            <el-table-column :label="$t('cluster.monitorUv')" prop="uv" sortable align="right" />
                            <el-table-column :label="$t('cluster.monitorIps')" prop="ips" sortable align="right" />
                            <el-table-column :label="$t('cluster.monitorTraffic')" sortable align="right">
                                <template #default="{ row }">{{ formatBytes(row.trafficBytes) }}</template>
                            </el-table-column>
                            <el-table-column :label="$t('cluster.monitorSpiders')" prop="spiders" sortable align="right" />
                            <el-table-column :label="$t('cluster.monitorReqs')" prop="reqs" sortable align="right" />
                            <el-table-column :label="$t('cluster.monitor4xx')" prop="e4xx" sortable align="right" />
                            <el-table-column :label="$t('cluster.monitor5xx')" prop="e5xx" sortable align="right" />
                            <el-table-column :label="$t('commons.table.operate')" width="110px" align="center">
                                <template #default="{ row }">
                                    <el-button link type="primary" @click="selectSite(row.id)">
                                        {{ $t('cluster.monitorViewSite') }}
                                    </el-button>
                                </template>
                            </el-table-column>
                        </el-table>
                        <el-empty v-if="!siteRows.length && !sitesLoading" :description="$t('commons.msg.noneData')" />
                    </el-tab-pane>

                    <el-tab-pane :label="$t('cluster.monitorSettings')" name="settings">
                        <el-card shadow="never">
                            <el-form label-width="180px">
                                <el-form-item :label="$t('cluster.monitorAccessLog')">
                                    <el-switch v-model="logEnabledSwitch" @change="onToggleLog" />
                                    <el-text type="info" size="small" class="ml-2">
                                        {{ $t('cluster.monitorAccessLogHelper') }}
                                    </el-text>
                                </el-form-item>
                                <el-form-item :label="$t('cluster.monitorErrorLog')">
                                    <el-switch v-model="errorLogSwitch" @change="onToggleErrorLog" />
                                </el-form-item>
                                <el-form-item>
                                    <el-button @click="clearSiteLog">{{ $t('commons.button.clean') }}</el-button>
                                </el-form-item>
                            </el-form>
                            <el-alert type="info" :closable="false" :title="$t('cluster.monitorSettingsNote')" />
                        </el-card>
                    </el-tab-pane>
                </el-tabs>
            </template>
        </LayoutContent>
    </div>
</template>

<script lang="ts" setup>
import { computed, onMounted, ref, watch } from 'vue';
import i18n from '@/lang';
import { searchClusterNodes } from '@/api/modules/cluster';
import { searchWebsites, getWebsiteLog, opWebsiteLog } from '@/api/modules/website';
import { readByLine } from '@/api/modules/files';
import { Website } from '@/api/interface/website';
import { Cluster } from '@/api/interface/cluster';
import { MsgError, MsgSuccess } from '@/utils/message';
import { useGlobalStore } from '@/composables/useGlobalStore';

const { globalStore } = useGlobalStore();
const masterAlias = globalStore.getMasterAlias();

// --- Access-log parsing (nginx combined format) ---
// 1.2.3.4 - - [20/Nov/2026:10:11:12 +0800] "GET /path HTTP/1.1" 200 1234 "-" "UA"
const LOG_RE =
    /^(\S+) \S+ \S+ \[([^\]]+)\] "(\S+) ([^"]*) \S*" (\d{3}) (\d+|-) "[^"]*" "([^"]*)"/;
const MONTHS: Record<string, number> = {
    Jan: 0, Feb: 1, Mar: 2, Apr: 3, May: 4, Jun: 5,
    Jul: 6, Aug: 7, Sep: 8, Oct: 9, Nov: 10, Dec: 11,
};
const SPIDER_RE = /bot|spider|crawl|slurp|baidu|googlebot|bingbot|sogou|360spider|yandex|duckduck/i;

interface ReqRow {
    ip: string;
    domain: string;
    url: string;
    method: string;
    status: number;
    bytes: number;
    ua: string;
    spider: boolean;
    time: number;
    timeLabel: string;
    dateKey: string;
}

const parseLine = (line: string, domain: string): ReqRow | null => {
    const m = LOG_RE.exec(line);
    if (!m) return null;
    // [20/Nov/2026:10:11:12 +0800]
    const parts = m[2].split(/[/: ]/).filter(Boolean);
    if (parts.length < 7) return null;
    const month = MONTHS[parts[1]];
    if (month === undefined) return null;
    const day = Number(parts[0]);
    const year = Number(parts[2]);
    const hh = Number(parts[3]);
    const mm = Number(parts[4]);
    const ss = Number(parts[5]);
    const tz = parts[6]; // +0800
    const tzSign = tz.startsWith('-') ? -1 : 1;
    const tzMin = Number(tz.slice(1, 3)) * 60 + Number(tz.slice(3, 5));
    const time = Date.UTC(year, month, day, hh, mm, ss) - tzSign * tzMin * 60000;
    const status = Number(m[5]);
    return {
        ip: m[1],
        domain,
        url: m[4],
        method: m[3],
        status,
        bytes: m[6] === '-' ? 0 : Number(m[6]),
        ua: m[7],
        spider: SPIDER_RE.test(m[7]),
        time,
        timeLabel: new Date(time).toLocaleString(),
        dateKey: `${year}-${String(month + 1).padStart(2, '0')}-${String(day).padStart(2, '0')}`,
    };
};

const formatBytes = (bytes: number) => {
    if (!bytes) return '0 B';
    if (bytes < 1024) return `${bytes} B`;
    if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
    if (bytes < 1024 * 1024 * 1024) return `${(bytes / 1024 / 1024).toFixed(1)} MB`;
    return `${(bytes / 1024 / 1024 / 1024).toFixed(2)} GB`;
};

const aggregateRows = (rows: ReqRow[]) => {
    const ips = new Set<string>();
    const uvs = new Set<string>();
    let pv = 0;
    let trafficBytes = 0;
    let spiders = 0;
    let e4xx = 0;
    let e5xx = 0;
    for (const row of rows) {
        pv += 1;
        ips.add(row.ip);
        uvs.add(`${row.ip}|${row.ua}`);
        trafficBytes += row.bytes;
        if (row.spider) spiders += 1;
        if (row.status >= 400 && row.status < 500) e4xx += 1;
        if (row.status >= 500) e5xx += 1;
    }
    return { pv, uv: uvs.size, ips: ips.size, trafficBytes, spiders, reqs: rows.length, e4xx, e5xx };
};

const loading = ref(false);
const sitesLoading = ref(false);
const activeTab = ref('overview');
const nodeName = ref('local');
const onlineNodes = ref<Cluster.NodeInfo[]>([]);
const websites = ref<Website.WebsiteRes[]>([]);
const websiteId = ref<number | undefined>(undefined);
const logEnabled = ref(false);
const logEnabledSwitch = ref(false);
const errorLogSwitch = ref(false);
const parsedLines = ref(0);
const totalLines = ref(0);
const reqRows = ref<ReqRow[]>([]);
const trendRange = ref<'today' | '7d' | '30d'>('today');
const trendOption = ref<any>({});
const logFilterStatus = ref('');
const logFilterSpider = ref<boolean | undefined>(undefined);
const logFilterUrl = ref('');
const logFilterIp = ref('');
const siteRows = ref<Array<{ id: number; domain: string } & ReturnType<typeof aggregateRows>>>([]);

const selectedWebsite = computed(() => websites.value.find((item) => item.id === websiteId.value));
const todayKey = computed(() => {
    const now = new Date();
    return `${now.getFullYear()}-${String(now.getMonth() + 1).padStart(2, '0')}-${String(now.getDate()).padStart(2, '0')}`;
});
const todayRows = computed(() => reqRows.value.filter((row) => row.dateKey === todayKey.value));
const todayAgg = computed(() => aggregateRows(todayRows.value));
const todayStats = computed(() => [
    { label: i18n.global.t('cluster.monitorPv'), value: todayAgg.value.pv },
    { label: i18n.global.t('cluster.monitorUv'), value: todayAgg.value.uv },
    { label: i18n.global.t('cluster.monitorIps'), value: todayAgg.value.ips },
    { label: i18n.global.t('cluster.monitorTraffic'), value: formatBytes(todayAgg.value.trafficBytes) },
    { label: i18n.global.t('cluster.monitorSpiders'), value: todayAgg.value.spiders },
    { label: i18n.global.t('cluster.monitorReqs'), value: todayAgg.value.reqs },
    { label: i18n.global.t('cluster.monitor4xx'), value: todayAgg.value.e4xx },
    { label: i18n.global.t('cluster.monitor5xx'), value: todayAgg.value.e5xx },
]);
const dailyRows = computed(() => {
    const byDay = new Map<string, ReqRow[]>();
    for (const row of reqRows.value) {
        const list = byDay.get(row.dateKey) || [];
        list.push(row);
        byDay.set(row.dateKey, list);
    }
    return [...byDay.entries()]
        .map(([date, rows]) => ({ date, ...aggregateRows(rows) }))
        .sort((a, b) => (a.date < b.date ? 1 : -1));
});
const hourlyRows = computed(() => {
    const days = trendRange.value === 'today' ? 1 : trendRange.value === '7d' ? 7 : 30;
    const cutoff = Date.now() - days * 24 * 3600 * 1000;
    const byHour = new Map<string, ReqRow[]>();
    for (const row of reqRows.value) {
        if (row.time < cutoff) continue;
        const d = new Date(row.time);
        const key = `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(
            d.getDate(),
        ).padStart(2, '0')} ${String(d.getHours()).padStart(2, '0')}:00`;
        const list = byHour.get(key) || [];
        list.push(row);
        byHour.set(key, list);
    }
    return [...byHour.entries()]
        .map(([label, rows]) => ({ label, ...aggregateRows(rows) }))
        .sort((a, b) => (a.label < b.label ? -1 : 1));
});
const filteredReqRows = computed(() => {
    return reqRows.value
        .filter((row) => {
            if (logFilterStatus.value && String(row.status)[0] !== logFilterStatus.value) return false;
            if (logFilterSpider.value !== undefined && row.spider !== logFilterSpider.value) return false;
            if (logFilterUrl.value && !row.url.includes(logFilterUrl.value)) return false;
            if (logFilterIp.value && !row.ip.includes(logFilterIp.value)) return false;
            return true;
        })
        .slice(0, 500);
});

const buildTrend = () => {
    const rows = hourlyRows.value;
    trendOption.value = {
        tooltip: { trigger: 'axis' },
        xAxis: { type: 'category', data: rows.map((item) => item.label) },
        yAxis: { type: 'value' },
        series: [
            { name: i18n.global.t('cluster.monitorPv'), type: 'line', data: rows.map((item) => item.pv) },
            { name: i18n.global.t('cluster.monitorUv'), type: 'line', data: rows.map((item) => item.uv) },
        ],
    };
};

const loadNodes = async () => {
    try {
        const res = await searchClusterNodes({ page: 1, pageSize: 500 });
        onlineNodes.value = (res.data.items || []).filter((item) => item.status === 'online');
    } catch {
        onlineNodes.value = [];
    }
};

const onNodeChange = async () => {
    websiteId.value = undefined;
    websites.value = [];
    reqRows.value = [];
    parsedLines.value = 0;
    await loadWebsites();
};

const loadWebsites = async () => {
    const param = nodeName.value;
    try {
        const res = await searchWebsites(
            { page: 1, pageSize: 100, name: '', orderBy: 'created_at', order: 'descending', websiteGroupId: 0 },
            param,
        );
        websites.value = res.data.items || [];
        if (websites.value.length && !websiteId.value) {
            websiteId.value = websites.value[0].id;
            await loadSite();
        }
    } catch {
        websites.value = [];
    }
};

const loadSite = async () => {
    const site = selectedWebsite.value;
    if (!site) return;
    loading.value = true;
    reqRows.value = [];
    parsedLines.value = 0;
    totalLines.value = 0;
    try {
        const param = nodeName.value;
        const meta = await getWebsiteLog({ id: site.id, logType: 'access.log', page: 1, pageSize: 1 }, param);
        logEnabled.value = !!meta.data?.enable;
        logEnabledSwitch.value = logEnabled.value;
        if (!logEnabled.value) {
            return;
        }
        // Newest lines first, capped: stats cover only this window.
        const res = await readByLine(
            { id: site.id, type: 'website', name: 'access.log', page: 1, pageSize: 2000, latest: true },
            param,
        );
        const lines: string[] = res.data?.lines || [];
        totalLines.value = res.data?.totalLines || 0;
        const rows: ReqRow[] = [];
        for (const line of lines) {
            const row = parseLine(line, site.primaryDomain);
            if (row) rows.push(row);
        }
        // readByLine latest mode returns newest-first; keep chronological
        // order for tables (newest first is what the log tab shows).
        rows.sort((a, b) => b.time - a.time);
        reqRows.value = rows;
        parsedLines.value = lines.length;
        buildTrend();
    } catch {
        reqRows.value = [];
    } finally {
        loading.value = false;
    }
};

const selectSite = async (id: number) => {
    websiteId.value = id;
    activeTab.value = 'overview';
    await loadSite();
};

const onToggleLog = async () => {
    const site = selectedWebsite.value;
    if (!site) return;
    try {
        await opWebsiteLog(
            { id: site.id, operate: logEnabledSwitch.value ? 'enable' : 'disable', logType: 'access.log' },
            nodeName.value,
        );
        MsgSuccess(i18n.global.t('commons.msg.operationSuccess'));
        await loadSite();
    } catch {
        logEnabledSwitch.value = logEnabled.value;
    }
};

const onToggleErrorLog = async () => {
    const site = selectedWebsite.value;
    if (!site) return;
    try {
        await opWebsiteLog(
            { id: site.id, operate: errorLogSwitch.value ? 'enable' : 'disable', logType: 'error.log' },
            nodeName.value,
        );
        MsgSuccess(i18n.global.t('commons.msg.operationSuccess'));
    } catch {
        errorLogSwitch.value = false;
    }
};

const clearSiteLog = async () => {
    const site = selectedWebsite.value;
    if (!site) return;
    try {
        await opWebsiteLog(
            { id: site.id, operate: 'delete', logType: 'access.log' },
            nodeName.value,
        );
        MsgSuccess(i18n.global.t('commons.msg.operationSuccess'));
        await loadSite();
    } catch {
        MsgError(i18n.global.t('commons.msg.operationFailed'));
    }
};

const exportLogCsv = () => {
    const header = 'ip,domain,url,method,status,bytes,spider,time\n';
    const body = filteredReqRows.value
        .map((row) =>
            [row.ip, row.domain, `"${row.url.replace(/"/g, '""')}"`, row.method, row.status, row.bytes, row.spider, row.timeLabel].join(','),
        )
        .join('\n');
    const blob = new Blob([header + body], { type: 'text/csv;charset=utf-8' });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = `monitor-${selectedWebsite.value?.primaryDomain || 'site'}.csv`;
    a.click();
    URL.revokeObjectURL(url);
};

const loadSiteRows = async () => {
    const param = nodeName.value;
    sitesLoading.value = true;
    try {
        const list = websites.value.slice(0, 20);
        const results = await Promise.all(
            list.map(async (site) => {
                try {
                    const meta = await getWebsiteLog({ id: site.id, logType: 'access.log', page: 1, pageSize: 1 }, param);
                    if (!meta.data?.enable) {
                        return { id: site.id, domain: site.primaryDomain, ...aggregateRows([]) };
                    }
                    const res = await readByLine(
                        { id: site.id, type: 'website', name: 'access.log', page: 1, pageSize: 300, latest: true },
                        param,
                    );
                    const rows: ReqRow[] = [];
                    for (const line of res.data?.lines || []) {
                        const row = parseLine(line, site.primaryDomain);
                        if (row) rows.push(row);
                    }
                    const today = rows.filter((row) => row.dateKey === todayKey.value);
                    return { id: site.id, domain: site.primaryDomain, ...aggregateRows(today) };
                } catch {
                    return { id: site.id, domain: site.primaryDomain, ...aggregateRows([]) };
                }
            }),
        );
        siteRows.value = results;
    } finally {
        sitesLoading.value = false;
    }
};

watch(activeTab, (tab) => {
    if (tab === 'sites' && websites.value.length) {
        loadSiteRows();
    }
    if (tab === 'overview' || tab === 'trends') {
        buildTrend();
    }
});

onMounted(async () => {
    await loadNodes();
    await loadWebsites();
});
</script>

<style scoped>
.mb-4 {
    margin-bottom: 16px;
}

.ml-2 {
    margin-left: 8px;
}

.card-header-with-select {
    display: flex;
    justify-content: space-between;
    align-items: center;
}
</style>

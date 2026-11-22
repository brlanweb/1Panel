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
                    <el-alert
                        v-if="loadError"
                        type="error"
                        :closable="false"
                        show-icon
                        :title="$t('cluster.monitorLoadErrorTitle')"
                        :description="loadError"
                    />
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
                        <el-row :gutter="12" class="mb-4">
                            <el-col :span="6">
                                <el-card shadow="hover">
                                    <el-statistic
                                        :title="$t('cluster.monitorRealtimeReqs')"
                                        :value="realtimeAgg.reqs"
                                    />
                                </el-card>
                            </el-col>
                            <el-col :span="6">
                                <el-card shadow="hover">
                                    <el-statistic
                                        :title="$t('cluster.monitorRealtimeTraffic')"
                                        :value="realtimeAgg.traffic"
                                    />
                                </el-card>
                            </el-col>
                            <el-col :span="12">
                                <el-card shadow="hover">
                                    <el-text type="info" size="small">
                                        {{ $t('cluster.monitorRealtimeNote') }}
                                    </el-text>
                                </el-card>
                            </el-col>
                        </el-row>
                        <el-card shadow="never" class="mb-4">
                            <template #header>
                                <div class="card-header-with-select">
                                    <span>{{ $t('cluster.visitorTrend') }}</span>
                                    <el-select v-model="trendRange" class="p-w-150" @change="loadTrend">
                                        <el-option :label="$t('cluster.today')" value="today" />
                                        <el-option :label="$t('cluster.last7Days')" value="7d" />
                                        <el-option :label="$t('cluster.last30Days')" value="30d" />
                                    </el-select>
                                </div>
                            </template>
                            <v-charts type="line" :option="trendOption" height="280px" v-loading="trendLoading" />
                        </el-card>
                    </el-tab-pane>

                    <el-tab-pane :label="$t('cluster.monitorVisits')" name="visits">
                        <el-row :gutter="12">
                            <el-col :span="8" v-for="card in rankCards" :key="card.title" class="mb-4">
                                <el-card shadow="never">
                                    <template #header>{{ card.title }}</template>
                                    <el-table :data="card.rows" size="small" max-height="320">
                                        <el-table-column :label="card.colLabel" prop="label" show-overflow-tooltip />
                                        <el-table-column
                                            :label="$t('cluster.monitorCount')"
                                            prop="count"
                                            width="80px"
                                            align="right"
                                        />
                                    </el-table>
                                    <el-empty v-if="!card.rows.length" :description="$t('commons.msg.noneData')" />
                                </el-card>
                            </el-col>
                        </el-row>
                    </el-tab-pane>

                    <el-tab-pane :label="$t('cluster.monitorTrends')" name="trends">
                        <el-card shadow="never" class="mb-4">
                            <template #header>
                                <div class="card-header-with-select">
                                    <span>{{ $t('cluster.visitorTrend') }}</span>
                                    <el-select v-model="trendRange" class="p-w-150" @change="loadTrend">
                                        <el-option :label="$t('cluster.today')" value="today" />
                                        <el-option :label="$t('cluster.last7Days')" value="7d" />
                                        <el-option :label="$t('cluster.last30Days')" value="30d" />
                                    </el-select>
                                </div>
                            </template>
                            <v-charts type="line" :option="trendOption" height="280px" v-loading="trendLoading" />
                        </el-card>
                        <el-table :data="trendRows" style="width: 100%" v-loading="trendLoading">
                            <el-table-column :label="$t('commons.table.date')" prop="label" width="160px" />
                            <el-table-column :label="$t('cluster.monitorPv')" prop="pv" align="right" />
                            <el-table-column :label="$t('cluster.monitorUv')" prop="uv" align="right" />
                            <el-table-column :label="$t('cluster.monitorIps')" prop="ips" align="right" />
                        </el-table>
                    </el-tab-pane>

                    <el-tab-pane :label="$t('cluster.monitorReqLog')" name="reqlog">
                        <el-row :gutter="12" class="mb-4">
                            <el-col :span="3" v-for="chip in reqChips" :key="chip.label">
                                <el-card shadow="hover">
                                    <el-statistic :title="chip.label" :value="chip.value" />
                                </el-card>
                            </el-col>
                        </el-row>
                        <el-form :inline="true" class="mb-4">
                            <el-form-item>
                                <el-date-picker
                                    v-model="logFilterRange"
                                    type="datetimerange"
                                    range-separator="→"
                                    :start-placeholder="$t('commons.table.date')"
                                    :end-placeholder="$t('commons.table.date')"
                                />
                            </el-form-item>
                            <el-form-item>
                                <el-select
                                    v-model="logFilterMethod"
                                    class="p-w-150"
                                    clearable
                                    :placeholder="$t('cluster.monitorMethod')"
                                >
                                    <el-option label="GET" value="GET" />
                                    <el-option label="POST" value="POST" />
                                    <el-option label="PUT" value="PUT" />
                                    <el-option label="DELETE" value="DELETE" />
                                    <el-option label="HEAD" value="HEAD" />
                                </el-select>
                            </el-form-item>
                            <el-form-item>
                                <el-select
                                    v-model="logFilterStatus"
                                    class="p-w-150"
                                    clearable
                                    :placeholder="$t('cluster.monitorStatus')"
                                >
                                    <el-option label="2xx" value="2" />
                                    <el-option label="3xx" value="3" />
                                    <el-option label="4xx" value="4" />
                                    <el-option label="5xx" value="5" />
                                </el-select>
                            </el-form-item>
                            <el-form-item>
                                <el-select
                                    v-model="logFilterSpider"
                                    class="p-w-150"
                                    clearable
                                    :placeholder="$t('cluster.monitorSpiders')"
                                >
                                    <el-option :label="$t('cluster.monitorSpiderOnly')" :value="true" />
                                    <el-option :label="$t('cluster.monitorNonSpider')" :value="false" />
                                </el-select>
                            </el-form-item>
                            <el-form-item>
                                <el-input
                                    v-model="logFilterUrl"
                                    class="p-w-200"
                                    clearable
                                    :placeholder="$t('cluster.monitorUrlSearch')"
                                />
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
                            <el-table-column
                                :label="$t('cluster.monitorSpiders')"
                                prop="spiders"
                                sortable
                                align="right"
                            />
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
import { searchWebsites, getWebsiteLog, getWebsiteLogTrend, opWebsiteLog } from '@/api/modules/website';
import { readByLine } from '@/api/modules/files';
import { Website } from '@/api/interface/website';
import { Cluster } from '@/api/interface/cluster';
import { MsgError, MsgSuccess } from '@/utils/message';
import { ElMessageBox } from 'element-plus';
import { useGlobalStore } from '@/composables/useGlobalStore';

const { globalStore } = useGlobalStore();
const masterAlias = globalStore.getMasterAlias();

// --- Access-log parsing (nginx combined format) ---
// 1.2.3.4 - - [20/Nov/2026:10:11:12 +0800] "GET /path HTTP/1.1" 200 1234 "https://ref" "UA"
const LOG_RE = /^(\S+) \S+ \S+ \[([^\]]+)\] "(\S+) ([^"]*) \S*" (\d{3}) (\d+|-) "([^"]*)" "([^"]*)"/;
const MONTHS: Record<string, number> = {
    Jan: 0,
    Feb: 1,
    Mar: 2,
    Apr: 3,
    May: 4,
    Jun: 5,
    Jul: 6,
    Aug: 7,
    Sep: 8,
    Oct: 9,
    Nov: 10,
    Dec: 11,
};
const SPIDER_RE = /bot|spider|crawl|slurp|baidu|googlebot|bingbot|sogou|360spider|yandex|duckduck/i;

// Minimal UA parsing for the visits rankings (browser / OS / device). The
// default access-log format carries no response-time field, so request-log
// latency is shown as — by design, not as 0.
const parseUA = (ua: string): { browser: string; os: string; device: string } => {
    const lower = ua.toLowerCase();
    let browser = 'Other';
    if (lower.includes('edg/')) browser = 'Edge';
    else if (lower.includes('opr/') || lower.includes('opera')) browser = 'Opera';
    else if (lower.includes('chrome/')) browser = 'Chrome';
    else if (lower.includes('firefox/')) browser = 'Firefox';
    else if (lower.includes('safari/')) browser = 'Safari';
    else if (lower.includes('msie') || lower.includes('trident/')) browser = 'IE';
    let os = 'Other';
    if (lower.includes('windows')) os = 'Windows';
    else if (lower.includes('mac os')) os = 'macOS';
    else if (lower.includes('android')) os = 'Android';
    else if (lower.includes('iphone') || lower.includes('ipad')) os = 'iOS';
    else if (lower.includes('linux')) os = 'Linux';
    const device = /mobile|android|iphone|ipad/i.test(ua) ? 'Mobile' : 'Desktop';
    return { browser, os, device };
};

interface ReqRow {
    ip: string;
    domain: string;
    url: string;
    method: string;
    status: number;
    bytes: number;
    referer: string;
    ua: string;
    browser: string;
    os: string;
    device: string;
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
    const ua = m[8];
    const { browser, os, device } = parseUA(ua);
    return {
        ip: m[1],
        domain,
        url: m[4],
        method: m[3],
        status,
        bytes: m[6] === '-' ? 0 : Number(m[6]),
        referer: m[7],
        ua,
        browser,
        os,
        device,
        spider: SPIDER_RE.test(ua),
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
// Set when a fetch actually fails (node unreachable, API error, log read
// error). Shown as an alert; distinct from "log not enabled" (banner) and
// from "enabled but no rows yet" (empty state).
const loadError = ref('');
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
const logFilterMethod = ref('');
const logFilterRange = ref<[Date, Date] | null>(null);
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
// --- Rankings for the visits tab (top 10 each) ---
const topBy = (key: (row: ReqRow) => string, rows: ReqRow[], limit = 10) => {
    const counts = new Map<string, number>();
    for (const row of rows) {
        const k = key(row);
        counts.set(k, (counts.get(k) || 0) + 1);
    }
    return [...counts.entries()]
        .map(([label, count]) => ({ label, count }))
        .sort((a, b) => b.count - a.count)
        .slice(0, limit);
};
const rankings = computed(() => ({
    urls: topBy((r) => r.url, reqRows.value),
    referers: topBy((r) => (r.referer && r.referer !== '-' ? r.referer : '—'), reqRows.value),
    ips: topBy((r) => r.ip, reqRows.value),
    browsers: topBy((r) => r.browser, reqRows.value),
    os: topBy((r) => r.os, reqRows.value),
    devices: topBy((r) => r.device, reqRows.value),
    statuses: topBy((r) => String(r.status), reqRows.value),
}));
const rankCards = computed(() => {
    const t = (key: string) => i18n.global.t(key);
    const r = rankings.value;
    return [
        { title: t('cluster.monitorRankUrl'), colLabel: 'URL', rows: r.urls },
        { title: t('cluster.monitorRankReferer'), colLabel: t('cluster.monitorRankSource'), rows: r.referers },
        { title: t('cluster.monitorRankIp'), colLabel: 'IP', rows: r.ips },
        { title: t('cluster.monitorRankBrowser'), colLabel: t('cluster.monitorRankBrowser'), rows: r.browsers },
        { title: t('cluster.monitorRankOs'), colLabel: t('cluster.monitorRankOs'), rows: r.os },
        { title: t('cluster.monitorRankDevice'), colLabel: t('cluster.monitorRankDevice'), rows: r.devices },
        { title: t('cluster.monitorRankStatus'), colLabel: t('cluster.monitorStatus'), rows: r.statuses },
    ];
});
// --- Realtime (last 60 s) for the overview tab, from the parsed window ---
const realtimeAgg = computed(() => {
    const cutoff = Date.now() - 60 * 1000;
    const rows = reqRows.value.filter((row) => row.time >= cutoff);
    return { reqs: rows.length, traffic: formatBytes(rows.reduce((sum, r) => sum + r.bytes, 0)) };
});
// --- Request-log chips (whole parsed window) ---
const reqChips = computed(() => {
    const agg = aggregateRows(reqRows.value);
    const ok = reqRows.value.filter((r) => r.status >= 200 && r.status < 300).length;
    const errors = agg.e4xx + agg.e5xx;
    return [
        { label: i18n.global.t('cluster.monitorReqs'), value: agg.reqs },
        { label: '200', value: ok },
        { label: i18n.global.t('cluster.monitorErrors'), value: errors },
        { label: i18n.global.t('cluster.monitorIps'), value: agg.ips },
        { label: i18n.global.t('cluster.monitorSpiders'), value: agg.spiders },
        { label: i18n.global.t('cluster.monitorTraffic'), value: formatBytes(agg.trafficBytes) },
        { label: i18n.global.t('cluster.monitorAvgResp'), value: '—' },
    ];
});
const filteredReqRows = computed(() => {
    return reqRows.value
        .filter((row) => {
            if (logFilterStatus.value && String(row.status)[0] !== logFilterStatus.value) return false;
            if (logFilterSpider.value !== undefined && row.spider !== logFilterSpider.value) return false;
            if (logFilterUrl.value && !row.url.includes(logFilterUrl.value)) return false;
            if (logFilterIp.value && !row.ip.includes(logFilterIp.value)) return false;
            if (logFilterMethod.value && row.method !== logFilterMethod.value) return false;
            if (logFilterRange.value) {
                const [start, end] = logFilterRange.value;
                if (row.time < start.getTime() || row.time > end.getTime()) return false;
            }
            return true;
        })
        .slice(0, 500);
});

// Visitor trend is aggregated by the agent over the whole range (current
// access.log plus cut-log archives); the 2000-line window used by the other
// cards cannot cover 7 / 30 days.
const trendData = ref<Website.WebSiteLogTrend | null>(null);
const trendLoading = ref(false);
let trendSeq = 0;
const trendRows = computed(() => {
    const d = trendData.value;
    if (!d) return [];
    return d.labels.map((label, i) => ({ label, pv: d.pv[i] || 0, uv: d.uv[i] || 0, ips: d.ips[i] || 0 })).reverse();
});

const buildTrend = () => {
    const d = trendData.value;
    trendOption.value = {
        xData: d?.labels || [],
        yData: [
            { name: i18n.global.t('cluster.monitorPv'), data: d?.pv || [] },
            { name: i18n.global.t('cluster.monitorUv'), data: d?.uv || [] },
            { name: i18n.global.t('cluster.monitorIps'), data: d?.ips || [] },
        ],
        grid: { left: '5%', right: '5%', bottom: '12%' },
        formatStr: '',
        yAxis: [{ type: 'value', minInterval: 1 }],
    };
};

const loadTrend = async () => {
    const site = selectedWebsite.value;
    const seq = ++trendSeq;
    if (!site || !logEnabled.value) {
        trendData.value = null;
        buildTrend();
        return;
    }
    trendLoading.value = true;
    try {
        const res = await getWebsiteLogTrend({ id: site.id, range: trendRange.value }, nodeName.value);
        if (seq !== trendSeq) return;
        trendData.value = res.data;
    } catch {
        if (seq !== trendSeq) return;
        trendData.value = null;
    } finally {
        if (seq === trendSeq) {
            trendLoading.value = false;
            buildTrend();
        }
    }
};

const loadNodes = async () => {
    try {
        const res = await searchClusterNodes({ page: 1, pageSize: 500 });
        // Only nodes whose last check failed are hidden; a freshly added
        // node is "unknown" until the first check and must still be usable.
        onlineNodes.value = (res.data.items || []).filter((item) => item.status !== 'offline');
    } catch {
        onlineNodes.value = [];
    }
};

const onNodeChange = async () => {
    websiteId.value = undefined;
    websites.value = [];
    reqRows.value = [];
    siteRows.value = [];
    parsedLines.value = 0;
    totalLines.value = 0;
    logEnabled.value = false;
    logEnabledSwitch.value = false;
    errorLogSwitch.value = false;
    loadError.value = '';
    trendData.value = null;
    await loadWebsites();
    buildTrend();
};

const loadWebsites = async () => {
    const param = nodeName.value;
    try {
        const res = await searchWebsites(
            { page: 1, pageSize: 100, name: '', orderBy: 'created_at', order: 'descending', websiteGroupId: 0 },
            param,
        );
        websites.value = res.data.items || [];
        loadError.value = websites.value.length ? '' : i18n.global.t('cluster.monitorNoWebsite');
        if (websites.value.length && !websiteId.value) {
            websiteId.value = websites.value[0].id;
            await loadSite();
        }
    } catch (e: any) {
        websites.value = [];
        loadError.value = e?.message || i18n.global.t('cluster.monitorLoadError');
    }
};

const loadSite = async () => {
    const site = selectedWebsite.value;
    if (!site) return;
    loading.value = true;
    logFilterStatus.value = '';
    logFilterSpider.value = undefined;
    logFilterUrl.value = '';
    logFilterIp.value = '';
    logFilterMethod.value = '';
    logFilterRange.value = null;
    reqRows.value = [];
    parsedLines.value = 0;
    totalLines.value = 0;
    loadError.value = '';
    try {
        const param = nodeName.value;
        const meta = await getWebsiteLog({ id: site.id, logType: 'access.log', page: 1, pageSize: 1 }, param);
        logEnabled.value = !!meta.data?.enable;
        logEnabledSwitch.value = logEnabled.value;
        // Error-log switch state was never initialised from the server.
        try {
            const errMeta = await getWebsiteLog({ id: site.id, logType: 'error.log', page: 1, pageSize: 1 }, param);
            errorLogSwitch.value = !!errMeta.data?.enable;
        } catch {
            errorLogSwitch.value = false;
        }
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
    } catch (e: any) {
        reqRows.value = [];
        loadError.value = e?.message || i18n.global.t('cluster.monitorLoadError');
    } finally {
        loading.value = false;
        loadTrend();
        if (activeTab.value === 'sites') {
            loadSiteRows();
        }
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
    // Deleting the access log is irreversible and wipes every statistic on
    // this page for the site: always confirm first.
    try {
        await ElMessageBox.confirm(
            i18n.global.t('cluster.monitorClearConfirm', [site.primaryDomain]),
            i18n.global.t('commons.button.clean'),
            { type: 'warning' },
        );
    } catch {
        return;
    }
    try {
        await opWebsiteLog({ id: site.id, operate: 'delete', logType: 'access.log' }, nodeName.value);
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
            [
                row.ip,
                row.domain,
                `"${row.url.replace(/"/g, '""')}"`,
                row.method,
                row.status,
                row.bytes,
                row.spider,
                row.timeLabel,
            ].join(','),
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
                    const meta = await getWebsiteLog(
                        { id: site.id, logType: 'access.log', page: 1, pageSize: 1 },
                        param,
                    );
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

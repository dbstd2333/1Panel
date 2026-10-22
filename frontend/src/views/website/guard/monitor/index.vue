<template>
    <div v-loading="loading">
        <div class="flex flex-wrap items-center gap-3 mb-4">
            <el-select v-model="site" class="p-w-200" @change="search">
                <el-option :label="$t('website.guard.allSites')" value="" />
                <el-option
                    v-for="item in enabledSites"
                    :key="item.alias"
                    :label="item.primaryDomain"
                    :value="item.alias"
                />
            </el-select>
            <el-radio-group v-model="range" @change="search">
                <el-radio-button value="1h">{{ $t('website.guard.range1h') }}</el-radio-button>
                <el-radio-button value="24h">{{ $t('website.guard.range24h') }}</el-radio-button>
                <el-radio-button value="7d">{{ $t('website.guard.range7d') }}</el-radio-button>
                <el-radio-button value="30d">{{ $t('website.guard.range30d') }}</el-radio-button>
            </el-radio-group>
            <el-button :icon="Refresh" circle @click="search" />
            <el-checkbox v-model="autoRefresh" @change="onAutoRefresh">
                {{ $t('website.guard.autoRefresh') }}
            </el-checkbox>
            <el-button type="primary" plain @click="openEnable()">{{ $t('website.guard.monitorManage') }}</el-button>
        </div>
        <div class="input-help !mt-0 mb-3">{{ $t('website.guard.monitorHelper') }}</div>

        <el-alert
            v-if="pendingSite"
            type="warning"
            show-icon
            :closable="false"
            class="mb-3"
            :title="$t('website.guard.monitorSiteOff', [pendingSite.primaryDomain])"
        >
            <el-button type="primary" link @click="openEnable(pendingSite.websiteId)">
                {{ $t('website.guard.monitorEnable') }}
            </el-button>
        </el-alert>

        <el-empty v-if="statusLoaded && enabledSites.length === 0" :description="$t('website.guard.monitorEmpty')">
            <el-button type="primary" @click="openEnable()">{{ $t('website.guard.monitorEnable') }}</el-button>
        </el-empty>

        <template v-else>
        <el-row :gutter="12" class="mb-2">
            <el-col v-for="card in cards" :key="card.label" :xs="12" :sm="8" :md="6" :lg="4" class="mb-3">
                <el-card shadow="never" class="stat-card">
                    <div class="stat-label">{{ card.label }}</div>
                    <div class="stat-value">{{ card.value }}</div>
                </el-card>
            </el-col>
        </el-row>

        <el-row :gutter="12">
            <el-col :xs="24" :lg="12" class="mb-3">
                <el-card shadow="never">
                    <template #header>{{ $t('website.guard.chartRequests') }}</template>
                    <v-charts height="260px" :option="charts.requests" v-if="charts.requests" />
                </el-card>
            </el-col>
            <el-col :xs="24" :lg="12" class="mb-3">
                <el-card shadow="never">
                    <template #header>{{ $t('website.guard.chartTraffic') }}</template>
                    <v-charts height="260px" :option="charts.traffic" v-if="charts.traffic" />
                </el-card>
            </el-col>
            <el-col :xs="24" :lg="12" class="mb-3">
                <el-card shadow="never">
                    <template #header>{{ $t('website.guard.chartStatus') }}</template>
                    <v-charts height="260px" :option="charts.status" v-if="charts.status" />
                </el-card>
            </el-col>
            <el-col :xs="24" :lg="12" class="mb-3">
                <el-card shadow="never">
                    <template #header>{{ $t('website.guard.chartRt') }}</template>
                    <v-charts height="260px" :option="charts.rt" v-if="charts.rt" />
                </el-card>
            </el-col>
        </el-row>

        <el-row :gutter="12">
            <el-col v-for="item in topTables" :key="item.key" :xs="24" :lg="12" class="mb-3">
                <el-card shadow="never">
                    <template #header>{{ item.label }}</template>
                    <el-table :data="top[item.key] || []" size="small" max-height="300">
                        <el-table-column :label="item.column" prop="name" show-overflow-tooltip>
                            <template #default="{ row }">
                                {{ item.key === 'country' ? countryName(row.name) : row.name }}
                            </template>
                        </el-table-column>
                        <el-table-column :label="$t('website.guard.count')" prop="count" width="110" />
                    </el-table>
                </el-card>
            </el-col>
        </el-row>

        <el-card shadow="never">
            <template #header>{{ $t('website.guard.slowRequests') }}</template>
            <ComplexTable :data="slow" :pagination-config="slowPage" @search="loadSlow">
                <el-table-column :label="$t('commons.table.date')" width="170">
                    <template #default="{ row }">{{ formatTime(row.time) }}</template>
                </el-table-column>
                <el-table-column :label="$t('website.guard.site')" prop="site" width="140" show-overflow-tooltip />
                <el-table-column label="URI" min-width="260" show-overflow-tooltip>
                    <template #default="{ row }">{{ row.method }} {{ row.uri }}</template>
                </el-table-column>
                <el-table-column label="IP" prop="ip" width="150" />
                <el-table-column :label="$t('website.guard.statusCode')" prop="status" width="90" />
                <el-table-column :label="$t('website.guard.duration')" width="110">
                    <template #default="{ row }">{{ Math.round(row.requestTime) }} ms</template>
                </el-table-column>
            </ComplexTable>
        </el-card>
        </template>

        <DialogPro v-model="enableOpen" :title="$t('website.guard.monitorEnable')" size="small">
            <el-alert type="info" :closable="false" class="mb-3" :title="$t('website.guard.monitorEnableHelper')" />
            <el-checkbox-group v-model="enableIds" class="flex flex-col">
                <el-checkbox v-for="item in sites" :key="item.websiteId" :value="item.websiteId" :disabled="item.enable">
                    {{ item.primaryDomain }}
                    <span v-if="item.enable" class="input-help !mt-0 ml-1">
                        {{ item.mode === 'off' ? $t('website.guard.monitorOnly') : $t('website.guard.guarded') }}
                    </span>
                </el-checkbox>
            </el-checkbox-group>
            <span v-if="sites.length === 0" class="input-help">{{ $t('website.guard.noWebsite') }}</span>
            <template #footer>
                <el-button @click="enableOpen = false">{{ $t('commons.button.cancel') }}</el-button>
                <el-button type="primary" :loading="enabling" :disabled="newIds.length === 0" @click="submitEnable">
                    {{ $t('commons.button.confirm') }}
                </el-button>
            </template>
        </DialogPro>
    </div>
</template>

<script lang="ts" setup>
import { Refresh } from '@element-plus/icons-vue';
import { WebsiteGuard } from '@/api/interface/website-guard';
import {
    getGuardSite,
    getGuardStatus,
    loadGuardCountries,
    updateGuardGlobal,
    updateGuardSite,
    loadMonitorOverview,
    loadMonitorTop,
    loadMonitorTrend,
    searchMonitorSlow,
} from '@/api/modules/website-guard';
import { computeSize } from '@/utils/size';
import { dateFormat } from '@/utils/date';
import { useGlobalStore } from '@/composables/useGlobalStore';
import { MsgSuccess } from '@/utils/message';
import i18n from '@/lang';

const { globalStore } = useGlobalStore();
const route = useRoute();
const loading = ref(false);
const site = ref(String(route.query.site || ''));
const range = ref('24h');
const status = ref<WebsiteGuard.Status>();
const statusLoaded = ref(false);
const sites = computed(() => status.value?.sites || []);
const enabledSites = computed(() => sites.value.filter((s) => s.enable));
const pendingSite = ref<WebsiteGuard.SiteBrief>();
const autoRefresh = ref(false);
let refreshTimer: ReturnType<typeof setInterval> | undefined;
const enableOpen = ref(false);
const enabling = ref(false);
const enableIds = ref<number[]>([]);
const newIds = computed(() =>
    enableIds.value.filter((id) => !sites.value.find((s) => s.websiteId === id)?.enable),
);
const overview = ref<WebsiteGuard.MonitorOverview>();
const top = ref<Partial<WebsiteGuard.MonitorTop>>({});
const slow = ref<WebsiteGuard.Slow[]>([]);
const countries = ref<Record<string, string>>({});
const charts = reactive<Record<string, any>>({});
const slowPage = reactive({ currentPage: 1, pageSize: 10, total: 0 });

const rangeSeconds: Record<string, number> = { '1h': 3600, '24h': 86400, '7d': 7 * 86400, '30d': 30 * 86400 };

const timeRange = () => {
    const end = Math.floor(Date.now() / 1000);
    return { site: site.value, startTime: end - rangeSeconds[range.value], endTime: end };
};

const formatNumber = (v: number) => (v || 0).toLocaleString();
const formatTime = (ts: number) => dateFormat(0, 0, ts * 1000);

const cards = computed(() => {
    const o = overview.value;
    if (!o) {
        return [];
    }
    const errors = o.s4xx + o.s5xx;
    return [
        { label: i18n.global.t('website.guard.requests'), value: formatNumber(o.requests) },
        { label: 'UV', value: formatNumber(o.uv) },
        { label: 'IP', value: formatNumber(o.ip) },
        { label: i18n.global.t('website.guard.peakQps'), value: o.peakQps.toFixed(2) },
        { label: i18n.global.t('website.guard.trafficOut'), value: computeSize(o.bytesOut) },
        { label: i18n.global.t('website.guard.trafficIn'), value: computeSize(o.bytesIn) },
        {
            label: i18n.global.t('website.guard.errorRate'),
            value: o.requests ? ((errors / o.requests) * 100).toFixed(2) + '%' : '0%',
        },
        { label: '5xx', value: formatNumber(o.s5xx) },
        { label: i18n.global.t('website.guard.avgRt'), value: Math.round(o.avgRt) + ' ms' },
        { label: 'P95', value: Math.round(o.p95Rt) + ' ms' },
        { label: i18n.global.t('website.guard.blocked'), value: formatNumber(o.blocked) },
    ];
});

const topTables = computed(() => [
    { key: 'url', label: i18n.global.t('website.guard.topUrl'), column: 'URL' },
    { key: 'ip', label: i18n.global.t('website.guard.topIp'), column: 'IP' },
    { key: 'referer', label: i18n.global.t('website.guard.topReferer'), column: 'Referer' },
    {
        key: 'country',
        label: i18n.global.t('website.guard.topCountry'),
        column: i18n.global.t('website.guard.country'),
    },
    { key: 'ua', label: i18n.global.t('website.guard.topUa'), column: 'User-Agent' },
    { key: 'errUrl', label: i18n.global.t('website.guard.topErr'), column: 'URL' },
]);

const countryName = (code: string) => {
    if (code === 'LAN') {
        return i18n.global.t('website.guard.lan');
    }
    return countries.value[code] ? `${countries.value[code]} (${code})` : code;
};

const pad = (n: number) => String(n).padStart(2, '0');
const axisLabel = (ts: number, interval: number) => {
    const d = new Date(ts * 1000);
    const day = `${pad(d.getMonth() + 1)}-${pad(d.getDate())}`;
    const time = `${pad(d.getHours())}:${pad(d.getMinutes())}`;
    if (interval >= 86400) {
        return day;
    }
    return interval >= 3600 ? `${day} ${time}` : time;
};

const buildCharts = (trend: WebsiteGuard.MonitorTrend) => {
    const xData = trend.points.map((p) => axisLabel(p.time, trend.interval));
    const grid = { left: '6%', right: '4%', bottom: '12%', top: '15%', containLabel: true };
    charts.requests = {
        xData,
        yData: [
            { name: i18n.global.t('website.guard.requests'), data: trend.points.map((p) => p.requests) },
            { name: 'UV', data: trend.points.map((p) => p.uv) },
            { name: i18n.global.t('website.guard.blocked'), data: trend.points.map((p) => p.blocked) },
        ],
        yAxis: [{ type: 'value' }],
        formatStr: '',
        grid,
    };
    charts.traffic = {
        xData,
        yData: [
            {
                name: i18n.global.t('website.guard.trafficOut'),
                data: trend.points.map((p) => +(p.bytesOut / 1024).toFixed(2)),
            },
            {
                name: i18n.global.t('website.guard.trafficIn'),
                data: trend.points.map((p) => +(p.bytesIn / 1024).toFixed(2)),
            },
        ],
        yAxis: [{ type: 'value', axisLabel: { formatter: (v: number) => computeSize(v * 1024) } }],
        formatStr: 'KB',
        grid,
    };
    charts.status = {
        xData,
        yData: ['s2xx', 's3xx', 's4xx', 's5xx'].map((k) => ({
            name: k.slice(1),
            data: trend.points.map((p) => p[k]),
        })),
        yAxis: [{ type: 'value' }],
        formatStr: '',
        grid,
    };
    charts.rt = {
        xData,
        yData: [
            { name: i18n.global.t('website.guard.avgRt'), data: trend.points.map((p) => Math.round(p.avgRt)) },
            { name: 'P95', data: trend.points.map((p) => Math.round(p.p95Rt)) },
        ],
        yAxis: [{ type: 'value', axisLabel: { formatter: '{value} ms' } }],
        formatStr: ' ms',
        grid,
    };
};

const loadSlow = async () => {
    const r = timeRange();
    const res = await searchMonitorSlow({ ...r, page: slowPage.currentPage, pageSize: slowPage.pageSize });
    slow.value = res.data.items || [];
    slowPage.total = res.data.total;
};

const search = async () => {
    loading.value = true;
    try {
        const req = timeRange();
        const [o, t, tp] = await Promise.all([loadMonitorOverview(req), loadMonitorTrend(req), loadMonitorTop(req)]);
        overview.value = o.data;
        buildCharts(t.data);
        top.value = tp.data;
        slowPage.currentPage = 1;
        await loadSlow();
    } finally {
        loading.value = false;
    }
};

const loadStatus = async () => {
    const res = await getGuardStatus();
    status.value = res.data;
    statusLoaded.value = true;
    pendingSite.value = undefined;
    if (site.value && !enabledSites.value.find((s) => s.alias === site.value)) {
        pendingSite.value = sites.value.find((s) => s.alias === site.value);
        site.value = '';
    }
};

const openEnable = (id?: number) => {
    enableIds.value = enabledSites.value.map((s) => s.websiteId);
    if (id) {
        enableIds.value.push(id);
    }
    enableOpen.value = true;
};

// Monitoring rides on the guard hooks: enable the guard for the chosen sites
// with every protection off, so only the access log is collected.
const submitEnable = async () => {
    enabling.value = true;
    try {
        const cfg = status.value!.config;
        if (!cfg.enable) {
            await updateGuardGlobal({ ...cfg, enable: true });
        }
        for (const id of newIds.value) {
            const res = await getGuardSite(id);
            await updateGuardSite({
                ...res.data,
                websiteId: id,
                enable: true,
                mode: 'off',
                ccEnable: false,
                ipAllow: [],
                ipDeny: [],
                geoMode: 'none',
                countries: [],
            });
        }
        MsgSuccess(i18n.global.t('website.guard.monitorEnabled'));
        enableOpen.value = false;
        const target = sites.value.find((s) => s.websiteId === newIds.value[0]);
        await loadStatus();
        if (target && enabledSites.value.length === 1) {
            site.value = target.alias;
        }
        search();
    } finally {
        enabling.value = false;
    }
};

const onAutoRefresh = () => {
    clearInterval(refreshTimer);
    refreshTimer = undefined;
    if (autoRefresh.value) {
        refreshTimer = setInterval(search, 60 * 1000);
    }
};

onMounted(async () => {
    await Promise.all([
        loadStatus(),
        loadGuardCountries()
            .then((res) => {
                const zh = globalStore.language.startsWith('zh');
                for (const c of res.data || []) {
                    countries.value[c.code] = zh ? c.zh || c.en : c.en || c.zh;
                }
            })
            .catch(() => {}),
    ]);
    if (enabledSites.value.length > 0) {
        search();
    }
});

onUnmounted(() => {
    clearInterval(refreshTimer);
});
</script>

<style scoped lang="scss">
.stat-card {
    :deep(.el-card__body) {
        padding: 14px 16px;
    }
}
.stat-label {
    font-size: 12px;
    color: var(--el-text-color-secondary);
}
.stat-value {
    margin-top: 6px;
    font-size: 20px;
    font-weight: 600;
    color: var(--el-text-color-primary);
    font-variant-numeric: tabular-nums;
}
</style>

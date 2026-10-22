<template>
    <div v-loading="loading">
        <div class="flex flex-wrap items-center gap-3 mb-4">
            <el-select v-model="filters.site" class="p-w-200" @change="search">
                <el-option :label="$t('website.guard.allSites')" value="" />
                <el-option v-for="item in sites" :key="item.alias" :label="item.primaryDomain" :value="item.alias" />
            </el-select>
            <el-select v-model="filters.type" class="p-w-150" @change="search">
                <el-option :label="$t('website.guard.allTypes')" value="" />
                <el-option v-for="t in types" :key="t" :label="typeLabel(t)" :value="t" />
            </el-select>
            <el-input v-model.trim="filters.ip" class="p-w-200" placeholder="IP" clearable @change="search" />
            <el-radio-group v-model="range" @change="search">
                <el-radio-button value="24h">{{ $t('website.guard.range24h') }}</el-radio-button>
                <el-radio-button value="7d">{{ $t('website.guard.range7d') }}</el-radio-button>
                <el-radio-button value="30d">{{ $t('website.guard.range30d') }}</el-radio-button>
            </el-radio-group>
            <el-button :icon="Refresh" circle @click="search" />
        </div>

        <el-row :gutter="12" class="mb-2">
            <el-col :xs="12" :sm="8" :md="4" class="mb-3">
                <el-card shadow="never" class="stat-card">
                    <div class="stat-label">{{ $t('website.guard.eventTotal') }}</div>
                    <div class="stat-value">{{ stats.total || 0 }}</div>
                </el-card>
            </el-col>
            <el-col v-for="t in types" :key="t" :xs="12" :sm="8" :md="4" class="mb-3">
                <el-card shadow="never" class="stat-card">
                    <div class="stat-label">{{ typeLabel(t) }}</div>
                    <div class="stat-value">{{ stats.byType?.[t] || 0 }}</div>
                </el-card>
            </el-col>
        </el-row>

        <el-row :gutter="12">
            <el-col :xs="24" :lg="16" class="mb-3">
                <el-card shadow="never">
                    <template #header>{{ $t('website.guard.eventTrend') }}</template>
                    <v-charts height="240px" :option="trendChart" v-if="trendChart" />
                </el-card>
            </el-col>
            <el-col :xs="24" :lg="8" class="mb-3">
                <el-card shadow="never">
                    <template #header>{{ $t('website.guard.topAttackIp') }}</template>
                    <el-table :data="stats.topIP || []" size="small" max-height="240">
                        <el-table-column label="IP" prop="name" />
                        <el-table-column :label="$t('website.guard.count')" prop="count" width="90" />
                    </el-table>
                </el-card>
            </el-col>
        </el-row>

        <el-card shadow="never">
            <ComplexTable :data="events" :pagination-config="page" @search="loadEvents">
                <el-table-column :label="$t('commons.table.date')" width="170">
                    <template #default="{ row }">{{ formatTime(row.time) }}</template>
                </el-table-column>
                <el-table-column :label="$t('website.guard.site')" prop="site" width="130" show-overflow-tooltip />
                <el-table-column :label="$t('website.guard.eventType')" width="110">
                    <template #default="{ row }">
                        <el-tag size="small" :type="row.type === 'attack' ? 'danger' : 'warning'">
                            {{ typeLabel(row.type) }}
                        </el-tag>
                    </template>
                </el-table-column>
                <el-table-column :label="$t('website.guard.action')" width="90">
                    <template #default="{ row }">
                        {{ row.action === 'block' ? $t('website.guard.modeBlock') : $t('website.guard.modeMonitor') }}
                    </template>
                </el-table-column>
                <el-table-column label="IP" width="150">
                    <template #default="{ row }">
                        <el-link type="primary" @click="filterIP(row.ip)">{{ row.ip }}</el-link>
                        <span v-if="row.country" class="input-help !mt-0 ml-1">{{ row.country }}</span>
                    </template>
                </el-table-column>
                <el-table-column label="URI" min-width="260" show-overflow-tooltip>
                    <template #default="{ row }">{{ row.method }} {{ row.host }}{{ row.uri }}</template>
                </el-table-column>
                <el-table-column label="User-Agent" prop="ua" min-width="160" show-overflow-tooltip />
                <el-table-column
                    :label="$t('website.guard.eventId')"
                    prop="eventId"
                    width="140"
                    show-overflow-tooltip
                />
            </ComplexTable>
        </el-card>
    </div>
</template>

<script lang="ts" setup>
import { Refresh } from '@element-plus/icons-vue';
import { WebsiteGuard } from '@/api/interface/website-guard';
import { getGuardStatus, loadGuardEventStats, searchGuardEvents } from '@/api/modules/website-guard';
import { dateFormat } from '@/utils/date';
import i18n from '@/lang';

const loading = ref(false);
const range = ref('24h');
const sites = ref<WebsiteGuard.SiteBrief[]>([]);
const filters = reactive({ site: '', type: '', ip: '' });
const events = ref<WebsiteGuard.Event[]>([]);
const stats = ref<Partial<WebsiteGuard.EventStats>>({});
const trendChart = ref<any>();
const page = reactive({ currentPage: 1, pageSize: 20, total: 0 });
const types = ['attack', 'cc', 'ban', 'ipDeny', 'geo'];

const rangeSeconds: Record<string, number> = { '24h': 86400, '7d': 7 * 86400, '30d': 30 * 86400 };

const timeRange = () => {
    const end = Math.floor(Date.now() / 1000);
    return { startTime: end - rangeSeconds[range.value], endTime: end };
};

const typeLabel = (t: string) => i18n.global.t('website.guard.type_' + t);
const formatTime = (ts: number) => dateFormat(0, 0, ts * 1000);

const loadEvents = async () => {
    const res = await searchGuardEvents({
        ...filters,
        ...timeRange(),
        page: page.currentPage,
        pageSize: page.pageSize,
    });
    events.value = res.data.items || [];
    page.total = res.data.total;
};

const search = async () => {
    loading.value = true;
    try {
        const res = await loadGuardEventStats({ site: filters.site, ...timeRange() });
        stats.value = res.data;
        const daily = range.value !== '24h';
        trendChart.value = {
            xData: (res.data.trend || []).map((p) => {
                const d = new Date(p.name);
                const md = `${d.getMonth() + 1}-${d.getDate()}`;
                return daily ? md : `${md} ${String(d.getHours()).padStart(2, '0')}:00`;
            }),
            yData: [
                { name: i18n.global.t('website.guard.eventTotal'), data: (res.data.trend || []).map((p) => p.count) },
            ],
            yAxis: [{ type: 'value', minInterval: 1 }],
            formatStr: '',
            grid: { left: '6%', right: '4%', bottom: '12%', top: '15%', containLabel: true },
        };
        page.currentPage = 1;
        await loadEvents();
    } finally {
        loading.value = false;
    }
};

const filterIP = (ip: string) => {
    filters.ip = ip;
    search();
};

onMounted(async () => {
    const res = await getGuardStatus();
    sites.value = res.data.sites || [];
    search();
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

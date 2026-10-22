<template>
    <div v-loading="loading">
        <el-alert v-if="!status.openrestyInstalled" type="warning" :closable="false" class="common-prompt">
            {{ $t('website.guard.noOpenresty') }}
        </el-alert>
        <el-alert v-if="status.wafConflict" type="error" :closable="false" class="common-prompt">
            {{ $t('website.guard.wafConflict') }}
        </el-alert>

        <el-row :gutter="20">
            <el-col :xs="24" :lg="10">
                <el-card shadow="never" class="mb-4">
                    <template #header>
                        <div class="flex items-center justify-between">
                            <span>{{ $t('website.guard.detector') }}</span>
                            <Status :status="containerStatus(status.detectorStatus)" v-if="status.detectorInstalled" />
                        </div>
                    </template>
                    <p class="input-help !mt-0">{{ $t('website.guard.detectorHelper') }}</p>
                    <div class="text-sm mb-3 flex flex-col gap-1">
                        <span>
                            {{ $t('website.guard.detectorImage') }}:
                            <span class="font-mono">SafeLine CE {{ status.safelineVersion }}</span>
                        </span>
                        <span v-if="status.detectorInstalled">
                            {{ $t('website.guard.mgtStatus') }}:
                            <Status :status="containerStatus(status.mgtStatus)" />
                        </span>
                        <span v-if="status.detectorInstalled && status.consolePort">
                            {{ $t('website.guard.console') }}:
                            <el-link type="primary" :href="consoleUrl" target="_blank">{{ consoleUrl }}</el-link>
                        </span>
                    </div>
                    <div class="flex flex-wrap gap-2">
                        <template v-if="!status.detectorInstalled">
                            <el-input-number
                                v-model="consolePort"
                                :min="1"
                                :max="65535"
                                controls-position="right"
                                class="!w-40"
                            />
                            <el-button
                                v-permission
                                type="primary"
                                :disabled="!status.openrestyInstalled"
                                @click="operate('install')"
                            >
                                {{ $t('commons.button.install') }}
                            </el-button>
                        </template>
                        <template v-else>
                            <el-button
                                v-permission
                                v-if="status.detectorStatus !== 'running'"
                                @click="operate('start')"
                            >
                                {{ $t('commons.button.start') }}
                            </el-button>
                            <el-button v-permission v-else @click="operate('stop')">
                                {{ $t('commons.button.stop') }}
                            </el-button>
                            <el-button v-permission @click="operate('restart')">
                                {{ $t('commons.button.restart') }}
                            </el-button>
                            <el-button v-permission @click="operate('resetAdmin')">
                                {{ $t('website.guard.resetAdmin') }}
                            </el-button>
                            <el-button v-permission type="danger" plain @click="operate('remove')">
                                {{ $t('commons.button.uninstall') }}
                            </el-button>
                        </template>
                    </div>
                    <p class="input-help" v-if="!status.detectorInstalled">
                        {{ $t('website.guard.consolePortHelper') }}
                    </p>
                    <p class="input-help" v-if="!status.detectorInstalled">
                        {{ $t('website.guard.detectorOptional') }}
                    </p>
                </el-card>

                <el-card shadow="never">
                    <template #header>{{ $t('website.guard.sites') }}</template>
                    <el-table :data="status.sites" size="small" max-height="420">
                        <el-table-column
                            :label="$t('website.primaryDomain')"
                            prop="primaryDomain"
                            show-overflow-tooltip
                        />
                        <el-table-column :label="$t('commons.table.status')" width="100">
                            <template #default="{ row }">
                                <el-tag :type="row.enable ? 'success' : 'info'" size="small">
                                    {{ row.enable ? $t('commons.status.enabled') : $t('commons.status.disabled') }}
                                </el-tag>
                            </template>
                        </el-table-column>
                        <el-table-column :label="$t('website.guard.attackMode')" width="100">
                            <template #default="{ row }">
                                {{ row.enable ? modeLabel(row.mode) : '-' }}
                            </template>
                        </el-table-column>
                        <el-table-column :label="$t('commons.table.operate')" width="90">
                            <template #default="{ row }">
                                <el-button link type="primary" @click="openSite(row)">
                                    {{ $t('commons.button.set') }}
                                </el-button>
                            </template>
                        </el-table-column>
                    </el-table>
                </el-card>
            </el-col>

            <el-col :xs="24" :lg="14">
                <el-card shadow="never">
                    <template #header>{{ $t('website.guard.globalConfig') }}</template>
                    <el-form :model="config" label-position="top" @submit.prevent>
                        <el-form-item :label="$t('website.guard.globalEnable')">
                            <el-switch
                                v-model="config.enable"
                                :disabled="!status.openrestyInstalled || (status.wafConflict && !config.enable)"
                            />
                            <span class="input-help">{{ $t('website.guard.globalEnableHelper') }}</span>
                        </el-form-item>

                        <el-divider content-position="left">{{ $t('website.guard.siteDefaults') }}</el-divider>
                        <el-form-item :label="$t('website.guard.attackMode')">
                            <el-radio-group v-model="config.mode">
                                <el-radio-button value="block">{{ $t('website.guard.modeBlock') }}</el-radio-button>
                                <el-radio-button value="monitor">
                                    {{ $t('website.guard.modeMonitor') }}
                                </el-radio-button>
                                <el-radio-button value="off">{{ $t('website.guard.modeOff') }}</el-radio-button>
                            </el-radio-group>
                        </el-form-item>
                        <el-form-item :label="$t('website.guard.cc')">
                            <div class="flex flex-wrap items-center gap-2">
                                <el-switch v-model="config.ccEnable" />
                                <template v-if="config.ccEnable">
                                    <el-input-number
                                        v-model="config.ccDuration"
                                        :min="1"
                                        :max="3600"
                                        controls-position="right"
                                    />
                                    <span>{{ $t('website.guard.ccWithin') }}</span>
                                    <el-input-number
                                        v-model="config.ccThreshold"
                                        :min="1"
                                        :max="1000000"
                                        controls-position="right"
                                    />
                                    <span>{{ $t('website.guard.ccRequests') }}</span>
                                    <el-input-number
                                        v-model="config.ccBanTime"
                                        :min="0"
                                        :max="86400"
                                        controls-position="right"
                                    />
                                    <span>{{ $t('website.guard.ccBanSeconds') }}</span>
                                </template>
                            </div>
                        </el-form-item>

                        <el-divider content-position="left">{{ $t('website.guard.globalRules') }}</el-divider>
                        <el-form-item :label="$t('website.guard.ipAllow')">
                            <el-input
                                v-model="ipAllow"
                                type="textarea"
                                :rows="3"
                                :placeholder="$t('website.guard.ipHelper')"
                            />
                        </el-form-item>
                        <el-form-item :label="$t('website.guard.ipDeny')">
                            <el-input
                                v-model="ipDeny"
                                type="textarea"
                                :rows="3"
                                :placeholder="$t('website.guard.ipHelper')"
                            />
                        </el-form-item>
                        <el-form-item :label="$t('website.guard.geo')">
                            <el-radio-group v-model="config.geoMode">
                                <el-radio-button value="none">{{ $t('website.guard.geoOff') }}</el-radio-button>
                                <el-radio-button value="deny">{{ $t('website.guard.geoDeny') }}</el-radio-button>
                                <el-radio-button value="allow">{{ $t('website.guard.geoAllow') }}</el-radio-button>
                            </el-radio-group>
                        </el-form-item>
                        <el-form-item v-if="config.geoMode !== 'none'">
                            <CountrySelect v-model="config.countries" />
                            <span class="input-help">{{ $t('website.guard.geoHelper') }}</span>
                        </el-form-item>

                        <el-divider content-position="left">{{ $t('website.guard.monitorSetting') }}</el-divider>
                        <el-form-item :label="$t('website.guard.slowThreshold')">
                            <el-input-number
                                v-model="config.slowThreshold"
                                :min="100"
                                :max="60000"
                                :step="100"
                                controls-position="right"
                            />
                            <span class="ml-2">ms</span>
                        </el-form-item>
                        <el-form-item :label="$t('website.guard.retention')">
                            <div class="flex flex-wrap items-center gap-2">
                                <span>{{ $t('website.guard.minuteDays') }}</span>
                                <el-input-number
                                    v-model="config.minuteDays"
                                    :min="1"
                                    :max="30"
                                    controls-position="right"
                                />
                                <span>{{ $t('website.guard.dayDays') }}</span>
                                <el-input-number
                                    v-model="config.dayDays"
                                    :min="1"
                                    :max="365"
                                    controls-position="right"
                                />
                                <span>{{ $t('website.guard.eventDays') }}</span>
                                <el-input-number
                                    v-model="config.eventDays"
                                    :min="1"
                                    :max="365"
                                    controls-position="right"
                                />
                            </div>
                        </el-form-item>
                        <el-form-item>
                            <el-button v-permission type="primary" :loading="saving" @click="save">
                                {{ $t('commons.button.save') }}
                            </el-button>
                        </el-form-item>
                    </el-form>
                </el-card>
            </el-col>
        </el-row>

        <DrawerPro v-model="siteOpen" :header="siteTitle" size="large" @close="load">
            <SiteGuard v-if="siteOpen" :id="siteId" @saved="load" />
        </DrawerPro>
        <TaskLog ref="taskLogRef" @close="load" />
    </div>
</template>

<script lang="ts" setup>
import { WebsiteGuard } from '@/api/interface/website-guard';
import { getGuardStatus, operateGuardDetector, updateGuardGlobal } from '@/api/modules/website-guard';
import CountrySelect from '@/views/website/guard/country-select/index.vue';
import SiteGuard from '@/views/website/guard/site/index.vue';
import TaskLog from '@/components/log/task/index.vue';
import Status from '@/components/status/index.vue';
import { MsgSuccess } from '@/utils/message';
import { newUUID } from '@/utils/id';
import i18n from '@/lang';

const loading = ref(false);
const saving = ref(false);
const taskLogRef = ref();
const siteOpen = ref(false);
const siteId = ref(0);
const siteTitle = ref('');
const status = ref<WebsiteGuard.Status>({
    openrestyInstalled: false,
    openrestyRunning: false,
    wafConflict: false,
    detectorInstalled: false,
    detectorStatus: '',
    mgtStatus: '',
    consolePort: 0,
    safelineVersion: '',
    config: {} as WebsiteGuard.GlobalConfig,
    sites: [],
});
const config = ref<WebsiteGuard.GlobalConfig>({} as WebsiteGuard.GlobalConfig);
const ipAllow = ref('');
const ipDeny = ref('');

const consolePort = ref(9443);
const consoleUrl = computed(() => `https://${window.location.hostname}:${status.value.consolePort}`);

const containerStatus = (state: string) => {
    switch (state) {
        case 'running':
            return 'Running';
        case 'restarting':
            return 'Restarting';
        default:
            return 'Stopped';
    }
};

const modeLabel = (mode: string) => {
    switch (mode) {
        case 'block':
            return i18n.global.t('website.guard.modeBlock');
        case 'monitor':
            return i18n.global.t('website.guard.modeMonitor');
        default:
            return i18n.global.t('website.guard.modeOff');
    }
};

const splitLines = (value: string) =>
    value
        .split(/[\n,]/)
        .map((item) => item.trim())
        .filter((item) => item !== '');

const load = async () => {
    loading.value = true;
    try {
        const res = await getGuardStatus();
        status.value = res.data;
        config.value = { ...res.data.config, countries: res.data.config.countries || [] };
        ipAllow.value = (res.data.config.ipAllow || []).join('\n');
        ipDeny.value = (res.data.config.ipDeny || []).join('\n');
    } finally {
        loading.value = false;
    }
};

const save = async () => {
    saving.value = true;
    try {
        await updateGuardGlobal({
            ...config.value,
            ipAllow: splitLines(ipAllow.value),
            ipDeny: splitLines(ipDeny.value),
        });
        MsgSuccess(i18n.global.t('commons.msg.updateSuccess'));
        await load();
    } finally {
        saving.value = false;
    }
};

const operate = async (op: string) => {
    if (op === 'remove') {
        await ElMessageBox.confirm(
            i18n.global.t('website.guard.removeDetectorHelper'),
            i18n.global.t('commons.button.uninstall'),
            {
                confirmButtonText: i18n.global.t('commons.button.confirm'),
                cancelButtonText: i18n.global.t('commons.button.cancel'),
                type: 'warning',
            },
        );
    }
    const taskID = newUUID();
    loading.value = true;
    try {
        const res = await operateGuardDetector({ operate: op, taskID, consolePort: consolePort.value });
        if (op === 'install') {
            taskLogRef.value.openWithTaskID(taskID);
            return;
        }
        if (op === 'resetAdmin') {
            const output = String(res.data || '')
                .replace(/&/g, '&amp;')
                .replace(/</g, '&lt;')
                .replace(/>/g, '&gt;');
            ElMessageBox.alert(
                `<pre class="whitespace-pre-wrap">${output}</pre>`,
                i18n.global.t('website.guard.resetAdmin'),
                {
                    dangerouslyUseHTMLString: true,
                    confirmButtonText: i18n.global.t('commons.button.confirm'),
                },
            );
            return;
        }
        MsgSuccess(i18n.global.t('commons.msg.operationSuccess'));
        await load();
    } finally {
        loading.value = false;
    }
};

const openSite = (row: WebsiteGuard.SiteBrief) => {
    siteId.value = row.websiteId;
    siteTitle.value = `${i18n.global.t('website.guard.siteConfig')} - ${row.primaryDomain}`;
    siteOpen.value = true;
};

onMounted(() => {
    load();
});
</script>

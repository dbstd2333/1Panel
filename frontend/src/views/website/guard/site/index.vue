<template>
    <div v-loading="loading">
        <el-alert v-if="!globalEnable" type="warning" :closable="false" class="common-prompt">
            {{ $t('website.guard.globalDisabledHelper') }}
            <el-link type="primary" class="ml-2" @click="goGuard">{{ $t('website.guard.goGuard') }}</el-link>
        </el-alert>
        <el-form ref="formRef" :model="form" label-position="top" @submit.prevent>
            <el-form-item :label="$t('website.guard.siteEnable')">
                <el-switch v-model="form.enable" :disabled="!globalEnable && !form.enable" />
                <span class="input-help">{{ $t('website.guard.siteEnableHelper') }}</span>
            </el-form-item>

            <el-form-item :label="$t('website.guard.attackMode')">
                <el-radio-group v-model="form.mode">
                    <el-radio-button value="block">{{ $t('website.guard.modeBlock') }}</el-radio-button>
                    <el-radio-button value="monitor">{{ $t('website.guard.modeMonitor') }}</el-radio-button>
                    <el-radio-button value="off">{{ $t('website.guard.modeOff') }}</el-radio-button>
                </el-radio-group>
                <span class="input-help">{{ $t('website.guard.attackModeHelper') }}</span>
            </el-form-item>

            <el-form-item :label="$t('website.guard.cc')">
                <el-switch v-model="form.ccEnable" />
            </el-form-item>
            <template v-if="form.ccEnable">
                <el-form-item>
                    <div class="flex flex-wrap items-center gap-2">
                        <el-input-number v-model="form.ccDuration" :min="1" :max="3600" controls-position="right" />
                        <span>{{ $t('website.guard.ccWithin') }}</span>
                        <el-input-number v-model="form.ccThreshold" :min="1" :max="1000000" controls-position="right" />
                        <span>{{ $t('website.guard.ccRequests') }}</span>
                        <el-input-number v-model="form.ccBanTime" :min="0" :max="86400" controls-position="right" />
                        <span>{{ $t('website.guard.ccBanSeconds') }}</span>
                    </div>
                    <span class="input-help">{{ $t('website.guard.ccHelper') }}</span>
                </el-form-item>
            </template>

            <el-form-item :label="$t('website.guard.ipAllow')">
                <el-input v-model="ipAllow" type="textarea" :rows="4" :placeholder="$t('website.guard.ipHelper')" />
            </el-form-item>
            <el-form-item :label="$t('website.guard.ipDeny')">
                <el-input v-model="ipDeny" type="textarea" :rows="4" :placeholder="$t('website.guard.ipHelper')" />
                <span class="input-help">{{ $t('website.guard.ipSiteHelper') }}</span>
            </el-form-item>

            <el-form-item :label="$t('website.guard.geo')">
                <el-radio-group v-model="form.geoMode">
                    <el-radio-button value="none">{{ $t('website.guard.geoInherit') }}</el-radio-button>
                    <el-radio-button value="deny">{{ $t('website.guard.geoDeny') }}</el-radio-button>
                    <el-radio-button value="allow">{{ $t('website.guard.geoAllow') }}</el-radio-button>
                </el-radio-group>
            </el-form-item>
            <el-form-item v-if="form.geoMode !== 'none'">
                <CountrySelect v-model="form.countries" />
                <span class="input-help">{{ $t('website.guard.geoHelper') }}</span>
            </el-form-item>

            <el-form-item>
                <el-button v-permission type="primary" :loading="loading" @click="submit">
                    {{ $t('commons.button.save') }}
                </el-button>
            </el-form-item>
        </el-form>
    </div>
</template>

<script lang="ts" setup>
import { WebsiteGuard } from '@/api/interface/website-guard';
import { getGuardSite, getGuardStatus, updateGuardSite } from '@/api/modules/website-guard';
import CountrySelect from '@/views/website/guard/country-select/index.vue';
import { MsgSuccess } from '@/utils/message';
import { routerToName } from '@/utils/router';
import i18n from '@/lang';

const props = defineProps({
    id: {
        type: Number,
        default: 0,
    },
});
const emit = defineEmits(['saved']);

const loading = ref(false);
const globalEnable = ref(true);
const form = ref<WebsiteGuard.SiteConfig>({
    websiteId: 0,
    enable: false,
    mode: 'block',
    ccEnable: true,
    ccDuration: 60,
    ccThreshold: 300,
    ccBanTime: 600,
    ipAllow: [],
    ipDeny: [],
    geoMode: 'none',
    countries: [],
});
const ipAllow = ref('');
const ipDeny = ref('');

const splitLines = (value: string) =>
    value
        .split(/[\n,]/)
        .map((item) => item.trim())
        .filter((item) => item !== '');

const load = async () => {
    loading.value = true;
    try {
        const [site, status] = await Promise.all([getGuardSite(props.id), getGuardStatus()]);
        form.value = site.data;
        form.value.geoMode = form.value.geoMode || 'none';
        ipAllow.value = (site.data.ipAllow || []).join('\n');
        ipDeny.value = (site.data.ipDeny || []).join('\n');
        globalEnable.value = status.data.config.enable;
    } finally {
        loading.value = false;
    }
};

const submit = async () => {
    loading.value = true;
    try {
        await updateGuardSite({
            ...form.value,
            websiteId: props.id,
            ipAllow: splitLines(ipAllow.value),
            ipDeny: splitLines(ipDeny.value),
        });
        MsgSuccess(i18n.global.t('commons.msg.updateSuccess'));
        emit('saved');
        await load();
    } finally {
        loading.value = false;
    }
};

const goGuard = () => {
    routerToName('WebsiteGuard');
};

onMounted(() => {
    load();
});
</script>

<template>
    <div>
        <el-form-item :label="$t('app.tuning')">
            <el-radio-group v-model="params[keys.template]" @change="loadPreview">
                <el-radio-button value="tiny">
                    {{ $t('app.tuningTiny') }}
                    <span v-if="preview?.recommend === 'tiny'">({{ $t('app.tuningRecommend') }})</span>
                </el-radio-button>
                <el-radio-button value="oltp">
                    {{ $t('app.tuningOltp') }}
                    <span v-if="preview?.recommend === 'oltp'">({{ $t('app.tuningRecommend') }})</span>
                </el-radio-button>
                <el-radio-button value="none">{{ $t('app.tuningNone') }}</el-radio-button>
            </el-radio-group>
            <span class="input-help" v-if="preview">
                {{ $t('app.tuningHelper', [preview.cpu, computeSizeFromMB(preview.memMB), preview.diskGB]) }}
            </span>
        </el-form-item>
        <el-form-item v-if="preview?.items?.length">
            <el-collapse class="w-full">
                <el-collapse-item :title="$t('app.tuningPreview', [preview.items.length])" name="preview">
                    <el-table :data="preview.items" size="small" max-height="300">
                        <el-table-column prop="key" :label="$t('app.tuningParam')" />
                        <el-table-column prop="value" :label="$t('app.tuningValue')" />
                    </el-table>
                </el-collapse-item>
            </el-collapse>
        </el-form-item>

        <template v-if="appKey === 'postgresql'">
            <el-form-item>
                <el-checkbox v-model="pgBouncer" :label="$t('app.pgBouncer')" size="large" @change="changePgBouncer" />
                <span class="input-help">{{ $t('app.pgBouncerHelper') }}</span>
            </el-form-item>
            <template v-if="pgBouncer">
                <el-form-item
                    :label="$t('app.pgBouncerPort')"
                    :prop="propStart + keys.port"
                    :rules="[Rules.requiredInput, Rules.port]"
                >
                    <el-input v-model.number="params[keys.port]" class="p-w-300" />
                </el-form-item>
                <el-form-item :label="$t('app.pgBouncerPoolMode')">
                    <el-select v-model="params[keys.poolMode]" class="p-w-300">
                        <el-option label="transaction" value="transaction" />
                        <el-option label="session" value="session" />
                    </el-select>
                    <span class="input-help">{{ $t('app.pgBouncerPoolModeHelper') }}</span>
                </el-form-item>
            </template>
        </template>
    </div>
</template>

<script lang="ts" setup>
import { App } from '@/api/interface/app';
import { loadAppTuningPreview } from '@/api/modules/app';
import { Rules } from '@/global/form-rules';
import { computeSizeFromMB } from '@/utils/size';

const props = defineProps<{
    appKey: string;
    version: string;
    params: Record<string, any>;
    advanced: boolean;
    cpuQuota: number;
    memoryLimit: number;
    memoryUnit: string;
    propStart?: string;
}>();

const keys = {
    template: 'PANEL_TUNE_TEMPLATE',
    enable: 'PANEL_PGBOUNCER_ENABLE',
    port: 'PANEL_APP_PORT_PGBOUNCER',
    poolMode: 'PANEL_PGBOUNCER_POOL_MODE',
};
const propStart = computed(() => props.propStart || 'params.');
const params = computed(() => props.params);
const preview = ref<App.TuningPreview>();
const pgBouncer = ref(false);
let timer: ReturnType<typeof setTimeout> | undefined;

const loadPreview = async () => {
    const res = await loadAppTuningPreview({
        key: props.appKey,
        version: props.version,
        template: params.value[keys.template] || '',
        pgBouncer: pgBouncer.value,
        advanced: props.advanced,
        cpuQuota: Number(props.cpuQuota) || 0,
        memoryLimit: Number(props.memoryLimit) || 0,
        memoryUnit: props.memoryUnit,
    });
    preview.value = res.data;
    if (!params.value[keys.template]) {
        params.value[keys.template] = res.data.template;
    }
};

const changePgBouncer = () => {
    params.value[keys.enable] = pgBouncer.value;
    if (pgBouncer.value) {
        params.value[keys.port] = params.value[keys.port] || 6432;
        params.value[keys.poolMode] = params.value[keys.poolMode] || 'transaction';
    } else {
        delete params.value[keys.port];
        delete params.value[keys.poolMode];
    }
    loadPreview();
};

watch(
    () => [props.advanced, props.cpuQuota, props.memoryLimit, props.memoryUnit],
    () => {
        clearTimeout(timer);
        timer = setTimeout(loadPreview, 500);
    },
);

onMounted(() => {
    pgBouncer.value = !!params.value[keys.enable];
    loadPreview();
});

onBeforeUnmount(() => clearTimeout(timer));
</script>

<template>
    <el-select
        v-model="value"
        multiple
        filterable
        collapse-tags
        collapse-tags-tooltip
        :max-collapse-tags="8"
        :loading="loading"
        class="w-full"
        :placeholder="$t('website.guard.countryPlaceholder')"
    >
        <el-option v-for="item in countries" :key="item.code" :label="label(item)" :value="item.code" />
    </el-select>
</template>

<script lang="ts" setup>
import { WebsiteGuard } from '@/api/interface/website-guard';
import { loadGuardCountries } from '@/api/modules/website-guard';
import { useGlobalStore } from '@/composables/useGlobalStore';

const value = defineModel<string[]>({ default: () => [] });
const { globalStore } = useGlobalStore();
const countries = ref<WebsiteGuard.Country[]>([]);
const loading = ref(false);

const label = (item: WebsiteGuard.Country) => {
    const name = globalStore.language.startsWith('zh') ? item.zh || item.en : item.en || item.zh;
    return `${name} (${item.code})`;
};

onMounted(async () => {
    loading.value = true;
    try {
        const res = await loadGuardCountries();
        countries.value = res.data || [];
    } finally {
        loading.value = false;
    }
});
</script>

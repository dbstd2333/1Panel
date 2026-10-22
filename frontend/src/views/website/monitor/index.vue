<template>
    <div>
        <RouterButton :buttons="routerButton" />
        <LayoutContent :title="$t('menu.websiteMonitor')">
            <template #main>
                <Monitor />
            </template>
        </LayoutContent>
    </div>
</template>

<script lang="ts" setup>
import Monitor from '@/views/website/guard/monitor/index.vue';
import i18n from '@/lang';

defineOptions({ name: 'WebsiteMonitor' });

const routerButton = [
    {
        label: i18n.global.t('menu.websiteMonitor'),
        path: '/websites/monitor',
    },
];
</script>

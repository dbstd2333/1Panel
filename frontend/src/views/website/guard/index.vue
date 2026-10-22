<template>
    <div>
        <RouterButton :buttons="routerButton" />
        <LayoutContent :title="$t('menu.websiteGuard')">
            <template #main>
                <el-tabs v-model="tab">
                    <el-tab-pane :label="$t('website.guard.tabOverview')" name="overview">
                        <Overview v-if="tab === 'overview'" />
                    </el-tab-pane>
                    <el-tab-pane :label="$t('website.guard.tabMonitor')" name="monitor">
                        <Monitor v-if="tab === 'monitor'" />
                    </el-tab-pane>
                    <el-tab-pane :label="$t('website.guard.tabEvents')" name="events">
                        <Events v-if="tab === 'events'" />
                    </el-tab-pane>
                </el-tabs>
            </template>
        </LayoutContent>
    </div>
</template>

<script lang="ts" setup>
import Overview from '@/views/website/guard/overview/index.vue';
import Monitor from '@/views/website/guard/monitor/index.vue';
import Events from '@/views/website/guard/events/index.vue';
import i18n from '@/lang';

defineOptions({ name: 'WebsiteGuard' });

const route = useRoute();
const tab = ref(
    ['overview', 'monitor', 'events'].includes(String(route.query.tab)) ? String(route.query.tab) : 'overview',
);
const routerButton = [
    {
        label: i18n.global.t('menu.websiteGuard'),
        path: '/websites/guard',
    },
];
</script>

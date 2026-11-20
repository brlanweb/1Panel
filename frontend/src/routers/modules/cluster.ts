import { Layout } from '@/routers/constant';

const clusterRouter = {
    sort: 1050,
    path: '/cluster',
    name: 'Cluster-Menu',
    component: Layout,
    redirect: '/cluster/overview',
    meta: {
        icon: 'p-host',
        title: 'menu.cluster',
    },
    children: [
        {
            path: '/cluster/overview',
            name: 'ClusterOverview',
            component: () => import('@/views/cluster/overview/index.vue'),
            meta: {
                title: 'menu.clusterOverview',
                permission: 'cluster_node_view',
            },
        },
        {
            path: '/cluster/nodes',
            name: 'ClusterNodes',
            component: () => import('@/views/cluster/nodes/index.vue'),
            meta: {
                icon: 'p-host',
                title: 'menu.clusterNodes',
                permission: 'cluster_node_view',
            },
        },
        {
            path: '/cluster/panels',
            name: 'ClusterPanels',
            component: () => import('@/views/cluster/panels/index.vue'),
            meta: {
                title: 'menu.clusterPanels',
                permission: 'cluster_node_view',
            },
        },
        {
            path: '/cluster/waf',
            name: 'ClusterWaf',
            component: () => import('@/views/cluster/waf/index.vue'),
            meta: {
                title: 'menu.clusterWaf',
                permission: 'cluster_node_view',
            },
        },
        {
            path: '/cluster/waf/websites',
            name: 'ClusterWafWebsites',
            component: () => import('@/views/cluster/waf/websites/index.vue'),
            hidden: true,
            meta: {
                activeMenu: '/cluster/waf',
                title: 'menu.clusterWafWebsites',
                permission: 'cluster_node_view',
            },
        },
        {
            path: '/cluster/sync',
            name: 'ClusterSync',
            component: () => import('@/views/cluster/sync/index.vue'),
            meta: {
                title: 'menu.clusterSync',
                permission: 'cluster_node_view',
            },
        },
        {
            path: '/cluster/monitor',
            name: 'ClusterMonitor',
            component: () => import('@/views/cluster/monitor/index.vue'),
            meta: {
                title: 'menu.clusterMonitor',
                permission: 'cluster_node_view',
            },
        },
    ],
};

export default clusterRouter;

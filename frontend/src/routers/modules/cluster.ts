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
            path: '/cluster/resource',
            name: 'ClusterResource',
            component: () => import('@/views/cluster/resource/index.vue'),
            meta: {
                title: 'menu.clusterResource',
                permission: 'cluster_node_view',
            },
        },
        {
            path: '/cluster/compute',
            name: 'ClusterCompute',
            component: () => import('@/views/cluster/compute/index.vue'),
            meta: {
                title: 'menu.clusterCompute',
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
            // Panel management was merged into node management.
            path: '/cluster/panels',
            redirect: '/cluster/nodes',
            hidden: true,
        },
        {
            // Sidebar entry; the five sync pages below are hidden children
            // switched with the RouterButton bar (like the official
            // /xpack/sync/* pages).
            path: '/cluster/sync',
            name: 'ClusterSync',
            redirect: '/cluster/sync/file',
            meta: {
                title: 'menu.clusterSync',
                permission: 'cluster_node_view',
            },
        },
        {
            path: '/cluster/sync/file',
            name: 'ClusterSyncFile',
            component: () => import('@/views/cluster/sync/file/index.vue'),
            hidden: true,
            meta: {
                parent: 'menu.clusterSync',
                title: 'xpack.sync.file',
                activeMenu: '/cluster/sync',
                permission: 'cluster_node_view',
            },
        },
        {
            path: '/cluster/sync/image',
            name: 'ClusterSyncImage',
            component: () => import('@/views/cluster/sync/image/index.vue'),
            hidden: true,
            meta: {
                parent: 'menu.clusterSync',
                title: 'xpack.sync.image',
                activeMenu: '/cluster/sync',
                permission: 'cluster_node_view',
            },
        },
        {
            path: '/cluster/sync/app',
            name: 'ClusterSyncApp',
            component: () => import('@/views/cluster/sync/app/index.vue'),
            hidden: true,
            meta: {
                parent: 'menu.clusterSync',
                title: 'xpack.sync.app',
                activeMenu: '/cluster/sync',
                permission: 'cluster_node_view',
            },
        },
        {
            path: '/cluster/sync/ssl',
            name: 'ClusterSyncSsl',
            component: () => import('@/views/cluster/sync/ssl/index.vue'),
            hidden: true,
            meta: {
                parent: 'menu.clusterSync',
                title: 'xpack.sync.ssl',
                activeMenu: '/cluster/sync',
                permission: 'cluster_node_view',
            },
        },
        {
            path: '/cluster/sync/php-extensions',
            name: 'ClusterSyncPhp',
            component: () => import('@/views/cluster/sync/php/index.vue'),
            hidden: true,
            meta: {
                parent: 'menu.clusterSync',
                title: 'xpack.sync.phpExtensionsTitle',
                activeMenu: '/cluster/sync',
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
        {
            path: '/cluster/db-replication',
            name: 'ClusterDbReplication',
            component: () => import('@/views/cluster/db-replication/index.vue'),
            meta: {
                title: 'menu.clusterDbReplication',
                permission: 'cluster_node_view',
            },
        },
    ],
};

export default clusterRouter;

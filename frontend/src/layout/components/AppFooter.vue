<template>
    <div class="footer" :class="{ 'footer--mobile': isMobile }">
        <div class="footer-content">
            <!-- Cluster fork: upstream copyright and the footer links
                 (商业版 / 论坛求助 / 使用手册 / 项目地址) are removed. -->
            <SystemUpgrade class="footer-upgrade" />
        </div>
    </div>
</template>

<script setup lang="ts">
import SystemUpgrade from '@/components/system-upgrade/index.vue';
import { useGlobalStore } from '@/composables/useGlobalStore';

const { isMobile } = useGlobalStore();
</script>

<style scoped lang="scss">
.footer {
    display: flex;
    align-items: center;
    justify-content: space-between;
    min-height: 48px;
    height: auto;
    background: var(--panel-footer-bg);
    border-top: 1px solid var(--panel-footer-border);
    box-sizing: border-box;
    padding: 10px 12px;
    a {
        font-size: 12px;
        color: #858585;
        text-decoration: none;
        letter-spacing: 0.5px;
    }
    span {
        font-size: 12px;
        color: #858585;
        text-decoration: none;
        letter-spacing: 0.5px;
    }
}

.footer--mobile {
    min-height: 48px;
}

.footer-content {
    display: flex;
    width: 100%;
    align-items: center;
    justify-content: center;
}

.footer-upgrade {
    white-space: nowrap;
}

@media (min-width: 768px) {
    .footer {
        padding: 10px 20px;
    }

    .footer-content {
        justify-content: flex-end;
    }
}
</style>

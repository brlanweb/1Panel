<template>
    <div>
        <div class="flex flex-wrap items-center">
            <div class="flex flex-wrap items-center">
                <el-link v-if="isEE" underline="never" type="primary" @click="toEdition">
                    {{ $t('license.ee') }}
                </el-link>
                <el-link v-else-if="isMasterPro" underline="never" type="primary" @click="toLxware">
                    {{ $t('license.pro') }}
                </el-link>
                <el-link v-else-if="isOffline" underline="never" type="primary" @click="to1Panel">
                    {{ $t('license.offLine') }}
                </el-link>
                <el-link v-else underline="never" type="primary" @click="toEdition">
                    {{ $t('license.community') }}
                </el-link>
                <el-link underline="never" class="version" type="primary" @click="getVersionLog()">
                    {{ version }}
                </el-link>
                <!-- Cluster fork: the upstream "update" link and its badge
                     are hidden on purpose (see DESIGN.md §14). Version
                     upgrades are out of scope for this fork: the panel
                     must never offer or run an automatic update that would
                     replace the cluster build with an upstream release. -->
                <el-tag v-if="version === 'Waiting'" round class="ml-2.5">{{ $t('setting.upgrading') }}</el-tag>
            </div>
        </div>

        <Upgrade ref="upgradeRef" @search="search" />
        <Releases ref="releasesRef" />
    </div>
</template>

<script setup lang="ts">
import { getSettingBaseInfo } from '@/api/modules/setting';
import Upgrade from '@/components/system-upgrade/upgrade/index.vue';
import Releases from '@/components/system-upgrade/releases/index.vue';
import { onMounted, ref } from 'vue';
import { useGlobalStore } from '@/composables/useGlobalStore';

const { isOffline, isMasterPro, isEE, isIntl } = useGlobalStore();
const upgradeRef = ref();
const releasesRef = ref();

const version = ref<string>('');

const search = async () => {
    const res = await getSettingBaseInfo();
    version.value = res.data.systemVersion;
};

const getVersionLog = () => {
    if (isOffline.value) {
        return;
    }
    releasesRef.value.acceptParams();
};

const toLxware = () => {
    if (!isIntl.value) {
        window.open('https://www.lxware.cn/1panel' + '', '_blank', 'noopener,noreferrer');
    } else {
        window.open('https://1panel.pro/pricing' + '', '_blank', 'noopener,noreferrer');
    }
};

const to1Panel = () => {
    let url = isIntl.value ? 'https://1panel.pro' : 'https://1panel.cn';
    window.open(url, '_blank', 'noopener,noreferrer');
};

const toEdition = () => {
    if (!isIntl.value) {
        window.open('https://1panel.cn/versions.html' + '', '_blank', 'noopener,noreferrer');
    } else {
        window.open('https://1panel.pro/pricing' + '', '_blank', 'noopener,noreferrer');
    }
};

onMounted(() => {
    search();
});
</script>

<style lang="scss" scoped>
.line-height {
    line-height: 25px;
}
:deep(.el-link__inner) {
    font-weight: 400;
}
.version {
    margin-left: 8px;
    font-size: 14px;
    color: var(--panel-color-primary-light-4);
    text-decoration: none;
    letter-spacing: 0.5px;
    cursor: pointer;
    font-family: auto;
}
</style>

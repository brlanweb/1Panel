<template>
    <div class="flex items-center justify-center min-h-screen relative login-page">
        <div class="absolute inset-0 bg-cover bg-center bg-no-repeat login-bg" :style="backgroundStyle"></div>
        <div
            v-if="externalLoginPending"
            v-loading="true"
            class="absolute inset-0 z-20 bg-gray-100"
            aria-busy="true"
        ></div>
        <div
            v-show="!externalLoginPending"
            :style="{ opacity: backgroundOpacity, width: containerWidth, height: containerHeight }"
            class="bg-white relative z-10 flex overflow-hidden login-card"
            id="login-container"
        >
            <div class="grid items-stretch w-full" :style="loginGridStyle">
                <div v-if="showLogo" class="flex justify-center" :style="{ height: containerHeight }">
                    <LoginArt v-if="useDefaultArt" :title="themeConfig.panelName" />
                    <img
                        v-else
                        v-show="imgLoaded"
                        :src="loadImage('loginImage')"
                        class="max-w-full max-h-full object-cover bg-cover bg-center"
                        alt="1panel"
                        @load="onImgLoad"
                        @error="onImgError"
                    />
                </div>
                <div :class="loginFormClass">
                    <LoginForm ref="loginRef" @external-login-ready="externalLoginPending = false"></LoginForm>
                </div>
            </div>
        </div>
    </div>
</template>

<script setup lang="ts">
import LoginForm from './components/login-form.vue';
import LoginArt from './components/login-art.vue';
import { ref, onMounted } from 'vue';
import { useGlobalStore } from '@/composables/useGlobalStore';
import { preloadImage } from '@/utils/browser';
import { hasExternalLoginTicket } from '@/utils/external-login';
defineOptions({ name: 'Login' });
const { entrance, themeConfig } = useGlobalStore();
const backgroundOpacity = ref(1);
// The old bundled jpg/png login images were the teal 1Panel artwork. The
// default left panel is now the theme-aware ginkgo illustration and the
// default background a CSS gradient (see .login-bg); both follow the
// ginkgo / red market theme. Custom uploaded images still win.
const loadedLoginImage = ref<string | null>(null);
const loadedBackgroundImage = ref<string | null>(null);
const backgroundStyle = ref<{ backgroundImage?: string; backgroundColor?: string }>({});
const imgLoaded = ref(false);
const loginImageFailed = ref(false);
const useDefaultArt = computed(() => {
    const { loginImage } = themeConfig.value;
    if (loginImageFailed.value || !loginImage) return true;
    if (loginImage === 'loginImage') return !loadedLoginImage.value;
    return false;
});

const externalLoginPending = ref(hasExternalLoginTicket());

function onImgLoad() {
    imgLoaded.value = true;
}
const mySafetyCode = defineProps({
    code: {
        type: String,
        default: '',
    },
});

const getStatus = async () => {
    let code = mySafetyCode.code;
    if (code != '') {
        entrance.value = code;
    }
};

const loadImage = (name: string) => {
    const { loginImage, loginBackground, loginBgType } = themeConfig.value;
    if (name === 'loginImage') {
        if (loginImage === 'loginImage') {
            return loadedLoginImage.value || '';
        }
        return loginImage || '';
    }
    if (name === 'loginBackground') {
        if (loginBgType === 'image') {
            if (loginBackground === 'loginBackground') {
                return loadedBackgroundImage.value || '';
            }
            return loginBackground || '';
        }
        if (loginBgType === 'color') {
            return loginBackground;
        }
    }
    return '';
};

const onImgError = () => {
    loginImageFailed.value = true;
};

onMounted(async () => {
    await getStatus();
    const loginImageUrl = `/api/v2/images/loginImage?t=${Date.now()}`;
    const backgroundImageUrl = `/api/v2/images/loginBackground?t=${Date.now()}`;
    if (themeConfig.value.loginImage === 'loginImage') {
        loadedLoginImage.value = await preloadImage(loginImageUrl);
    }
    if (themeConfig.value.loginBgType === 'image' && themeConfig.value.loginBackground === 'loginBackground') {
        loadedBackgroundImage.value = await preloadImage(backgroundImageUrl);
    }
    if (themeConfig.value.loginBgType === 'color') {
        backgroundStyle.value = {
            backgroundColor: themeConfig.value.loginBackground,
        };
    } else {
        // No custom background: keep the theme gradient from .login-bg.
        const url = loadImage('loginBackground');
        if (!url) return;
        const img = new Image();
        img.onload = () => {
            backgroundStyle.value = {
                backgroundImage: `url(${url})`,
            };
        };
        img.src = url;
    }
});

const FIXED_WIDTH = 1000;
const FIXED_HEIGHT = 415;
const useWindowSize = () => {
    const width = ref(window.innerWidth);
    const height = ref(window.innerHeight);

    const updateSize = () => {
        width.value = window.innerWidth;
        height.value = window.innerHeight;
    };

    onMounted(() => window.addEventListener('resize', updateSize));
    onUnmounted(() => window.removeEventListener('resize', updateSize));

    return { width, height };
};
const { width } = useWindowSize();
const showLogo = computed(() => width.value >= FIXED_WIDTH);
const containerWidth = computed(() => `${FIXED_WIDTH}px`);
const containerHeight = computed(() => `${FIXED_HEIGHT}px`);
const loginGridStyle = computed(() => ({
    gridTemplateColumns: showLogo.value ? 'repeat(2, minmax(0, 1fr))' : 'minmax(0, 1fr)',
}));
const loginFormClass = computed(() => {
    return showLogo.value
        ? 'flex items-center justify-center p-4 min-w-0'
        : 'flex items-center justify-center p-4 w-full min-w-0';
});
</script>

<style scoped lang="scss">
.login-page {
    background-color: var(--panel-color-primary-light-9);
}
// Default background: soft accent wash with two blurred light spots, so it
// matches the ginkgo / red theme instead of the old teal photo.
.login-bg {
    background-image:
        radial-gradient(circle at 12% 18%, rgba(var(--panel-accent-rgb), 0.18), transparent 42%),
        radial-gradient(circle at 88% 82%, rgba(var(--panel-accent-rgb), 0.14), transparent 46%),
        linear-gradient(
            135deg,
            var(--panel-color-primary-light-9) 0%,
            #ffffff 55%,
            var(--panel-color-primary-light-7) 100%
        );
}
.login-card {
    border-radius: 14px;
    border: 1px solid rgba(var(--panel-accent-rgb), 0.14);
    box-shadow:
        0 24px 60px -20px rgba(var(--panel-accent-rgb), 0.35),
        0 6px 18px rgba(15, 23, 42, 0.06);
}
@media (max-width: 1000px) {
    .login-card {
        width: min(440px, calc(100vw - 32px)) !important;
        height: auto !important;
        min-height: 415px;
    }
}
</style>

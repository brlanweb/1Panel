<template>
    <div class="login-art">
        <svg class="login-art-svg" viewBox="0 0 500 415" preserveAspectRatio="xMidYMid slice" aria-hidden="true">
            <defs>
                <linearGradient id="loginArtSky" x1="0" y1="0" x2="1" y2="1">
                    <stop offset="0%" class="stop-soft" />
                    <stop offset="100%" class="stop-mid" />
                </linearGradient>
                <radialGradient id="loginArtGlow" cx="0.78" cy="0.22" r="0.6">
                    <stop offset="0%" class="stop-glow" stop-opacity="0.55" />
                    <stop offset="100%" class="stop-glow" stop-opacity="0" />
                </radialGradient>
                <symbol id="loginArtLeaf" viewBox="-40 -70 80 104">
                    <path
                        d="M0,0 C-8,-6 -30,-22 -34,-44 C-36,-58 -24,-66 -12,-62 C-6,-60 -2,-55 0,-48 C2,-55 6,-60 12,-62 C24,-66 36,-58 34,-44 C30,-22 8,-6 0,0 Z"
                    />
                    <path
                        class="vein"
                        d="M0,-2 L-26,-50 M0,-2 L-14,-58 M0,-2 L-3,-50 M0,-2 L3,-50 M0,-2 L14,-58 M0,-2 L26,-50"
                    />
                    <path class="stem" d="M0,0 Q3,14 -3,30" />
                </symbol>
            </defs>

            <rect width="500" height="415" fill="url(#loginArtSky)" />
            <rect width="500" height="415" fill="url(#loginArtGlow)" />

            <!-- distant hills -->
            <path
                class="hill hill-back"
                d="M0,330 C90,290 170,300 250,318 C340,338 420,300 500,288 L500,415 L0,415 Z"
            />
            <path
                class="hill hill-front"
                d="M0,362 C110,336 200,350 300,362 C390,372 450,352 500,346 L500,415 L0,415 Z"
            />

            <!-- branch -->
            <path class="branch" d="M500,52 C430,66 380,92 330,128 C300,150 280,168 262,190" />
            <path class="branch thin" d="M392,86 C380,62 360,48 336,40" />
            <path class="branch thin" d="M318,140 C300,124 278,118 254,120" />

            <g class="leaf leaf-strong">
                <use href="#loginArtLeaf" x="300" y="70" width="64" height="83" transform="rotate(-24 332 112)" />
                <use href="#loginArtLeaf" x="238" y="86" width="56" height="73" transform="rotate(-62 266 122)" />
                <use href="#loginArtLeaf" x="350" y="10" width="52" height="68" transform="rotate(-8 376 44)" />
                <use href="#loginArtLeaf" x="226" y="156" width="60" height="78" transform="rotate(14 256 195)" />
            </g>
            <g class="leaf leaf-mid">
                <use href="#loginArtLeaf" x="420" y="56" width="50" height="65" transform="rotate(28 445 88)" />
                <use href="#loginArtLeaf" x="282" y="128" width="46" height="60" transform="rotate(-40 305 158)" />
            </g>

            <!-- falling leaves -->
            <g class="leaf leaf-soft falling">
                <use href="#loginArtLeaf" x="96" y="96" width="44" height="57" transform="rotate(38 118 124)" />
                <use href="#loginArtLeaf" x="150" y="218" width="36" height="47" transform="rotate(-52 168 241)" />
                <use href="#loginArtLeaf" x="58" y="262" width="30" height="39" transform="rotate(118 73 281)" />
                <use href="#loginArtLeaf" x="372" y="232" width="34" height="44" transform="rotate(72 389 254)" />
            </g>
            <g class="leaf leaf-ground">
                <use href="#loginArtLeaf" x="120" y="340" width="34" height="44" transform="rotate(96 137 362)" />
                <use href="#loginArtLeaf" x="300" y="352" width="30" height="39" transform="rotate(-104 315 371)" />
                <use href="#loginArtLeaf" x="420" y="334" width="38" height="49" transform="rotate(70 439 358)" />
            </g>
        </svg>
        <div class="login-art-caption">
            <div class="login-art-title">{{ title }}</div>
            <div class="login-art-line" />
        </div>
    </div>
</template>

<script setup lang="ts">
defineProps({
    title: {
        type: String,
        default: '',
    },
});
</script>

<style scoped lang="scss">
// Every color comes from the active panel accent, so the art follows the
// ginkgo (default) and red market themes without separate image assets.
.login-art {
    position: relative;
    width: 100%;
    height: 100%;
    overflow: hidden;
}
.login-art-svg {
    display: block;
    width: 100%;
    height: 100%;
}
.stop-soft {
    stop-color: var(--panel-color-primary-light-9);
}
.stop-mid {
    stop-color: var(--panel-color-primary-light-6);
}
.stop-glow {
    stop-color: #ffffff;
}
.hill-back {
    fill: var(--panel-color-primary-light-5);
    opacity: 0.55;
}
.hill-front {
    fill: var(--panel-color-primary-light-4);
    opacity: 0.6;
}
.branch {
    fill: none;
    stroke: #6b4f2a;
    stroke-width: 4;
    stroke-linecap: round;
    opacity: 0.75;
    &.thin {
        stroke-width: 2.2;
    }
}
.leaf {
    :deep(path) {
        fill: currentColor;
    }
    :deep(.vein) {
        fill: none;
        stroke: #ffffff;
        stroke-width: 1;
        opacity: 0.35;
    }
    :deep(.stem) {
        fill: none;
        stroke: currentColor;
        stroke-width: 2.4;
        stroke-linecap: round;
    }
}
.leaf-strong {
    color: var(--panel-color-primary);
}
.leaf-mid {
    color: var(--panel-color-primary-light-1);
}
.leaf-soft {
    color: var(--panel-color-primary-light-2);
    opacity: 0.85;
}
.leaf-ground {
    color: var(--panel-color-primary-light-3);
    opacity: 0.9;
}
.falling {
    animation: login-art-drift 9s ease-in-out infinite alternate;
    transform-origin: 50% 0;
}
@keyframes login-art-drift {
    from {
        transform: translate(0, -4px);
    }
    to {
        transform: translate(8px, 6px);
    }
}
@media (prefers-reduced-motion: reduce) {
    .falling {
        animation: none;
    }
}
.login-art-caption {
    position: absolute;
    left: 36px;
    bottom: 34px;
    right: 36px;
}
.login-art-title {
    font-size: 26px;
    font-weight: 600;
    letter-spacing: 1px;
    color: #3a2a10;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
}
.login-art-line {
    width: 44px;
    height: 3px;
    margin-top: 10px;
    border-radius: 2px;
    background: var(--panel-color-primary);
}
</style>

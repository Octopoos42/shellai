<template>
  <q-layout view="hHh lpR fFf" class="layout-shellai">
    <q-header class="header-shellai">
      <q-toolbar class="toolbar-shellai">
        <div class="wrap-icon-brand" aria-label="Brand icon">
          <q-icon name="memory" class="icon-brand" />
        </div>

        <div class="center-brand">
          <div class="title-brand">intelligent shell interface</div>
          <div class="subtitle-brand">Advanced AI assistant</div>
        </div>
      </q-toolbar>
    </q-header>

    <q-page-container>
      <router-view />
    </q-page-container>
  </q-layout>
</template>

<script setup lang="ts"></script>

<style scoped>
.layout-shellai {
  background: radial-gradient(circle at 10% 10%, #08324f 0%, #041426 45%, #020712 100%);
  min-height: 100vh;
}

.header-shellai {
  background: rgba(6, 20, 35, 0.88);
  border-bottom: 1px solid rgba(81, 196, 255, 0.35);
  backdrop-filter: blur(8px);
}

.toolbar-shellai {
  min-height: 58px;
  position: relative;
}

.wrap-icon-brand {
  width: 34px;
  height: 34px;
  display: flex;
  align-items: center;
  justify-content: center;
  border-radius: 10px;
  background: linear-gradient(145deg, rgba(25, 144, 197, 0.28), rgba(33, 84, 184, 0.18));
  border: 1px solid rgba(114, 209, 255, 0.45);
}

.icon-brand {
  font-size: 20px;
  color: #9ceeff;
}

.center-brand {
  position: absolute;
  left: 50%;
  transform: translateX(-50%);
  text-align: center;
  line-height: 1.15;
}

.title-brand {
  font-size: 16px;
  font-weight: 700;
  letter-spacing: 0.08em;
  text-transform: lowercase;
  color: #e8f7ff;
}

.subtitle-brand {
  margin-top: 2px;
  font-size: 11px;
  color: #9acde8;
  letter-spacing: 0.06em;
}
</style>

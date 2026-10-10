<script setup lang="ts">
import { ref } from "vue";
import { HardDrive, Plus, TriangleAlert } from "@lucide/vue";
import { Window } from "@wailsio/runtime";
import type { RcloneStatus } from "../bindings/rclone-drive/models";
import * as SystemService from "../bindings/rclone-drive/systemservice";
import * as WindowService from "../bindings/rclone-drive/windowservice";
import DriveCard from "./components/DriveCard.vue";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Empty, EmptyContent, EmptyDescription, EmptyHeader, EmptyMedia, EmptyTitle } from "@/components/ui/empty";
import { Skeleton } from "@/components/ui/skeleton";
import { Toaster } from "@/components/ui/sonner";
import { TooltipProvider } from "@/components/ui/tooltip";
import { X } from "@lucide/vue";
import { useDrives } from "./composables/useDrives";
import { useTheme } from "./composables/useTheme";

const { remotes, statuses, loading, error, refresh } = useDrives();
const { isDark } = useTheme();

const rclone = ref<RcloneStatus | null>(null);

SystemService.RcloneStatus()
  .then((status) => {
    rclone.value = status;
  })
  .catch(() => {
    rclone.value = null;
  });

// 新增 / 设置使用独立窗口
function openAdd() {
  void WindowService.OpenAddDrive();
}

function openSettings() {
  void WindowService.OpenSettings();
}

// 关闭按钮：隐藏到托盘，程序继续后台运行
function closeWindow() {
  void Window.Close();
}
</script>

<template>
  <TooltipProvider>
    <div class="bg-background flex h-screen flex-col">
      <header
        class="border-b flex items-center justify-between gap-2 px-3 py-2"
        :style="{ '--wails-draggable': 'drag' }"
      >
        <div class="flex min-w-0 items-center gap-2">
          <img src="/logo.png" alt="RcloneDrive" class="size-5" />
          <h1 class="text-base font-semibold">RcloneDrive</h1>
        </div>
        <div class="flex items-center gap-2" :style="{ '--wails-draggable': 'no-drag' }">
          <Button size="sm" @click="openAdd">＋添加</Button>
          <Button variant="outline" size="sm" @click="openSettings">⚙设置</Button>
          <Button variant="ghost" size="icon" aria-label="关闭" @click="closeWindow">
            <X data-icon="inline-start" />
          </Button>
        </div>
      </header>

      <main class="flex-1 overflow-y-auto p-4">
        <Alert v-if="rclone && !rclone.found" variant="destructive" class="mb-4">
          <TriangleAlert />
          <AlertTitle>当前构建未启用挂载功能</AlertTitle>
          <AlertDescription>
            挂载依赖内嵌 rclone 引擎的 cmount 实现，需要以 <code>cmount</code> 构建标签编译，并在系统中安装
            <a href="https://winfsp.dev/" target="_blank" class="font-medium underline">WinFsp</a>
            （macOS 为 macFUSE，Linux 为 fuse3）。
          </AlertDescription>
        </Alert>

        <div v-if="loading" class="flex flex-col gap-3">
          <Skeleton v-for="i in 3" :key="i" class="h-[76px] w-full" />
        </div>

        <Alert v-else-if="error" variant="destructive">
          <TriangleAlert />
          <AlertTitle>加载失败</AlertTitle>
          <AlertDescription>{{ error }}</AlertDescription>
        </Alert>

        <Empty v-else-if="remotes.length === 0" class="border rounded-xl">
          <EmptyHeader>
            <EmptyMedia variant="icon">
              <HardDrive />
            </EmptyMedia>
            <EmptyTitle>还没有虚拟驱动器</EmptyTitle>
            <EmptyDescription>点击右上角「＋添加」创建你的第一个远程存储驱动器。</EmptyDescription>
          </EmptyHeader>
          <EmptyContent>
            <Button @click="openAdd">
              <Plus data-icon="inline-start" />
              添加驱动器
            </Button>
          </EmptyContent>
        </Empty>

        <div v-else class="flex flex-col gap-3">
          <DriveCard
            v-for="remote in remotes"
            :key="remote.name"
            :remote="remote"
            :status="statuses[remote.name]"
            @changed="refresh"
          />
        </div>
      </main>
    </div>

    <Toaster :theme="isDark ? 'dark' : 'light'" position="top-center" />
  </TooltipProvider>
</template>

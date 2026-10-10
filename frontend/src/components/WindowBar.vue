<script setup lang="ts">
import { X } from "@lucide/vue";
import { Window } from "@wailsio/runtime";
import { Button } from "@/components/ui/button";

defineProps<{
  title: string;
  subtitle?: string;
}>();

// 关闭子窗口 = 销毁窗口（主窗口关闭才是隐藏到托盘）
function closeWindow() {
  void Window.Close();
}
</script>

<template>
  <header
    class="border-b flex items-center justify-between gap-2 px-3 py-2"
    :style="{ '--wails-draggable': 'drag' }"
  >
    <div class="flex min-w-0 flex-col">
      <h1 class="truncate text-sm font-semibold">{{ title }}</h1>
      <p v-if="subtitle" class="text-muted-foreground truncate text-xs">{{ subtitle }}</p>
    </div>
    <Button
      variant="ghost"
      size="icon"
      aria-label="关闭"
      :style="{ '--wails-draggable': 'no-drag' }"
      @click="closeWindow"
    >
      <X data-icon="inline-start" />
    </Button>
  </header>
</template>

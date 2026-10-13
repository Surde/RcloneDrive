import { computed, onMounted, onUnmounted, ref } from "vue";
import { Events } from "@wailsio/runtime";
import * as UpdateService from "../../bindings/rclone-drive/updateservice";
import type { UpdateStatus } from "../../bindings/rclone-drive/models";
import type { Progress } from "../../bindings/github.com/wailsapp/wails/v3/pkg/updater/models";

// update:error 事件的负载（Go 侧 UpdateErrorEvent 只作为事件 payload，不在 bindings models 里）
interface UpdateErrorEvent {
  stage: string;
  message: string;
}

export function useUpdate() {
  const status = ref<UpdateStatus | null>(null);
  const progress = ref<Progress | null>(null);
  const error = ref("");

  // 检查/下载/安装进行中时禁用「检查更新」按钮
  const busy = computed(() => {
    const state = status.value?.state ?? "";
    return ["checking", "downloading", "verifying", "installing"].includes(state);
  });

  // 仅当已下载且 SHA-256 二次校验通过时才允许「立即重启」
  const ready = computed(() => status.value?.downloaded === true && status.value?.verified === true);
  const downloading = computed(() => {
    const state = status.value?.state ?? "";
    return ["downloading", "verifying", "installing"].includes(state);
  });

  async function refresh() {
    try {
      status.value = await UpdateService.Status();
    } catch {
      // 保持现有状态
    }
  }

  async function check() {
    error.value = "";
    try {
      await UpdateService.CheckManual();
    } catch (e) {
      error.value = e instanceof Error ? e.message : String(e);
    } finally {
      await refresh();
    }
  }

  async function restart() {
    await UpdateService.Restart();
  }

  const offs: Array<() => void> = [];

  onMounted(async () => {
    await refresh();
    offs.push(
      Events.On("update:available", (event) => {
        status.value = event.data as UpdateStatus;
        error.value = "";
      }),
      Events.On("update:latest", (event) => {
        status.value = event.data as UpdateStatus;
        error.value = "";
      }),
      Events.On("update:progress", (event) => {
        progress.value = event.data as Progress;
      }),
      Events.On("update:ready", (event) => {
        status.value = event.data as UpdateStatus;
        progress.value = null;
      }),
      Events.On("update:error", (event) => {
        const info = event.data as UpdateErrorEvent;
        error.value = info.message;
        void refresh();
      }),
    );
  });

  onUnmounted(() => {
    for (const off of offs) off();
  });

  return { status, progress, error, busy, ready, downloading, refresh, check, restart };
}

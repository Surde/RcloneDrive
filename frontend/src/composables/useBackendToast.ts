import { onMounted, onUnmounted } from "vue";
import { Events } from "@wailsio/runtime";
import { notifyFromBackend } from "@/lib/notify";

// 后端 app:toast 事件携带目标窗口名，但 Wails 会把事件广播到所有窗口，
// 这里按 windowName 过滤：哪个窗口发起的操作，提示就出现在哪个窗口。
export function useBackendToast(windowName: string) {
  let off: (() => void) | undefined;
  onMounted(() => {
    off = Events.On("app:toast", (event) => {
      const data = event.data as { level?: string; title?: string; body?: string; target?: string } | undefined;
      if (!data?.title) return;
      if ((data.target || "main") !== windowName) return;
      notifyFromBackend(data.level ?? "success", data.title, data.body ?? "");
    });
  });
  onUnmounted(() => off?.());
}

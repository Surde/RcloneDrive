// 连接类型 → 界面名（中文）的缓存：主界面只用于显示，取一次即可
import { ref } from "vue";
import * as RemoteService from "../../bindings/rclone-drive/remoteservice";

const labels = ref<Record<string, string | undefined>>({});
let loaded = false;

export function useTypeLabels() {
  async function load() {
    if (loaded) return;
    loaded = true;
    try {
      labels.value = (await RemoteService.TypeLabels()) ?? {};
    } catch {
      // 取不到就退回显示原始类型名
      loaded = false;
    }
  }

  function label(type: string) {
    return labels.value[type] ?? type.toUpperCase();
  }

  return { load, label };
}

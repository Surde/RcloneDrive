import { onMounted, onUnmounted, ref, shallowRef } from "vue";
import { Events } from "@wailsio/runtime";
import type { MountStatus, Remote } from "../../bindings/rclone-drive/models";
import * as MountService from "../../bindings/rclone-drive/mountservice";
import * as RemoteService from "../../bindings/rclone-drive/remoteservice";

export function useDrives() {
  const remotes = shallowRef<Remote[]>([]);
  const statuses = ref<Record<string, MountStatus>>({});
  const loading = ref(true);
  const error = ref("");

  async function refreshStatus(name: string) {
    try {
      const status = await MountService.Status(name);
      statuses.value = { ...statuses.value, [name]: status };
    } catch {
      // keep existing state
    }
  }

  async function refresh() {
    try {
      error.value = "";
      const list = (await RemoteService.List()) ?? [];
      remotes.value = list;
      const entries = await Promise.all(
        list.map(async (remote) => {
          try {
            return [remote.name, await MountService.Status(remote.name)] as const;
          } catch {
            return [remote.name, { mounted: false, log: "" }] as const;
          }
        }),
      );
      statuses.value = Object.fromEntries(entries);
    } catch (e) {
      error.value = e instanceof Error ? e.message : String(e);
    } finally {
      loading.value = false;
    }
  }

  let offMount: (() => void) | undefined;
  let offRemotes: (() => void) | undefined;

  onMounted(async () => {
    await refresh();
    offMount = Events.On("mount:changed", (event) => {
      // 后台异常断开已由后端发系统通知，这里只同步界面状态
      refreshStatus((event.data as { name: string }).name);
    });
    // 新增/编辑窗口保存后，或远程列表变化时刷新
    offRemotes = Events.On("remotes:changed", () => {
      void refresh();
    });
  });

  onUnmounted(() => {
    offMount?.();
    offRemotes?.();
  });

  return { remotes, statuses, loading, error, refresh, refreshStatus };
}

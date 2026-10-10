import { onMounted, onUnmounted, ref, shallowRef } from "vue";
import { Events } from "@wailsio/runtime";
import type { MountStatus, Remote } from "../../bindings/rclone-drive/models";
import * as MountService from "../../bindings/rclone-drive/mountservice";
import * as RemoteService from "../../bindings/rclone-drive/remoteservice";

export function useDrives() {
  const remotes = shallowRef<Remote[]>([]);
  const statuses = ref<Record<string, MountStatus>>({});
  // 各驱动器的实时速率（字节/秒），由后端 mount:speed 事件推送
  const speedsRef = ref<Record<string, { down: number; up: number }>>({});
  // 各驱动器的容量信息（按需从后端取，变化很慢）
  const usages = ref<Record<string, { total: number; used: number; free: number }>>({});
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
  let offSpeed: (() => void) | undefined;

  // 速率由后端推送（前端轮询会被浏览器对后台窗口的定时器节流）；
  // 传输停下来后再保留一会儿，避免短传输一闪而过看不见
  const SPEED_HOLD_MS = 1500;
  const holdUntil = new Map<string, number>();
  const holdTimers = new Map<string, number>();
  let lastPayload: Record<string, { down?: number; up?: number }> = {};

  function applySpeeds(next: Record<string, { down?: number; up?: number }>) {
    lastPayload = next;
    const mounted = new Set(
      Object.entries(statuses.value)
        .filter(([, status]) => status?.mounted)
        .map(([name]) => name),
    );
    const speeds: Record<string, { down: number; up: number }> = {};
    const now = Date.now();
    for (const name of mounted) {
      const value = next[name];
      const down = value?.down ?? 0;
      const up = value?.up ?? 0;
      if (down > 0 || up > 0) {
        speeds[name] = { down, up };
        holdUntil.set(name, now + SPEED_HOLD_MS);
        scheduleHoldExpiry(name, SPEED_HOLD_MS);
        continue;
      }
      const until = holdUntil.get(name) ?? 0;
      const previous = speedsRef.value[name];
      if (now < until && previous) {
        // 刚停下来，先保持上一次的数字，到点后再清掉
        speeds[name] = previous;
        scheduleHoldExpiry(name, until - now);
      }
    }
    speedsRef.value = speeds;
  }

  function scheduleHoldExpiry(name: string, delay: number) {
    const timer = holdTimers.get(name);
    if (timer !== undefined) window.clearTimeout(timer);
    holdTimers.set(
      name,
      window.setTimeout(() => {
        holdTimers.delete(name);
        applySpeeds(lastPayload);
      }, Math.max(delay, 0) + 20),
    );
  }

  onMounted(async () => {
    await refresh();
    void refreshUsages();
    startUsageTimer();
    offMount = Events.On("mount:changed", (event) => {
      // 后台异常断开已由后端发系统通知，这里只同步界面状态
      void refreshStatus((event.data as { name: string }).name).then(() => refreshUsages());
    });
    // 新增/编辑窗口保存后，或远程列表变化时刷新
    offRemotes = Events.On("remotes:changed", () => {
      void refresh();
    });
    // 后端每 250ms 采样、变化时推送速率
    offSpeed = Events.On("mount:speed", (event) => {
      const data = event.data as { speeds?: Record<string, { down?: number; up?: number }> } | undefined;
      applySpeeds(data?.speeds ?? {});
    });
    // 窗口重新可见/获得焦点时补取一次（万一事件在后台被漏掉）
    window.addEventListener("focus", onFocusOrVisible);
    document.addEventListener("visibilitychange", onVisibilityChange);
  });

  function onFocusOrVisible() {
    void refreshSpeedsOnce();
    void refreshUsages();
  }

  async function refreshSpeedsOnce() {
    try {
      const next = (await MountService.Speeds()) ?? {};
      // 统一成事件里的形状再复用同一套过滤
      const speeds: Record<string, { down?: number; up?: number }> = {};
      for (const [name, value] of Object.entries(next)) {
        if (value) speeds[name] = { down: value.down ?? 0, up: value.up ?? 0 };
      }
      applySpeeds(speeds);
    } catch {
      // 取不到就保持原值
    }
  }

  // 容量变化很慢：挂载状态变化、窗口重新可见、以及每 60 秒各取一次
  const USAGE_REFRESH_MS = 60_000;
  let usageTimer: number | undefined;

  async function refreshUsages() {
    const mounted = Object.entries(statuses.value)
      .filter(([, status]) => status?.mounted)
      .map(([name]) => name);
    if (mounted.length === 0) {
      usages.value = {};
      return;
    }
    try {
      const next = (await MountService.Usages()) ?? {};
      const result: Record<string, { total: number; used: number; free: number }> = {};
      for (const [name, value] of Object.entries(next)) {
        if (!value || !value.total) continue;
        result[name] = { total: value.total, used: value.used ?? 0, free: value.free ?? 0 };
      }
      usages.value = result;
    } catch {
      // 取不到容量就保持原值（卡片上该块不显示）
    }
  }

  function startUsageTimer() {
    if (usageTimer !== undefined) return;
    usageTimer = window.setInterval(() => void refreshUsages(), USAGE_REFRESH_MS);
  }

  function onVisibilityChange() {
    if (!document.hidden) onFocusOrVisible();
  }

  onUnmounted(() => {
    offMount?.();
    offRemotes?.();
    offSpeed?.();
    window.removeEventListener("focus", onFocusOrVisible);
    document.removeEventListener("visibilitychange", onVisibilityChange);
    for (const timer of holdTimers.values()) window.clearTimeout(timer);
    if (usageTimer !== undefined) window.clearInterval(usageTimer);
  });

  return { remotes, statuses, speeds: speedsRef, usages, loading, error, refresh, refreshStatus };
}

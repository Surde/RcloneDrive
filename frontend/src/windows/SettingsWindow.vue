<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import { ArrowUpCircle, CircleHelp, Monitor, Moon, Palette, Power, Sun } from "@lucide/vue";
import * as SettingsService from "../../bindings/rclone-drive/settingsservice";
import { notifyError, notifySuccess } from "@/lib/notify";
import { formatSize, formatSpeed } from "@/lib/format";
import WindowBar from "../components/WindowBar.vue";
import { useTheme, type ThemeMode } from "../composables/useTheme";
import { useUpdate } from "../composables/useUpdate";
import { Field, FieldDescription, FieldGroup, FieldLabel, FieldLegend, FieldSet } from "@/components/ui/field";
import { Spinner } from "@/components/ui/spinner";
import { Toaster } from "@/components/ui/sonner";
import { Switch } from "@/components/ui/switch";
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group";
import { Button } from "@/components/ui/button";
import { Progress } from "@/components/ui/progress";
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from "@/components/ui/tooltip";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogTrigger,
} from "@/components/ui/alert-dialog";

const { mode, isDark, setMode } = useTheme();

type SectionKey = "appearance" | "startup" | "update";

const sections: { key: SectionKey; label: string; icon: typeof Palette }[] = [
  { key: "appearance", label: "外观", icon: Palette },
  { key: "startup", label: "启动", icon: Power },
  { key: "update", label: "更新", icon: ArrowUpCircle },
];

const section = ref<SectionKey>("appearance");

const themeOptions = [
  { value: "light" as ThemeMode, label: "浅色", icon: Sun },
  { value: "dark" as ThemeMode, label: "深色", icon: Moon },
  { value: "system" as ThemeMode, label: "跟随系统", icon: Monitor },
];

const loading = ref(false);
const saving = ref(false);
const supported = ref(true);
const autoStart = ref(false);
const target = ref("");

function errMsg(e: unknown) {
  return e instanceof Error ? e.message : String(e);
}

async function loadAutoStart() {
  loading.value = true;
  try {
    const status = await SettingsService.AutoStart();
    supported.value = status.supported;
    autoStart.value = status.enabled;
    target.value = status.target;
  } catch (e) {
    notifyError(errMsg(e), "读取开机自启状态失败");
  } finally {
    loading.value = false;
  }
}

onMounted(loadAutoStart);

async function onToggleAutoStart(next: boolean) {
  saving.value = true;
  try {
    await SettingsService.SetAutoStart(next);
    notifySuccess(
      next ? "已开启开机自启" : "已关闭开机自启",
      next ? "登录系统后自动启动 RcloneDrive（静默进入托盘）" : "登录系统后不再自动启动 RcloneDrive",
    );
    await loadAutoStart();
  } catch (e) {
    notifyError(errMsg(e), "设置开机自启失败");
    await loadAutoStart();
  } finally {
    saving.value = false;
  }
}

const autoStartHint = computed(() => {
  if (!supported.value) return "当前平台不支持开机自启。";
  if (target.value) return `登录后自动启动：${target.value}`;
  return "登录系统后自动启动 RcloneDrive（静默进入托盘）。";
});

const { status, progress, error, busy, ready, downloading, check, restart } = useUpdate();

const confirmRestart = ref(false);

// 更新状态的人类可读描述
const statusText = computed(() => {
  if (error.value) return "检查更新失败：" + error.value;
  const state = status.value?.state ?? "";
  switch (state) {
    case "checking":
      return "正在检查更新…";
    case "available":
      return status.value?.latest ? `发现新版本 v${status.value.latest}，准备下载…` : "发现新版本，准备下载…";
    case "verifying":
      return "正在校验更新…";
    case "installing":
      return "正在安装更新…";
    case "error":
      return "检查更新失败";
    default:
      break;
  }
  if (ready.value) return "更新已就绪，重启后生效";
  if (state === "up-to-date") return "已是最新版本";
  return "点击「检查更新」查看是否有新版本";
});

// 下载进度百分比（total 未知时保持 0，进度条显示不确定进度）
const percent = computed(() => {
  const total = progress.value?.total ?? 0;
  const written = progress.value?.written ?? 0;
  return total > 0 ? Math.min(100, Math.round((written / total) * 100)) : 0;
});

const progressText = computed(() => {
  const total = progress.value?.total ?? 0;
  const written = progress.value?.written ?? 0;
  const rate = formatSpeed(progress.value?.rate ?? 0);
  return total > 0 ? `${formatSize(written)} / ${formatSize(total)} · ${rate}` : `已下载 ${formatSize(written)} · ${rate}`;
});

async function onCheckUpdate() {
  await check();
}

async function onRestart() {
  try {
    await restart();
  } catch (e) {
    notifyError(errMsg(e), "重启失败");
  }
}
</script>

<template>
  <div class="bg-background flex h-screen flex-col">
    <WindowBar title="设置" subtitle="调整界面外观与启动方式" />

    <div class="flex min-h-0 flex-1">
      <aside class="w-35 shrink-0 space-y-1 border-r p-3">
        <button
          v-for="item in sections"
          :key="item.key"
          type="button"
          :data-active="section === item.key"
          class="flex w-full items-center gap-2 rounded-md px-3 py-2 text-sm text-muted-foreground transition-colors hover:bg-accent hover:text-accent-foreground data-[active=true]:bg-accent data-[active=true]:font-medium data-[active=true]:text-accent-foreground"
          @click="section = item.key"
        >
          <component :is="item.icon" class="size-4" />
          {{ item.label }}
        </button>
      </aside>

      <main class="flex-1 overflow-y-auto p-4">
        <TooltipProvider>
          <FieldGroup>
            <FieldSet v-if="section === 'appearance'">
              <FieldLegend variant="label">外观</FieldLegend>
              <Field>
                <FieldLabel for="theme-mode" class="flex items-center gap-1.5">
                  主题
                  <Tooltip>
                    <TooltipTrigger as-child>
                      <button type="button" class="cursor-help text-muted-foreground transition-colors hover:text-foreground" @click.stop>
                        <CircleHelp class="size-3.5" />
                      </button>
                    </TooltipTrigger>
                    <TooltipContent side="top" class="max-w-56">「跟随系统」会随操作系统的深浅色设置自动切换。</TooltipContent>
                  </Tooltip>
                </FieldLabel>
                <ToggleGroup
                  id="theme-mode"
                  :model-value="mode"
                  type="single"
                  variant="outline"
                  :spacing="2"
                  class="grid w-full grid-cols-3"
                  @update:model-value="setMode($event as ThemeMode)"
                >
                  <ToggleGroupItem
                    v-for="option in themeOptions"
                    :key="option.value"
                    :value="option.value"
                    class="gap-2 data-[state=on]:ring-ring data-[state=on]:ring-2"
                  >
                    <component :is="option.icon" />
                    {{ option.label }}
                  </ToggleGroupItem>
                </ToggleGroup>
              </Field>
            </FieldSet>

            <FieldSet v-if="section === 'startup'">
              <FieldLegend variant="label">启动</FieldLegend>
              <Field orientation="horizontal" :data-disabled="!supported">
                <Switch
                  id="settings-autostart"
                  :model-value="autoStart"
                  :disabled="!supported || loading || saving"
                  @update:model-value="onToggleAutoStart"
                />
                <FieldLabel for="settings-autostart" class="flex items-center gap-1.5 font-normal">
                  开机自启
                  <Tooltip>
                    <TooltipTrigger as-child>
                      <button type="button" class="cursor-help text-muted-foreground transition-colors hover:text-foreground" @click.stop>
                        <CircleHelp class="size-3.5" />
                      </button>
                    </TooltipTrigger>
                    <TooltipContent side="top" class="max-w-56">{{ autoStartHint }}</TooltipContent>
                  </Tooltip>
                </FieldLabel>
                <Spinner v-if="loading || saving" />
              </Field>
            </FieldSet>

            <FieldSet v-if="section === 'update'">
              <FieldLegend variant="label">更新</FieldLegend>
              <Field>
                <FieldLabel>当前版本</FieldLabel>
                <FieldDescription v-if="status">v{{ status.current }}</FieldDescription>
                <FieldDescription v-else>—</FieldDescription>
              </Field>
              <Field>
                <FieldLabel>状态</FieldLabel>
                <FieldDescription>{{ statusText }}</FieldDescription>
                <template v-if="downloading && progress">
                  <Progress :model-value="percent" class="mt-2" />
                  <FieldDescription class="mt-1">{{ percent }}% · {{ progressText }}</FieldDescription>
                </template>
              </Field>
              <Field class="flex-row flex-wrap items-center gap-2">
                <Button variant="outline" :disabled="busy" @click="onCheckUpdate">
                  <Spinner v-if="busy" data-icon="inline-start" />
                  检查更新
                </Button>
                <AlertDialog v-model:open="confirmRestart">
                  <AlertDialogTrigger as-child>
                    <Button v-if="ready">立即重启</Button>
                  </AlertDialogTrigger>
                  <AlertDialogContent>
                    <AlertDialogHeader>
                      <AlertDialogTitle>重启以完成更新？</AlertDialogTitle>
                      <AlertDialogDescription>重启期间所有云盘挂载会临时断开，重启完成后请重新连接。</AlertDialogDescription>
                    </AlertDialogHeader>
                    <AlertDialogFooter>
                      <AlertDialogCancel>取消</AlertDialogCancel>
                      <AlertDialogAction @click="onRestart">重启</AlertDialogAction>
                    </AlertDialogFooter>
                  </AlertDialogContent>
                </AlertDialog>
              </Field>
            </FieldSet>
          </FieldGroup>
        </TooltipProvider>
      </main>
    </div>

    <Toaster :theme="isDark ? 'dark' : 'light'" position="top-center" />
  </div>
</template>

<script setup lang="ts">
import { onMounted, ref } from "vue";
import { toast } from "vue-sonner";
import { Monitor, Moon, Sun } from "@lucide/vue";
import * as SettingsService from "../../bindings/rclone-drive/settingsservice";
import WindowBar from "../components/WindowBar.vue";
import { useTheme, type ThemeMode } from "../composables/useTheme";
import { Field, FieldDescription, FieldGroup, FieldLabel, FieldLegend, FieldSeparator, FieldSet } from "@/components/ui/field";
import { Spinner } from "@/components/ui/spinner";
import { Toaster } from "@/components/ui/sonner";
import { Switch } from "@/components/ui/switch";
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group";

const { mode, isDark, setMode } = useTheme();

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
    toast.error(errMsg(e));
  } finally {
    loading.value = false;
  }
}

onMounted(loadAutoStart);

async function onToggleAutoStart(next: boolean) {
  saving.value = true;
  try {
    await SettingsService.SetAutoStart(next);
    toast.success(next ? "已开启开机自启" : "已关闭开机自启");
    await loadAutoStart();
  } catch (e) {
    toast.error(errMsg(e));
    await loadAutoStart();
  } finally {
    saving.value = false;
  }
}
</script>

<template>
  <div class="bg-background flex h-screen flex-col">
    <WindowBar title="设置" subtitle="调整界面外观与启动方式" />

    <main class="flex-1 overflow-y-auto p-4">
      <FieldGroup>
        <FieldSet>
          <FieldLegend variant="label">外观</FieldLegend>
          <Field>
            <FieldLabel for="theme-mode">主题</FieldLabel>
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
            <FieldDescription>「跟随系统」会随操作系统的深浅色设置自动切换。</FieldDescription>
          </Field>
        </FieldSet>

        <FieldSeparator />

        <FieldSet>
          <FieldLegend variant="label">启动</FieldLegend>
          <Field orientation="horizontal" :data-disabled="!supported">
            <Switch
              id="settings-autostart"
              :model-value="autoStart"
              :disabled="!supported || loading || saving"
              @update:model-value="onToggleAutoStart"
            />
            <FieldLabel for="settings-autostart" class="font-normal">开机自启</FieldLabel>
            <Spinner v-if="loading || saving" />
          </Field>
          <FieldDescription v-if="!supported">当前平台不支持开机自启。</FieldDescription>
          <FieldDescription v-else-if="target">登录后自动启动：{{ target }}</FieldDescription>
          <FieldDescription v-else>登录系统后自动启动 RcloneDrive（静默进入托盘）。</FieldDescription>
        </FieldSet>
      </FieldGroup>
    </main>

    <Toaster :theme="isDark ? 'dark' : 'light'" position="top-center" />
  </div>
</template>

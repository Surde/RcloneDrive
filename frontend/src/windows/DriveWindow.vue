<script setup lang="ts">
import { computed, onMounted, ref, watch } from "vue";
import { toast } from "vue-sonner";
import { ChevronDown } from "@lucide/vue";
import { Window } from "@wailsio/runtime";
import type { Remote } from "../../bindings/rclone-drive/models";
import * as RemoteService from "../../bindings/rclone-drive/remoteservice";
import * as SystemService from "../../bindings/rclone-drive/systemservice";
import ProtocolIcon from "../components/ProtocolIcon.vue";
import WindowBar from "../components/WindowBar.vue";
import type { BackendDef } from "../types/backend";
import { cn } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from "@/components/ui/collapsible";
import { Field, FieldDescription, FieldGroup, FieldLabel, FieldLegend, FieldSeparator, FieldSet } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { Select, SelectContent, SelectGroup, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Spinner } from "@/components/ui/spinner";
import { Toaster } from "@/components/ui/sonner";
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group";
import { useTheme } from "../composables/useTheme";

// 编辑模式由窗口 URL 决定：#/drive?edit=<名称>
function editNameFromHash(): string {
  const hash = window.location.hash;
  const index = hash.indexOf("?");
  if (index < 0) return "";
  return new URLSearchParams(hash.slice(index + 1)).get("edit") ?? "";
}

const { isDark } = useTheme();
const editName = ref(editNameFromHash());
const editing = computed(() => editName.value !== "");

const backends = ref<BackendDef[]>([]);
const selectedType = ref("");
const defaultMountPoint = ref("Z:");
const name = ref("");
const mountPoint = ref("Z:");
const autoConnect = ref(false);
const readOnly = ref(false);
const options = ref<Record<string, string>>({});
const invalid = ref<Record<string, boolean>>({});
const submitting = ref(false);
const testing = ref(false);
const advancedOpen = ref(false);

const currentBackend = computed(() => backends.value.find((b) => b.type === selectedType.value));
const basicFields = computed(() => (currentBackend.value?.fields ?? []).filter((f) => !f.advanced));
const advancedFields = computed(() => (currentBackend.value?.fields ?? []).filter((f) => f.advanced));
const driveLetters = computed(() => Array.from({ length: 26 }, (_, i) => `${String.fromCharCode(90 - i)}:`));

function fieldId(key: string) {
  return `drive-field-${selectedType.value}-${key}`;
}

function errMsg(e: unknown) {
  return e instanceof Error ? e.message : String(e);
}

function resetForm() {
  name.value = "";
  mountPoint.value = defaultMountPoint.value;
  autoConnect.value = false;
  readOnly.value = false;
  invalid.value = {};
  advancedOpen.value = false;
  const opts: Record<string, string> = {};
  for (const f of currentBackend.value?.fields ?? []) {
    opts[f.key] = f.type === "bool" ? (f.default === "true" ? "true" : "false") : f.default;
  }
  options.value = opts;
}

function fillFrom(remote: Remote) {
  name.value = remote.name;
  mountPoint.value = remote.mountPoint || defaultMountPoint.value;
  autoConnect.value = remote.autoConnect;
  readOnly.value = remote.readOnly;
  invalid.value = {};
  advancedOpen.value = false;
  const opts: Record<string, string> = {};
  for (const f of currentBackend.value?.fields ?? []) {
    opts[f.key] = f.type === "bool" ? "false" : f.default;
  }
  for (const [key, value] of Object.entries(remote.options ?? {})) {
    if (value !== undefined) opts[key] = value;
  }
  options.value = opts;
}

async function load() {
  try {
    backends.value = ((await RemoteService.Backends()) ?? []) as BackendDef[];
    defaultMountPoint.value = await SystemService.DefaultMountPoint();

    if (editing.value) {
      const remotes = (await RemoteService.List()) ?? [];
      const remote = remotes.find((r) => r.name === editName.value);
      if (!remote) {
        toast.error(`驱动器不存在：${editName.value}`);
        void Window.Close();
        return;
      }
      selectedType.value = remote.type;
      fillFrom(remote);
      return;
    }

    if (backends.value.length > 0) selectedType.value = backends.value[0].type;
    resetForm();
  } catch (e) {
    toast.error(errMsg(e));
  }
}

// 非编辑模式下切换协议时重置表单
watch(selectedType, () => {
  if (!editing.value) resetForm();
});

onMounted(load);

function isInvalid(key: string) {
  return invalid.value[key] ? true : undefined;
}

function validate(): boolean {
  const errors: Record<string, boolean> = {};
  if (!name.value.trim()) errors.__name = true;
  for (const f of currentBackend.value?.fields ?? []) {
    if (f.required && !(options.value[f.key] ?? "").trim()) errors[f.key] = true;
  }
  invalid.value = errors;
  return Object.keys(errors).length === 0;
}

function buildRemote(): Remote {
  const opts: Record<string, string> = {};
  for (const [key, value] of Object.entries(options.value)) {
    if (value !== "") opts[key] = value;
  }
  return {
    name: name.value.trim(),
    type: selectedType.value,
    mountPoint: mountPoint.value,
    autoConnect: autoConnect.value,
    readOnly: readOnly.value,
    options: opts,
  };
}

async function persist() {
  if (editing.value) {
    await RemoteService.Update(buildRemote());
    return;
  }
  await RemoteService.Create(buildRemote());
}

async function onTest() {
  if (submitting.value || testing.value) return;
  if (!validate()) {
    toast.error("请填写所有必填项");
    return;
  }
  testing.value = true;
  try {
    await persist();
    await RemoteService.Test(name.value.trim());
    toast.success("连接成功");
  } catch (e) {
    toast.error(errMsg(e));
  } finally {
    testing.value = false;
  }
}

async function onSave() {
  if (submitting.value || testing.value) return;
  if (!validate()) {
    toast.error("请填写所有必填项");
    return;
  }
  submitting.value = true;
  try {
    await persist();
    // 保存成功后关闭窗口，主窗口会通过 remotes:changed 事件刷新列表
    void Window.Close();
  } catch (e) {
    toast.error(errMsg(e));
    submitting.value = false;
  }
}

function onCancel() {
  void Window.Close();
}
</script>

<template>
  <div class="bg-background flex h-screen flex-col">
    <WindowBar
      :title="editing ? '编辑驱动器' : '新虚拟驱动器'"
      :subtitle="editing ? '修改远程存储的连接信息' : '选择协议并填写远程存储的连接信息'"
    />

    <main class="flex-1 overflow-y-auto p-4">
      <ToggleGroup
        v-model="selectedType"
        type="single"
        variant="outline"
        :spacing="2"
        :disabled="editing"
        class="grid w-full grid-cols-3 gap-2"
      >
        <ToggleGroupItem
          v-for="backend in backends"
          :key="backend.type"
          :value="backend.type"
          class="data-[state=on]:ring-ring flex h-auto flex-col gap-2 py-3 data-[state=on]:ring-2"
        >
          <ProtocolIcon :type="backend.type" class="size-5" />
          <span class="text-sm">{{ backend.label }}</span>
        </ToggleGroupItem>
      </ToggleGroup>

      <div class="flex flex-col gap-6 pt-4 md:flex-row">
        <FieldGroup class="flex-1">
          <FieldSeparator>虚拟驱动器</FieldSeparator>
          <Field :data-invalid="isInvalid('__name')" :data-disabled="editing">
            <FieldLabel for="drive-name">名称</FieldLabel>
            <Input
              id="drive-name"
              v-model="name"
              :disabled="editing"
              :aria-invalid="isInvalid('__name')"
              placeholder="我的网盘"
            />
            <FieldDescription v-if="isInvalid('__name')">请输入驱动器名称。</FieldDescription>
            <FieldDescription v-else-if="editing">编辑模式下名称不可修改。</FieldDescription>
          </Field>
          <Field>
            <FieldLabel for="drive-letter">盘符</FieldLabel>
            <Select v-model="mountPoint">
              <SelectTrigger id="drive-letter">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectGroup>
                  <SelectItem v-for="letter in driveLetters" :key="letter" :value="letter">
                    {{ letter }}
                  </SelectItem>
                </SelectGroup>
              </SelectContent>
            </Select>
          </Field>
          <FieldSet>
            <FieldLegend variant="label">选项</FieldLegend>
            <FieldGroup class="gap-3">
              <Field orientation="horizontal">
                <Checkbox
                  id="drive-autoconnect"
                  :model-value="autoConnect"
                  @update:model-value="autoConnect = $event === true"
                />
                <FieldLabel for="drive-autoconnect" class="font-normal">自动连接</FieldLabel>
              </Field>
              <Field orientation="horizontal">
                <Checkbox
                  id="drive-readonly"
                  :model-value="readOnly"
                  @update:model-value="readOnly = $event === true"
                />
                <FieldLabel for="drive-readonly" class="font-normal">只读模式</FieldLabel>
              </Field>
            </FieldGroup>
          </FieldSet>
        </FieldGroup>

        <FieldGroup class="flex-1">
          <FieldSeparator>连接设置</FieldSeparator>
          <template v-for="f in basicFields" :key="f.key">
            <Field v-if="f.type === 'bool'" orientation="horizontal">
              <Checkbox
                :id="fieldId(f.key)"
                :model-value="options[f.key] === 'true'"
                @update:model-value="options[f.key] = $event === true ? 'true' : 'false'"
              />
              <FieldLabel :for="fieldId(f.key)" class="font-normal">{{ f.label }}</FieldLabel>
            </Field>
            <Field v-else-if="f.type === 'select'" :data-invalid="isInvalid(f.key)">
              <FieldLabel :for="fieldId(f.key)">{{ f.label }}</FieldLabel>
              <Select v-model="options[f.key]">
                <SelectTrigger :id="fieldId(f.key)" :aria-invalid="isInvalid(f.key)">
                  <SelectValue :placeholder="`请选择${f.label}`" />
                </SelectTrigger>
                <SelectContent>
                  <SelectGroup>
                    <SelectItem v-for="opt in f.options ?? []" :key="opt.value" :value="opt.value">
                      {{ opt.label }}
                    </SelectItem>
                  </SelectGroup>
                </SelectContent>
              </Select>
              <FieldDescription v-if="isInvalid(f.key)">此字段为必填项。</FieldDescription>
              <FieldDescription v-else-if="f.help">{{ f.help }}</FieldDescription>
            </Field>
            <Field v-else :data-invalid="isInvalid(f.key)">
              <FieldLabel :for="fieldId(f.key)">{{ f.label }}</FieldLabel>
              <Input
                :id="fieldId(f.key)"
                v-model="options[f.key]"
                :type="f.type === 'password' ? 'password' : f.type === 'int' ? 'number' : 'text'"
                :aria-invalid="isInvalid(f.key)"
              />
              <FieldDescription v-if="isInvalid(f.key)">此字段为必填项。</FieldDescription>
              <FieldDescription v-else-if="f.help">{{ f.help }}</FieldDescription>
            </Field>
          </template>

          <Collapsible v-if="advancedFields.length > 0" v-model:open="advancedOpen">
            <CollapsibleTrigger class="text-muted-foreground flex items-center gap-1 text-sm">
              <ChevronDown :class="cn('transition-transform', advancedOpen ? '' : '-rotate-90')" />
              高级选项
            </CollapsibleTrigger>
            <CollapsibleContent>
              <FieldGroup class="pt-4">
                <Field v-for="f in advancedFields" :key="f.key">
                  <FieldLabel :for="fieldId(f.key)">{{ f.label }}</FieldLabel>
                  <Input
                    :id="fieldId(f.key)"
                    v-model="options[f.key]"
                    :type="f.type === 'password' ? 'password' : f.type === 'int' ? 'number' : 'text'"
                  />
                  <FieldDescription v-if="f.help">{{ f.help }}</FieldDescription>
                </Field>
              </FieldGroup>
            </CollapsibleContent>
          </Collapsible>
        </FieldGroup>
      </div>
    </main>

    <footer class="border-t flex items-center justify-end gap-2 px-4 py-3">
      <Button variant="outline" :disabled="submitting || testing" @click="onTest">
        <Spinner v-if="testing" data-icon="inline-start" />
        测试连接
      </Button>
      <Button variant="outline" :disabled="submitting || testing" @click="onCancel">取消</Button>
      <Button :disabled="submitting || testing" @click="onSave">
        <Spinner v-if="submitting" data-icon="inline-start" />
        {{ editing ? "保存" : "连接" }}
      </Button>
    </footer>

    <Toaster :theme="isDark ? 'dark' : 'light'" position="top-center" />
  </div>
</template>

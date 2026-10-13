<script setup lang="ts">
import { computed, onMounted, onUnmounted, reactive, ref, watch } from "vue";
import { ChevronDown, CircleHelp, Search, TriangleAlert } from "@lucide/vue";
import { Events, Window } from "@wailsio/runtime";
import type { Field as RcloneField } from "../../bindings/rclone-drive/internal/rclone/models";
import type { BackendGroup, BackendType } from "../../bindings/rclone-drive/internal/rclone/models";
import type { Remote } from "../../bindings/rclone-drive/models";
import * as MountService from "../../bindings/rclone-drive/mountservice";
import * as RemoteService from "../../bindings/rclone-drive/remoteservice";
import * as SystemService from "../../bindings/rclone-drive/systemservice";
import { notifyError, notifySuccess } from "@/lib/notify";
import DriveField from "../components/DriveField.vue";
import ProtocolIcon from "../components/ProtocolIcon.vue";
import WindowBar from "../components/WindowBar.vue";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from "@/components/ui/collapsible";
import { Field, FieldDescription, FieldGroup, FieldLabel, FieldSeparator, FieldSet } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { Select, SelectContent, SelectGroup, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Spinner } from "@/components/ui/spinner";
import { Toaster } from "@/components/ui/sonner";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from "@/components/ui/tooltip";
import { useTheme } from "../composables/useTheme";

// 编辑模式由窗口 URL 决定：#/drive?edit=<名称>
const { isDark } = useTheme();
function editNameFromHash(): string {
  const hash = window.location.hash;
  const index = hash.indexOf("?");
  if (index < 0) return "";
  return new URLSearchParams(hash.slice(index + 1)).get("edit") ?? "";
}

const editName = ref(editNameFromHash());
const editing = computed(() => editName.value !== "");

const catalog = ref<BackendGroup[]>([]);
const selectedType = ref("");
const fields = ref<RcloneField[]>([]);
const tab = ref("basic");
const search = ref("");
const openGroups = reactive<Record<string, boolean>>({});
const defaultMountPoint = ref("Z:");
const remotes = ref<Remote[]>([]);
const usedLetters = ref<string[]>([]);
const name = ref("");
const mountPoint = ref("Z:");
const autoConnect = ref(false);
const readOnly = ref(false);
const insecure = ref(false);
const options = ref<Record<string, string>>({});
const invalid = ref<Record<string, boolean>>({});
const submitting = ref(false);
const testing = ref(false);

// 字段按类型缓存，切换类型时不用重复请求
const fieldCache = new Map<string, RcloneField[]>();

const allTypes = computed<BackendType[]>(() =>
  catalog.value.flatMap((group) => group.types ?? []).filter((item): item is BackendType => item !== null),
);
const currentType = computed<BackendType | undefined>(() => allTypes.value.find((t) => t.type === selectedType.value));
const docSlug = computed(() => selectedType.value.replace(/\s+/g, ""));

const searchResults = computed(() => {
  const keyword = search.value.trim().toLowerCase();
  if (!keyword) return [];
  return allTypes.value.filter((t) => t.label.toLowerCase().includes(keyword) || t.type.toLowerCase().includes(keyword));
});

// 选项带 Provider 过滤串时（如 s3 的各厂商专属选项），只展示与当前 provider 匹配的项
function providerMatches(filter: string, provider: string): boolean {
  const rules = filter
    .split(",")
    .map((rule) => rule.trim().toLowerCase())
    .filter(Boolean);
  if (rules.length === 0) return true;
  if (rules.some((rule) => rule.startsWith("!") && rule.slice(1) === provider)) return false;
  const positive = rules.filter((rule) => !rule.startsWith("!"));
  return positive.length === 0 || positive.includes(provider);
}

const visibleFields = computed(() => {
  const provider = (options.value.provider ?? "").toLowerCase();
  return fields.value.filter((field) => {
    // webdav / sftp / ftp 的部分字段由组合行单独渲染（见下方模板），不走通用渲染
    if (compositeKeys.value.includes(field.key)) return false;
    return !field.provider || providerMatches(field.provider, provider);
  });
});

// 组合行渲染的字段（按连接类型）：webdav 拆地址/服务商/凭据，sftp/ftp 合并主机+端口、用户名+密码
const compositeKeys = computed<string[]>(() => {
  switch (selectedType.value) {
    case "webdav":
      return ["url", "vendor", "user", "pass"];
    case "sftp":
    case "ftp":
      return ["host", "port", "user", "pass"];
    default:
      return [];
  }
});
const basicFields = computed(() => visibleFields.value.filter((f) => !f.advanced));
const advancedFields = computed(() => visibleFields.value.filter((f) => f.advanced));

// 连续的 bool 字段并排成一行（复选框 + 标签），非 bool 字段保持独占一行
type FieldSegment = { kind: "bools"; fields: RcloneField[] } | { kind: "field"; field: RcloneField };

function segmentFields(list: RcloneField[]): FieldSegment[] {
  const segments: FieldSegment[] = [];
  let bools: RcloneField[] = [];
  const flush = () => {
    if (bools.length) {
      segments.push({ kind: "bools", fields: bools });
      bools = [];
    }
  };
  for (const field of list) {
    if (field.type === "bool") {
      bools.push(field);
    } else {
      flush();
      segments.push({ kind: "field", field });
    }
  }
  flush();
  return segments;
}

const basicSegments = computed(() => segmentFields(basicFields.value));
const advancedSegments = computed(() => segmentFields(advancedFields.value));

// WebDAV：url 拆成 协议 / 地址 / 端口 三列，组合值仍写入 options.url，
// 校验、保存、测试连接沿用原有 url 字段的逻辑
const webdavProtocol = ref<"http" | "https">("http");
const webdavHost = ref("");
const webdavPort = ref("80");

const webdavUrlField = computed(() => fields.value.find((f) => f.key === "url"));

const webdavVendorField = computed(() => fields.value.find((f) => f.key === "vendor"));

// 服务商选项的 Label 由后端翻译为简短中文（value 仍是写入配置的标识符）
const webdavVendorOptions = computed(() => webdavVendorField.value?.examples ?? []);

const webdavUserField = computed(() => fields.value.find((f) => f.key === "user"));
const webdavPassField = computed(() => fields.value.find((f) => f.key === "pass"));

// sftp / ftp 组合行用到的字段
const hostField = computed(() => fields.value.find((f) => f.key === "host"));
const portField = computed(() => fields.value.find((f) => f.key === "port"));

function setOption(key: string, value: unknown) {
  options.value = { ...options.value, [key]: value == null ? "" : String(value) };
}

function defaultWebdavPort(protocol: string) {
  return protocol === "https" ? "443" : "80";
}

function splitWebdavUrl(url: string) {
  if (!url) {
    webdavProtocol.value = "http";
    webdavHost.value = "";
    webdavPort.value = "80";
    return;
  }
  try {
    const u = new URL(url);
    const protocol: "http" | "https" = u.protocol === "https:" ? "https" : "http";
    webdavProtocol.value = protocol;
    webdavHost.value = u.hostname;
    webdavPort.value = u.port || defaultWebdavPort(protocol);
  } catch {
    webdavProtocol.value = "http";
    webdavHost.value = url;
    webdavPort.value = "80";
  }
}

function joinWebdavUrl(): string {
  const host = webdavHost.value.trim();
  if (!host) return "";
  const port = webdavPort.value.trim() || defaultWebdavPort(webdavProtocol.value);
  return `${webdavProtocol.value}://${host}:${port}`;
}

function onWebdavProtocol(next: "http" | "https") {
  const prev = webdavProtocol.value;
  webdavProtocol.value = next;
  // 端口未手动填写、或仍是旧协议的默认值时，跟随协议切换默认端口
  if (!webdavPort.value.trim() || webdavPort.value === defaultWebdavPort(prev)) {
    webdavPort.value = defaultWebdavPort(next);
  }
}

watch([webdavProtocol, webdavHost, webdavPort], () => {
  if (selectedType.value !== "webdav") return;
  options.value = { ...options.value, url: joinWebdavUrl() };
});

const driveLetters = computed(() => {
  // 已被系统占用的盘符，以及分配给其他驱动器的盘符，都不再出现在下拉框里
  const taken = new Set(usedLetters.value);
  for (const remote of remotes.value) {
    if (remote.name === editName.value) continue;
    if (remote.mountPoint) taken.add(remote.mountPoint.toUpperCase());
  }
  return Array.from({ length: 26 }, (_, i) => `${String.fromCharCode(90 - i)}:`).filter((letter) => !taken.has(letter));
});

function errMsg(e: unknown) {
  return e instanceof Error ? e.message : String(e);
}

function isInvalid(key: string) {
  return invalid.value[key] ? true : undefined;
}

// 默认值来自 rclone 的字段元数据
function seedOptions(keepExisting: boolean) {
  const opts: Record<string, string> = {};
  for (const field of fields.value) {
    if (keepExisting && options.value[field.key] !== undefined) {
      opts[field.key] = options.value[field.key];
      continue;
    }
    opts[field.key] = field.default ?? "";
  }
  options.value = opts;
}

function resetForm() {
  name.value = "";
  mountPoint.value = defaultMountPoint.value;
  autoConnect.value = false;
  readOnly.value = false;
  insecure.value = false;
  invalid.value = {};
  seedOptions(false);
  // 默认盘符可能被其他驱动器占用，落到第一个可用项
  if (!editing.value && driveLetters.value.length > 0 && !driveLetters.value.includes(mountPoint.value)) {
    mountPoint.value = driveLetters.value[0];
  }
  if (selectedType.value === "webdav") splitWebdavUrl(options.value.url ?? "");
}

function fillFrom(remote: Remote) {
  name.value = remote.name;
  mountPoint.value = remote.mountPoint || defaultMountPoint.value;
  autoConnect.value = remote.autoConnect;
  readOnly.value = remote.readOnly;
  insecure.value = remote.insecure;
  invalid.value = {};
  seedOptions(false);
  // 凭据不落 remotes.json：配置项从 rclone.conf 回读（密码字段后端已解密）
  RemoteService.Options(remote.name)
    .then((stored) => {
      const opts = { ...options.value };
      for (const [key, value] of Object.entries(stored ?? {})) {
        if (value !== undefined) opts[key] = value;
      }
      options.value = opts;
      splitWebdavUrl(opts.url ?? "");
    })
    .catch((e) => notifyError(errMsg(e), "读取驱动器配置失败"));
}

async function loadFields(typ: string) {
  const cached = fieldCache.get(typ);
  if (cached) {
    fields.value = cached;
    return;
  }
  const list = (await RemoteService.BackendFields(typ)) ?? [];
  fieldCache.set(typ, list);
  fields.value = list;
}

// 切换连接类型：新增模式下重置表单，编辑模式不允许切换
async function selectType(typ: string) {
  if (editing.value || typ === selectedType.value) return;
  selectedType.value = typ;
  try {
    await loadFields(typ);
  } catch (e) {
    notifyError(errMsg(e), "加载连接类型字段失败");
    return;
  }
  resetForm();
}

async function load() {
  try {
    if (catalog.value.length === 0) {
      catalog.value = (await RemoteService.BackendCatalog()) ?? [];
      resetGroupState();
    }
    defaultMountPoint.value = await SystemService.DefaultMountPoint();
    usedLetters.value = (await SystemService.UsedDriveLetters()) ?? [];
    remotes.value = (await RemoteService.List()) ?? [];

    if (editing.value) {
      const remote = remotes.value.find((r) => r.name === editName.value);
      if (!remote) {
        notifyError(`驱动器不存在：${editName.value}`, "打开驱动器失败");
        void Window.Close();
        return;
      }
      selectedType.value = remote.type;
      await loadFields(remote.type);
      fillFrom(remote);
      return;
    }

    const first = allTypes.value[0]?.type ?? "";
    if (first) {
      selectedType.value = first;
      await loadFields(first);
    }
    resetForm();
  } catch (e) {
    notifyError(errMsg(e), "加载驱动器信息失败");
  }
}

let offDriveOpen: (() => void) | undefined;
const scrollArea = ref<HTMLElement | null>(null);

// 分组默认全部收起，只展开「常用协议」（分组顺序里的第一个）
function resetGroupState() {
  catalog.value.forEach((group, index) => {
    openGroups[group.label] = index === 0;
  });
}

// 窗口是复用的（关闭只隐藏、不销毁），重新打开时要清掉上一次遗留的进行中状态与滚动位置
function resetTransientState() {
  submitting.value = false;
  testing.value = false;
  invalid.value = {};
  tab.value = "basic";
  search.value = "";
  resetGroupState();
  if (scrollArea.value) scrollArea.value.scrollTop = 0;
}

onMounted(async () => {
  await load();
  // 窗口被复用（之前已打开过）时，主窗口只发 drive:open 事件切换目标，不再重新加载页面
  offDriveOpen = Events.On("drive:open", (event) => {
    editName.value = (event.data as { name?: string } | undefined)?.name ?? "";
    resetTransientState();
    void load();
  });
});

onUnmounted(() => offDriveOpen?.());

function validate(): boolean {
  const errors: Record<string, boolean> = {};
  if (!name.value.trim()) errors.__name = true;
  for (const field of fields.value) {
    if (field.required && !(options.value[field.key] ?? "").trim()) errors[field.key] = true;
  }
  invalid.value = errors;
  if (Object.keys(errors).length > 0) tab.value = "basic";
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
    insecure: insecure.value,
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
    notifyError("请填写所有必填项", "无法测试连接");
    return;
  }
  testing.value = true;
  try {
    await persist();
    await RemoteService.Test(name.value.trim());
    notifySuccess("连接成功", `「${name.value.trim()}」连接测试通过`);
  } catch (e) {
    notifyError(errMsg(e), "连接测试失败");
  } finally {
    testing.value = false;
  }
}

// 仅保存配置：不建立连接，新增模式下关闭窗口后主界面会显示新驱动器
async function onSave() {
  if (submitting.value || testing.value) return;
  if (!validate()) {
    notifyError("请填写所有必填项", "无法保存");
    return;
  }
  submitting.value = true;
  try {
    await persist();
    // 保存成功后隐藏窗口（子窗口只隐藏不销毁）；先复位状态，避免复用时按钮停在禁用态
    submitting.value = false;
    // 主窗口会通过 remotes:changed 事件刷新列表
    void Window.Close();
  } catch (e) {
    notifyError(errMsg(e), editing.value ? "保存失败" : "创建失败");
    submitting.value = false;
  }
}

// 保存并立即连接：先写入配置，再挂载该驱动器
async function onSaveAndConnect() {
  if (submitting.value || testing.value) return;
  if (!validate()) {
    notifyError("请填写所有必填项", "无法保存");
    return;
  }
  submitting.value = true;
  const driveName = name.value.trim();
  try {
    await persist();
  } catch (e) {
    notifyError(errMsg(e), editing.value ? "保存失败" : "创建失败");
    submitting.value = false;
    return;
  }
  try {
    await MountService.Mount(driveName);
    notifySuccess("已连接", `「${driveName}」已挂载`);
  } catch (e) {
    // 配置已经写入，只是挂载失败：照常关闭窗口，从主界面重试连接
    notifyError(errMsg(e), "已保存，但连接失败");
  } finally {
    submitting.value = false;
    void Window.Close();
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
      :subtitle="editing ? '修改远程存储的连接信息' : '选择连接类型并填写连接信息'"
    />

    <div class="flex min-h-0 flex-1">
      <!-- 左侧：连接类型树 -->
      <aside class="flex w-52 shrink-0 flex-col border-r">
        <div class="p-2">
          <div class="relative">
            <Search class="text-muted-foreground pointer-events-none absolute top-1/2 left-2 size-3.5 -translate-y-1/2" />
            <Input
              v-model="search"
              placeholder="搜索连接类型"
              class="h-7 pl-7 text-xs"
              :disabled="editing"
              aria-label="搜索连接类型"
            />
          </div>
        </div>

        <div class="no-scrollbar min-h-0 flex-1 overflow-y-auto px-1.5 pb-3">
          <!-- 搜索状态：扁平结果 -->
          <template v-if="search.trim()">
            <p v-if="searchResults.length === 0" class="text-muted-foreground px-1.5 py-2 text-xs">没有匹配的连接类型</p>
            <button
              v-for="item in searchResults"
              :key="item.type"
              type="button"
              class="hover:bg-muted flex w-full items-center gap-2 rounded-md px-1.5 py-1.5 text-left text-xs disabled:opacity-50"
              :class="item.type === selectedType ? 'bg-muted font-medium' : ''"
              :disabled="editing"
              @click="selectType(item.type)"
            >
              <ProtocolIcon :type="item.type" class="size-4 shrink-0" />
              <span class="truncate">{{ item.label }}</span>
              <Badge v-if="item.authorized" variant="outline" class="ml-auto shrink-0 px-1 text-[10px]">需授权</Badge>
            </button>
          </template>

          <!-- 默认状态：分组树 -->
          <template v-else>
            <Collapsible
              v-for="group in catalog"
              :key="group.label"
              :open="openGroups[group.label]"
              @update:open="openGroups[group.label] = $event"
            >
              <CollapsibleTrigger
                class="text-muted-foreground hover:text-foreground flex w-full items-center gap-1 rounded-md px-1.5 py-1.5 text-xs font-medium"
              >
                <ChevronDown class="size-3.5 transition-transform" :class="openGroups[group.label] ? '' : '-rotate-90'" />
                <span>{{ group.label }}</span>
                <span class="text-muted-foreground/70 ml-auto text-[10px]">{{ group.types?.length ?? 0 }}</span>
              </CollapsibleTrigger>
              <CollapsibleContent>
                <button
                  v-for="item in group.types ?? []"
                  :key="item.type"
                  type="button"
                  class="hover:bg-muted flex w-full items-center gap-2 rounded-md px-1.5 py-1.5 pl-3 text-left text-xs disabled:opacity-50"
                  :class="item.type === selectedType ? 'bg-muted font-medium' : ''"
                  :disabled="editing"
                  @click="selectType(item.type)"
                >
                  <ProtocolIcon :type="item.type" class="size-4 shrink-0" />
                  <span class="truncate">{{ item.label }}</span>
                  <Badge v-if="item.authorized" variant="outline" class="ml-auto shrink-0 px-1 text-[10px]">需授权</Badge>
                </button>
              </CollapsibleContent>
            </Collapsible>
          </template>
        </div>
      </aside>

      <!-- 右侧：配置 -->
      <section class="flex min-w-0 flex-1 flex-col">
        <TooltipProvider>
        <div class="flex shrink-0 items-center gap-2 border-b px-4 py-2.5">
          <ProtocolIcon :type="selectedType" class="size-4 shrink-0" />
          <span class="truncate text-sm font-medium">{{ currentType?.label ?? selectedType }}</span>
          <Badge v-if="currentType?.authorized" variant="outline" class="shrink-0 px-1.5 text-[10px]">需授权</Badge>
        </div>

        <Tabs v-model="tab" class="min-h-0 flex-1">
          <div class="shrink-0 px-4 pt-3">
            <TabsList>
              <TabsTrigger value="basic">基本</TabsTrigger>
              <TabsTrigger value="advanced">高级<span v-if="advancedFields.length">·{{ advancedFields.length }}</span></TabsTrigger>
              <TabsTrigger value="about">说明</TabsTrigger>
            </TabsList>
          </div>

          <div ref="scrollArea" class="no-scrollbar min-h-0 flex-1 overflow-y-auto px-4 pt-3 pb-4">
            <TabsContent value="basic">
              <FieldGroup>
                <Field :data-invalid="isInvalid('__name')" :data-disabled="editing">
                  <FieldLabel for="drive-name" class="flex items-center gap-1.5">
                    名称
                    <Tooltip v-if="!isInvalid('__name') && editing">
                      <TooltipTrigger as-child>
                        <button type="button" class="cursor-help text-muted-foreground transition-colors hover:text-foreground" @click.stop>
                          <CircleHelp class="size-3.5" />
                        </button>
                      </TooltipTrigger>
                      <TooltipContent side="top" class="max-w-56">编辑模式下名称不可修改。</TooltipContent>
                    </Tooltip>
                  </FieldLabel>
                  <Input
                    id="drive-name"
                    v-model="name"
                    :disabled="editing"
                    :aria-invalid="isInvalid('__name')"
                    placeholder="我的网盘"
                  />
                  <FieldDescription v-if="isInvalid('__name')">请输入驱动器名称。</FieldDescription>
                </Field>

                <div class="flex items-start gap-2">
                  <Field :class="selectedType === 'webdav' ? 'w-28 shrink-0' : 'min-w-0 flex-1'">
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

                  <Field v-if="selectedType === 'webdav'" class="min-w-0 flex-1">
                    <FieldLabel for="drive-vendor" class="flex items-center gap-1.5">
                      服务商
                      <Tooltip v-if="webdavVendorField?.help">
                        <TooltipTrigger as-child>
                          <button type="button" class="cursor-help text-muted-foreground transition-colors hover:text-foreground" @click.stop>
                            <CircleHelp class="size-3.5" />
                          </button>
                        </TooltipTrigger>
                        <TooltipContent side="top" class="max-w-56 whitespace-pre-line">{{ webdavVendorField.help }}</TooltipContent>
                      </Tooltip>
                    </FieldLabel>
                    <Select :model-value="options.vendor ?? ''" @update:model-value="setOption('vendor', $event)">
                      <SelectTrigger id="drive-vendor">
                        <SelectValue placeholder="选择服务商" />
                      </SelectTrigger>
                      <SelectContent>
                        <SelectItem v-for="opt in webdavVendorOptions" :key="opt.value" :value="opt.value">
                          {{ opt.label }}
                        </SelectItem>
                      </SelectContent>
                    </Select>
                  </Field>
                </div>

                <FieldSet class="flex-row flex-wrap items-center gap-x-5 gap-y-2">
                  <Field orientation="horizontal" class="w-auto">
                    <Checkbox
                      id="drive-autoconnect"
                      :model-value="autoConnect"
                      @update:model-value="autoConnect = $event === true"
                    />
                    <FieldLabel for="drive-autoconnect" class="font-normal">自动连接</FieldLabel>
                  </Field>
                  <Field orientation="horizontal" class="w-auto">
                    <Checkbox
                      id="drive-readonly"
                      :model-value="readOnly"
                      @update:model-value="readOnly = $event === true"
                    />
                    <FieldLabel for="drive-readonly" class="font-normal">只读模式</FieldLabel>
                  </Field>
                  <Field orientation="horizontal" class="w-auto">
                    <Checkbox
                      id="drive-insecure"
                      :model-value="insecure"
                      @update:model-value="insecure = $event === true"
                    />
                    <FieldLabel for="drive-insecure" class="flex items-center gap-1.5 font-normal">
                      忽略证书错误
                      <Tooltip>
                        <TooltipTrigger as-child>
                          <button type="button" class="cursor-help text-muted-foreground transition-colors hover:text-foreground" @click.stop>
                            <CircleHelp class="size-3.5" />
                          </button>
                        </TooltipTrigger>
                        <TooltipContent side="top" class="max-w-56">
                          自签名证书的 HTTPS 服务器无法校验证书时勾选。注意：开启后该驱动器的流量不再受 TLS 保护，存在被窃听、篡改的风险
                        </TooltipContent>
                      </Tooltip>
                    </FieldLabel>
                  </Field>
                </FieldSet>

                <div
                  v-if="insecure"
                  class="mt-1 flex items-start gap-2 rounded-md border border-amber-500/40 bg-amber-500/10 px-3 py-2 text-xs text-amber-600 dark:text-amber-400"
                >
                  <TriangleAlert class="mt-0.5 size-3.5 shrink-0" />
                  <span>
                    安全警告：启用后该驱动器的 HTTPS 流量将不再校验证书，同一网络下的攻击者可能窃听或篡改传输内容（包括账号密码）。仅建议对使用自签名证书的自建服务器开启。
                  </span>
                </div>

                <FieldSeparator>连接设置</FieldSeparator>

                <Field v-if="selectedType === 'webdav'" :data-invalid="isInvalid('url')">
                  <FieldLabel class="flex items-center gap-1.5">
                    地址
                    <Tooltip v-if="!isInvalid('url') && webdavUrlField?.help">
                      <TooltipTrigger as-child>
                        <button type="button" class="cursor-help text-muted-foreground transition-colors hover:text-foreground" @click.stop>
                          <CircleHelp class="size-3.5" />
                        </button>
                      </TooltipTrigger>
                      <TooltipContent side="top" class="max-w-56 whitespace-pre-line">{{ webdavUrlField.help }}</TooltipContent>
                    </Tooltip>
                  </FieldLabel>
                  <div class="flex items-center gap-2">
                    <Select :model-value="webdavProtocol" @update:model-value="onWebdavProtocol($event as 'http' | 'https')">
                      <SelectTrigger class="w-28" aria-label="协议">
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        <SelectItem value="http">http</SelectItem>
                        <SelectItem value="https">https</SelectItem>
                      </SelectContent>
                    </Select>
                    <Input v-model="webdavHost" placeholder="IP / 域名" class="min-w-0 flex-1" aria-label="地址" />
                    <Input v-model="webdavPort" placeholder="端口" class="w-20" aria-label="端口" />
                  </div>
                  <FieldDescription v-if="isInvalid('url')">请填写完整的地址信息。</FieldDescription>
                </Field>

                <div v-if="(selectedType === 'sftp' || selectedType === 'ftp') && hostField && portField" class="flex items-start gap-2">
                  <div class="min-w-0 flex-1">
                    <DriveField
                      v-model="options.host"
                      :field="hostField"
                      :scope="`drive-${selectedType}-basic`"
                      :invalid="Boolean(isInvalid('host'))"
                    />
                  </div>
                  <div class="w-28 shrink-0">
                    <DriveField
                      v-model="options.port"
                      :field="portField"
                      :scope="`drive-${selectedType}-basic`"
                      :invalid="Boolean(isInvalid('port'))"
                    />
                  </div>
                </div>

                <div
                  v-if="['webdav', 'sftp', 'ftp'].includes(selectedType) && webdavUserField && webdavPassField"
                  class="flex items-start gap-2"
                >
                  <div class="min-w-0 flex-1">
                    <DriveField
                      v-model="options.user"
                      :field="webdavUserField"
                      :scope="`drive-${selectedType}-basic`"
                      :invalid="Boolean(isInvalid('user'))"
                    />
                  </div>
                  <div class="min-w-0 flex-1">
                    <DriveField
                      v-model="options.pass"
                      :field="webdavPassField"
                      :scope="`drive-${selectedType}-basic`"
                      :invalid="Boolean(isInvalid('pass'))"
                    />
                  </div>
                </div>

                <template v-for="(seg, i) in basicSegments" :key="seg.kind === 'field' ? seg.field.key : `bools-${i}`">
                  <div v-if="seg.kind === 'bools'" class="flex flex-wrap items-center gap-x-5 gap-y-2">
                    <DriveField
                      v-for="f in seg.fields"
                      :key="f.key"
                      v-model="options[f.key]"
                      :field="f"
                      :scope="`drive-${selectedType}-basic`"
                      :invalid="Boolean(isInvalid(f.key))"
                    />
                  </div>
                  <DriveField
                    v-else
                    v-model="options[seg.field.key]"
                    :field="seg.field"
                    :scope="`drive-${selectedType}-basic`"
                    :invalid="Boolean(isInvalid(seg.field.key))"
                  />
                </template>
              </FieldGroup>
            </TabsContent>

            <TabsContent value="advanced">
              <FieldGroup>
                <p v-if="advancedFields.length === 0" class="text-muted-foreground text-xs">该连接类型没有高级选项。</p>
                <template v-for="(seg, i) in advancedSegments" :key="seg.kind === 'field' ? seg.field.key : `bools-${i}`">
                  <div v-if="seg.kind === 'bools'" class="flex flex-wrap items-center gap-x-5 gap-y-2">
                    <DriveField
                      v-for="f in seg.fields"
                      :key="f.key"
                      v-model="options[f.key]"
                      :field="f"
                      :scope="`drive-${selectedType}-advanced`"
                      :invalid="Boolean(isInvalid(f.key))"
                    />
                  </div>
                  <DriveField
                    v-else
                    v-model="options[seg.field.key]"
                    :field="seg.field"
                    :scope="`drive-${selectedType}-advanced`"
                    :invalid="Boolean(isInvalid(seg.field.key))"
                  />
                </template>
              </FieldGroup>
            </TabsContent>

            <TabsContent value="about">
              <div class="flex flex-col gap-3 text-xs">
                <p class="text-muted-foreground whitespace-pre-line">{{ currentType?.help }}</p>
                <FieldSeparator>文档</FieldSeparator>
                <a
                  class="text-primary font-medium underline"
                  :href="`https://rclone.org/${docSlug}/`"
                  target="_blank"
                  rel="noreferrer"
                >
                  rclone.org/{{ docSlug }}/
                </a>
                <template v-if="currentType?.authorized">
                  <FieldSeparator>需要授权</FieldSeparator>
                  <p class="text-muted-foreground">
                    该连接类型需要先完成 OAuth 授权，本界面不提供浏览器授权流程。可以先在其它机器上运行
                    <code class="bg-muted rounded px-1 py-0.5">rclone authorize "{{ selectedType }}"</code>
                    得到 token，再把它粘贴到「高级」里的「授权令牌（token）」；已有的 rclone 配置也可以直接复用。
                  </p>
                </template>
              </div>
            </TabsContent>
          </div>
        </Tabs>
        </TooltipProvider>
      </section>
    </div>

    <footer class="border-t flex shrink-0 items-center justify-end gap-2 px-4 py-3">
      <Button variant="outline" :disabled="submitting || testing" @click="onTest">
        <Spinner v-if="testing" data-icon="inline-start" />
        测试连接
      </Button>
      <Button variant="outline" :disabled="submitting || testing" @click="onCancel">取消</Button>
      <template v-if="editing">
        <Button :disabled="submitting || testing" @click="onSave">
          <Spinner v-if="submitting" data-icon="inline-start" />
          保存
        </Button>
      </template>
      <template v-else>
        <!-- 新增：保存配置不连接，或保存后立即挂载 -->
        <Button variant="outline" :disabled="submitting || testing" @click="onSave">
          <Spinner v-if="submitting" data-icon="inline-start" />
          保存
        </Button>
        <Button :disabled="submitting || testing" @click="onSaveAndConnect">
          <Spinner v-if="submitting" data-icon="inline-start" />
          保存并连接
        </Button>
      </template>
    </footer>

    <Toaster :theme="isDark ? 'dark' : 'light'" position="top-center" />
  </div>
</template>

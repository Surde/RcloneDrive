<script setup lang="ts">
import { computed, ref } from "vue";
import { FolderOpen, Pencil, Plug, Trash, Unplug } from "@lucide/vue";
import type { MountStatus, Remote } from "../../bindings/rclone-drive/models";
import * as MountService from "../../bindings/rclone-drive/mountservice";
import * as RemoteService from "../../bindings/rclone-drive/remoteservice";
import * as WindowService from "../../bindings/rclone-drive/windowservice";
import { formatSize, formatSpeed } from "@/lib/format";
import { notifyError, notifySuccess } from "@/lib/notify";
import { useTypeLabels } from "@/composables/useTypeLabels";
import ProtocolIcon from "./ProtocolIcon.vue";
import { AlertDialog, AlertDialogAction, AlertDialogCancel, AlertDialogContent, AlertDialogDescription, AlertDialogFooter, AlertDialogHeader, AlertDialogTitle, AlertDialogTrigger } from "@/components/ui/alert-dialog";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Spinner } from "@/components/ui/spinner";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";

const props = defineProps<{
  remote: Remote;
  status: MountStatus | undefined;
  /** 实时速率（字节/秒）：down = 从挂载盘读取，up = 写入挂载盘 */
  speed?: { down?: number; up?: number };
  /** 挂载盘的容量信息；后端不支持或未取到时为空 */
  usage?: { total?: number; used?: number; free?: number };
}>();

const downSpeed = computed(() => props.speed?.down ?? 0);
const upSpeed = computed(() => props.speed?.up ?? 0);

// 已用比例（进度条填充）
const usageTotal = computed(() => props.usage?.total ?? 0);
const usedPercent = computed(() => {
  const total = usageTotal.value;
  if (total <= 0) return 0;
  const used = props.usage?.used ?? 0;
  return Math.min(100, Math.max(0, Math.round((used / total) * 100)));
});
const showUsage = computed(() => mounted.value && usageTotal.value > 0);

const emit = defineEmits<{
  changed: [];
}>();

const busy = ref(false);
const editConfirmOpen = ref(false);
const disconnecting = ref(false);
const mounted = computed(() => props.status?.mounted ?? false);
const { label: typeLabel } = useTypeLabels();

function message(e: unknown) {
  return e instanceof Error ? e.message : String(e);
}

async function toggle() {
  if (busy.value) return;
  busy.value = true;
  try {
    if (mounted.value) {
      await MountService.Unmount(props.remote.name);
      notifySuccess("已断开", `「${props.remote.name}」已断开连接`);
    } else {
      await MountService.Mount(props.remote.name);
      notifySuccess("已连接", `「${props.remote.name}」已挂载`);
    }
    emit("changed");
  } catch (e) {
    notifyError(message(e), mounted.value ? "断开失败" : "挂载失败");
  } finally {
    busy.value = false;
  }
}

// 打开已连接驱动器的挂载目录（资源管理器 / 访达 / 文件管理器）；图标只在已连接时显示
async function openFolder() {
  try {
    await MountService.OpenFolder(props.remote.name);
  } catch (e) {
    notifyError(message(e), "打开文件夹失败");
  }
}

// 编辑已连接的驱动器会改写正在使用的配置，需要先断开连接：先确认，再自动断开并打开编辑窗口
function edit() {
  if (!mounted.value) {
    void WindowService.OpenEditDrive(props.remote.name);
    return;
  }
  editConfirmOpen.value = true;
}

async function disconnectAndEdit() {
  if (disconnecting.value) return;
  disconnecting.value = true;
  try {
    await MountService.Unmount(props.remote.name);
    notifySuccess("已断开", `「${props.remote.name}」已断开连接`);
    emit("changed");
    editConfirmOpen.value = false;
    void WindowService.OpenEditDrive(props.remote.name);
  } catch (e) {
    notifyError(message(e), "断开失败");
  } finally {
    disconnecting.value = false;
  }
}

async function remove() {
  try {
    // 删除成功不弹通知：主界面列表刷新即可，避免多余打扰
    await RemoteService.Delete(props.remote.name);
    emit("changed");
  } catch (e) {
    notifyError(message(e), "删除失败");
  }
}
</script>

<template>
  <Card class="gap-0 py-0">
    <CardContent class="flex items-center gap-2.5 px-3 py-3">
      <div class="bg-muted text-muted-foreground flex size-9 shrink-0 items-center justify-center rounded-full">
        <ProtocolIcon :type="remote.type" class="size-4" />
      </div>
      <div class="flex min-w-0 flex-1 flex-col gap-1">
        <div class="flex items-center gap-1.5">
          <!-- 盘符始终显示在名称前面（未连接时用配置里的盘符） -->
          <Badge v-if="remote.mountPoint" variant="secondary" class="shrink-0 px-1.5 font-mono text-[10px]">
            {{ remote.mountPoint }}
          </Badge>
          <span class="truncate font-medium">{{ remote.name }}</span>
          <Badge variant="outline" class="shrink-0 px-1.5 text-[10px]">{{ typeLabel(remote.type) }}</Badge>
        </div>

        <!-- 第 2 行：状态 + 容量（进度条、用量）+ 速率（固定宽度） -->
        <div class="flex items-center gap-2 text-xs leading-5">
          <span class="text-muted-foreground shrink-0">{{ mounted ? "已挂载" : "未连接" }}</span>

          <template v-if="mounted && showUsage">
            <div class="bg-muted h-1.5 min-w-12 flex-1 overflow-hidden rounded-full">
              <div class="bg-primary h-full rounded-full transition-[width]" :style="{ width: `${usedPercent}%` }" />
            </div>
            <span class="text-muted-foreground min-w-0 truncate text-[10px] tabular-nums">
              {{ formatSize(usage?.free ?? 0) }} 可用，共 {{ formatSize(usageTotal) }}
            </span>
          </template>
          <span v-else class="min-w-0 flex-1" />

          <div
            v-if="mounted"
            class="text-muted-foreground flex w-24 shrink-0 flex-col justify-center text-right text-[10px] leading-[13px] tabular-nums"
          >
            <span v-if="downSpeed > 0" class="truncate">↓ {{ formatSpeed(downSpeed) }}</span>
            <span v-if="upSpeed > 0" class="truncate">↑ {{ formatSpeed(upSpeed) }}</span>
          </div>
        </div>
      </div>
      <div class="flex shrink-0 items-center gap-0.5">
        <!-- 打开文件夹：未连接时用 invisible 占位，保证图标组宽度不变，连接后名称面板不会伸缩 -->
        <Tooltip>
          <TooltipTrigger as-child>
            <Button
              variant="ghost"
              size="icon-sm"
              :disabled="busy || !mounted"
              :class="mounted ? '' : 'invisible'"
              aria-label="打开文件夹"
              @click="openFolder"
            >
              <FolderOpen />
            </Button>
          </TooltipTrigger>
          <TooltipContent>打开文件夹</TooltipContent>
        </Tooltip>
        <Tooltip>
          <TooltipTrigger as-child>
            <Button variant="ghost" size="icon-sm" :disabled="busy" @click="toggle">
              <Spinner v-if="busy" data-icon="inline-start" />
              <Unplug v-else-if="mounted" data-icon="inline-start" />
              <Plug v-else data-icon="inline-start" />
            </Button>
          </TooltipTrigger>
          <TooltipContent>{{ mounted ? "断开" : "挂载" }}</TooltipContent>
        </Tooltip>
        <Tooltip>
          <TooltipTrigger as-child>
            <Button variant="ghost" size="icon-sm" :disabled="busy" @click="edit">
              <Pencil data-icon="inline-start" />
            </Button>
          </TooltipTrigger>
          <TooltipContent>{{ mounted ? "编辑（需先断开连接）" : "编辑" }}</TooltipContent>
        </Tooltip>
        <AlertDialog v-model:open="editConfirmOpen">
          <AlertDialogContent>
            <AlertDialogHeader>
              <AlertDialogTitle>编辑已连接的驱动器</AlertDialogTitle>
              <AlertDialogDescription>
                「{{ remote.name }}」当前已连接，编辑前需要先断开连接。是否断开连接并打开编辑窗口？
              </AlertDialogDescription>
            </AlertDialogHeader>
            <AlertDialogFooter>
              <AlertDialogCancel :disabled="disconnecting">取消</AlertDialogCancel>
              <AlertDialogAction :disabled="disconnecting" @click="disconnectAndEdit">
                <Spinner v-if="disconnecting" data-icon="inline-start" />
                断开并编辑
              </AlertDialogAction>
            </AlertDialogFooter>
          </AlertDialogContent>
        </AlertDialog>
        <AlertDialog>
          <Tooltip>
            <TooltipTrigger as-child>
              <AlertDialogTrigger as-child>
                <Button variant="ghost" size="icon-sm" :disabled="busy">
                  <Trash data-icon="inline-start" />
                </Button>
              </AlertDialogTrigger>
            </TooltipTrigger>
            <TooltipContent>删除</TooltipContent>
          </Tooltip>
          <AlertDialogContent>
            <AlertDialogHeader>
              <AlertDialogTitle>删除驱动器</AlertDialogTitle>
              <AlertDialogDescription>
                确定要删除「{{ remote.name }}」吗？相关的远程存储配置将被移除。
              </AlertDialogDescription>
            </AlertDialogHeader>
            <AlertDialogFooter>
              <AlertDialogCancel>取消</AlertDialogCancel>
              <AlertDialogAction variant="destructive" @click="remove">删除</AlertDialogAction>
            </AlertDialogFooter>
          </AlertDialogContent>
        </AlertDialog>
      </div>
    </CardContent>
  </Card>
</template>

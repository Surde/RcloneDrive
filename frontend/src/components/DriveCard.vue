<script setup lang="ts">
import { computed, ref } from "vue";
import { toast } from "vue-sonner";
import { Plug, Trash, Unplug } from "@lucide/vue";
import type { MountStatus, Remote } from "../../bindings/rclone-drive/models";
import * as MountService from "../../bindings/rclone-drive/mountservice";
import * as RemoteService from "../../bindings/rclone-drive/remoteservice";
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
}>();

const emit = defineEmits<{
  changed: [];
}>();

const busy = ref(false);
const mounted = computed(() => props.status?.mounted ?? false);

async function toggle() {
  if (busy.value) return;
  busy.value = true;
  try {
    if (mounted.value) {
      await MountService.Unmount(props.remote.name);
      toast.success(`已断开「${props.remote.name}」`);
    } else {
      await MountService.Mount(props.remote.name);
      toast.success(`已挂载「${props.remote.name}」`);
    }
    emit("changed");
  } catch (e) {
    toast.error(e instanceof Error ? e.message : String(e));
  } finally {
    busy.value = false;
  }
}

async function remove() {
  try {
    await RemoteService.Delete(props.remote.name);
    toast.success(`已删除「${props.remote.name}」`);
    emit("changed");
  } catch (e) {
    toast.error(e instanceof Error ? e.message : String(e));
  }
}
</script>

<template>
  <Card>
    <CardContent class="flex items-center gap-4 p-4">
      <div class="bg-muted text-muted-foreground flex size-11 shrink-0 items-center justify-center rounded-full">
        <ProtocolIcon :type="remote.type" class="size-5" />
      </div>
      <div class="flex min-w-0 flex-1 flex-col gap-1.5">
        <div class="flex items-center gap-2">
          <span class="truncate font-medium">{{ remote.name }}</span>
          <Badge variant="outline">{{ remote.type.toUpperCase() }}</Badge>
        </div>
        <div class="text-muted-foreground flex items-center gap-2 text-sm">
          <Badge v-if="mounted" variant="secondary">已挂载 · {{ remote.mountPoint }}</Badge>
          <span v-else>未连接</span>
        </div>
      </div>
      <div class="flex shrink-0 items-center gap-1">
        <Tooltip>
          <TooltipTrigger as-child>
            <Button variant="ghost" size="icon" :disabled="busy" @click="toggle">
              <Spinner v-if="busy" data-icon="inline-start" />
              <Unplug v-else-if="mounted" data-icon="inline-start" />
              <Plug v-else data-icon="inline-start" />
            </Button>
          </TooltipTrigger>
          <TooltipContent>{{ mounted ? "断开" : "挂载" }}</TooltipContent>
        </Tooltip>
        <AlertDialog>
          <Tooltip>
            <TooltipTrigger as-child>
              <AlertDialogTrigger as-child>
                <Button variant="ghost" size="icon" :disabled="busy">
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

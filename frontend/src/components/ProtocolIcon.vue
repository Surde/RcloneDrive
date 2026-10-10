<script setup lang="ts">
import type { Component } from "vue";
import {
  Archive,
  Cloud,
  Database,
  Folder,
  Globe,
  HardDrive,
  HardDriveDownload,
  KeyRound,
  Layers,
  Link2,
  Network,
  Shield,
  SquareTerminal,
  Server,
} from "@lucide/vue";

const props = defineProps<{
  type: string;
}>();

// 只挑常见连接类型给专属图标，其余按分组/兜底图标展示
const icons: Record<string, Component> = {
  // 常用协议
  webdav: Globe,
  sftp: SquareTerminal,
  ftp: Folder,
  smb: Network,
  http: Globe,
  local: HardDrive,
  memory: Database,
  // 对象存储
  s3: Database,
  azureblob: Database,
  azurefiles: Database,
  "google cloud storage": Database,
  b2: Database,
  swift: Database,
  storj: Cloud,
  // 网盘
  drive: Cloud,
  onedrive: Cloud,
  dropbox: Cloud,
  box: Cloud,
  mega: Cloud,
  pcloud: Cloud,
  yandex: Cloud,
  mailru: Cloud,
  protondrive: Shield,
  "google photos": Cloud,
  // 加密与联合
  crypt: KeyRound,
  alias: Link2,
  union: Layers,
  combine: Layers,
  chunker: Layers,
  compress: Archive,
  hasher: Shield,
  cache: HardDriveDownload,
  archive: Archive,
};

const icon = () => icons[props.type] ?? Server;
</script>

<template>
  <component :is="icon()" />
</template>

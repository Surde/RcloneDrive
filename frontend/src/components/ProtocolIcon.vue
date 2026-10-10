<script setup lang="ts">
import type { Component } from "vue";
import { Folder, Globe, HardDrive, SquareTerminal } from "@lucide/vue";

const props = defineProps<{
  type: string;
}>();

const icons: Record<string, Component> = {
  webdav: Globe,
  sftp: SquareTerminal,
  ftp: Folder,
};

const icon = () => icons[props.type] ?? HardDrive;
</script>

<template>
  <component :is="icon()" />
</template>

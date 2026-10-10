<script setup lang="ts">
import type { TabsTriggerProps } from 'reka-ui'
import type { HTMLAttributes } from 'vue'
import { reactiveOmit } from '@vueuse/core'
import { TabsTrigger } from 'reka-ui'
import { cn } from '@/lib/utils'

const props = defineProps<TabsTriggerProps & { class?: HTMLAttributes['class'] }>()

const delegatedProps = reactiveOmit(props, 'class')
</script>

<template>
  <TabsTrigger
    data-slot="tabs-trigger"
    v-bind="delegatedProps"
    :class="cn(
      'text-muted-foreground data-[state=active]:bg-background data-[state=active]:text-foreground focus-visible:ring-ring/50 inline-flex h-6 flex-1 items-center justify-center gap-1 rounded-md px-2.5 text-xs font-medium whitespace-nowrap transition-all outline-none focus-visible:ring-2 disabled:pointer-events-none disabled:opacity-50 data-[state=active]:shadow-xs [&_svg]:size-3.5',
      props.class,
    )"
  >
    <slot />
  </TabsTrigger>
</template>

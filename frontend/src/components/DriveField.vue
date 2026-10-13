<script setup lang="ts">
import type { Field as RcloneField } from "../../bindings/rclone-drive/internal/rclone/models";
import { CircleHelp } from "@lucide/vue";
import { Checkbox } from "@/components/ui/checkbox";
import { Field, FieldDescription, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";

// 按 rclone 的字段元数据渲染一个配置项：bool → 复选框，枚举 → 下拉，密码 → 密码框，其余 → 文本框
const props = defineProps<{
  field: RcloneField;
  scope: string;
  invalid?: boolean;
}>();

const value = defineModel<string>({ required: true });

const inputId = `${props.scope}-${props.field.key}`;
</script>

<template>
  <Field
    :data-invalid="invalid"
    :orientation="field.type === 'bool' ? 'horizontal' : 'vertical'"
    :class="field.type === 'bool' ? 'w-auto' : ''"
  >
    <!-- bool：复选框在前、标签在后，多个 bool 项由外层并排成一行 -->
    <template v-if="field.type === 'bool'">
      <Checkbox
        :id="inputId"
        :model-value="value === 'true'"
        @update:model-value="value = $event === true ? 'true' : 'false'"
      />
      <FieldLabel :for="inputId" class="flex items-center gap-1.5 font-normal">
        {{ field.label }}
        <Tooltip v-if="!invalid && field.help">
          <TooltipTrigger as-child>
            <button type="button" class="cursor-help text-muted-foreground transition-colors hover:text-foreground" @click.stop>
              <CircleHelp class="size-3.5" />
            </button>
          </TooltipTrigger>
          <TooltipContent side="top" class="max-w-56 whitespace-pre-line">{{ field.help }}</TooltipContent>
        </Tooltip>
      </FieldLabel>
    </template>

    <template v-else>
      <FieldLabel :for="inputId" class="flex items-center gap-1.5">
        {{ field.label }}
        <Tooltip v-if="!invalid && field.help">
          <TooltipTrigger as-child>
            <button type="button" class="cursor-help text-muted-foreground transition-colors hover:text-foreground" @click.stop>
              <CircleHelp class="size-3.5" />
            </button>
          </TooltipTrigger>
          <TooltipContent side="top" class="max-w-56 whitespace-pre-line">{{ field.help }}</TooltipContent>
        </Tooltip>
      </FieldLabel>

      <Select
        v-if="field.examples && field.examples.length > 0"
        :model-value="value"
        @update:model-value="value = $event == null ? '' : String($event)"
      >
        <SelectTrigger :id="inputId" :aria-invalid="invalid" class="w-full">
          <SelectValue :placeholder="`请选择${field.label}`" />
        </SelectTrigger>
        <SelectContent>
          <SelectItem v-for="example in field.examples" :key="example.value" :value="example.value">
            {{ example.label }}
          </SelectItem>
        </SelectContent>
      </Select>

      <Input
        v-else
        :id="inputId"
        v-model="value"
        :type="field.password ? 'password' : 'text'"
        :aria-invalid="invalid"
        autocomplete="off"
      />
    </template>

    <FieldDescription v-if="invalid">此字段为必填项。</FieldDescription>
  </Field>
</template>

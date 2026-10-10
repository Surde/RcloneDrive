<script setup lang="ts">
import type { Field as RcloneField } from "../../bindings/rclone-drive/internal/rclone/models";
import { Checkbox } from "@/components/ui/checkbox";
import { Field, FieldDescription, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";

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
  <Field :data-invalid="invalid">
    <FieldLabel :for="inputId">{{ field.label }}</FieldLabel>

    <Checkbox
      v-if="field.type === 'bool'"
      :id="inputId"
      :model-value="value === 'true'"
      @update:model-value="value = $event === true ? 'true' : 'false'"
    />

    <Select
      v-else-if="field.examples && field.examples.length > 0"
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

    <FieldDescription v-if="invalid">此字段为必填项。</FieldDescription>
    <FieldDescription v-else-if="field.help" class="whitespace-pre-line">{{ field.help }}</FieldDescription>
  </Field>
</template>

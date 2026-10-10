export type FieldTypeValue = "string" | "int" | "bool" | "password" | "select";

export interface FieldOption {
  value: string;
  label: string;
}

export interface FieldDef {
  key: string;
  label: string;
  type: FieldTypeValue;
  required: boolean;
  sensitive: boolean;
  advanced: boolean;
  default: string;
  options?: FieldOption[] | null;
  help: string;
}

export interface BackendDef {
  type: string;
  name: string;
  label: string;
  fields: FieldDef[] | null;
}

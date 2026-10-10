// 数值格式化：速率（字节/秒）与容量（字节）转成易读文本
const RATE_UNITS = ["B/s", "KB/s", "MB/s", "GB/s", "TB/s"];
const SIZE_UNITS = ["B", "KB", "MB", "GB", "TB", "PB"];

function scale(value: number, units: string[]): { value: number; unit: string } {
  let scaled = value;
  let index = 0;
  while (scaled >= 1024 && index < units.length - 1) {
    scaled /= 1024;
    index += 1;
  }
  return { value: scaled, unit: units[index] };
}

function digits(value: number, base = 100): number {
  return value >= base ? 0 : value >= base / 10 ? 1 : 2;
}

/** 速率：如 "1.2 MB/s" */
export function formatSpeed(bytesPerSecond: number): string {
  if (!Number.isFinite(bytesPerSecond) || bytesPerSecond <= 0) return "0 B/s";
  const { value, unit } = scale(bytesPerSecond, RATE_UNITS);
  return `${value.toFixed(digits(value))} ${unit}`;
}

/** 容量：如 "18.7 GB"（1024 进制，1 位小数，与系统显示习惯一致） */
export function formatSize(bytes: number): string {
  if (!Number.isFinite(bytes) || bytes <= 0) return "0 B";
  const { value, unit } = scale(bytes, SIZE_UNITS);
  return `${value.toFixed(value >= 100 ? 0 : 1)} ${unit}`;
}

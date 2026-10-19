//go:build !production

package main

// 开发构建（wails3 dev、task build DEV=true）不启用自动更新：
// 避免开发运行的实例被 release 版本覆盖，也省掉无谓的网络检查。
const autoUpdateEnabled = false

package main

// appVersion 是当前版本，自动更新时与 GitHub 最新 release 比较。
// 发布构建由 CI 按 tag 注入（-ldflags "-X main.appVersion=x.y.z"），
// 这里是没有注入时的默认版本。
var appVersion = "0.1.0"

# RcloneDrive

基于 [mygo](https://github.com/egoist/mygo) native 模式的网盘挂载工具：内嵌 rclone 引擎，把远程存储（WebDAV、SFTP、S3、网盘等）挂载为本地磁盘。

## 开发

```bash
task dev      # 热重载开发（含 cmount 挂载支持，需要 gcc + WinFsp 头文件）
task run      # 直接运行
task check    # vet + mygo vet + 测试
```

## 打包

```bash
task build          # 裸 exe → bin/
task package        # NSIS 安装包 + 便携 exe → build/（Windows）
task package:all    # 交叉打包三平台（无挂载支持）
task sync:version APP_VERSION=x.y.z   # 改版本号
```

版本号唯一来源是 `Taskfile.yml` 的 `APP_VERSION`（会同步写进 `mygo.json`）。

## Windows 挂载与 WinFsp

挂载依赖 [WinFsp](https://winfsp.dev/)（内核文件系统代理）。官方 MSI
（`resources/winfsp-2.1.25156.msi`）已内嵌进应用：首次点击「连接」时若
检测到系统未安装，会提取并经 UAC 静默安装，无需用户单独下载安装器。

## 第三方组件

- [rclone](https://rclone.org/)（MIT）— 云存储引擎
- [WinFsp](https://winfsp.dev/)（GPLv3，含链接例外，许可全文见
  `resources/winfsp.LICENSE.txt`）— Windows 文件系统代理
- [mygo](https://github.com/egoist/mygo) — 桌面应用框架

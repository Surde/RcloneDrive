# RcloneDrive

开源的跨平台云存储挂载工具，基于 Go + Wails v3 构建，采用 Vue 3 与 shadcn/vue 打造现代化界面。它将 [rclone](https://github.com/rclone/rclone) 强大的多协议支持封装为简洁的桌面应用，让用户能够像访问本地磁盘一样浏览和管理云端文件，是 RaiDrive 的开源替代方案。

## 特性

- **虚拟驱动器**：将云端存储挂载为本地磁盘（Windows 盘符，如 `Z:`；macOS / Linux 为挂载目录）
- **多协议支持**：首批支持 WebDAV、SFTP、FTP，架构上可平滑扩展到 rclone 的全部 70+ 后端
- **rclone 内嵌为 Go 依赖库**：直接以 `github.com/rclone/rclone` 作为库调用，无需安装或管理外部 rclone 可执行文件
- **连接管理**：添加 / 编辑 / 删除虚拟驱动器，支持连接测试、只读挂载、开机自动连接
- **主题与启动**：默认「跟随系统」，可切换浅色 / 深色（记忆上次选择）；支持开机自启（Windows 注册表 Run 项、macOS LaunchAgents、Linux autostart .desktop），自启时以 `--tray` 静默进托盘
- **系统托盘**：托盘图标 + 菜单 —— 显示主界面 / 检查更新 / 添加驱动器 / 设置 / 各驱动器的「连接」「断开连接」「编辑」/ 退出
- **系统原生通知**：挂载断开、托盘操作结果、检查更新等后台事件使用系统通知（Windows 通知中心 / macOS 通知中心 / Linux libnotify），窗口隐藏时同样可见
- **窗口行为**：无系统窗口边框、固定尺寸，标题栏由界面自绘（仅保留关闭按钮）；关闭主窗口只隐藏到托盘，挂载与传输继续在后台运行
- **多窗口界面**：主窗口（驱动器列表）+ 独立的「设置」「新虚拟驱动器 / 编辑驱动器」窗口（已打开则复用并聚焦）；Windows 上所有窗口与安装包创建的快捷方式共用同一个 AppUserModelID，任务栏只占一个按钮
- **单实例**：同一次登录只允许运行一个实例（开发构建除外）——重复启动会把已有窗口带到前台，不会出现第二个托盘图标或两组挂载状态；开机自启触发的重复启动静默退出

## 运行前提

挂载由内嵌的 rclone `cmount` 实现（基于 WinFsp / FUSE）完成，需要相应的系统级文件系统驱动：

- **Windows**：[WinFsp](https://winfsp.dev/)（`rclone mount` 的 Windows 依赖）
- **macOS**：[macFUSE](https://osxfuse.github.io/)
- **Linux**：`fuse3`

不需要单独安装 rclone。

## 技术栈

| 层 | 技术 |
| --- | --- |
| 桌面框架 | Wails v3 (Go 1.27) |
| 前端 | Vue 3 + Vite + TypeScript |
| UI 组件 | shadcn/vue (nova 预设) + Tailwind CSS v4 |
| 存储引擎 | rclone v1.75.x（Go 依赖库内嵌，`cmount` 挂载） |

## 开发

前置依赖：[Go](https://go.dev/)、[Node.js + pnpm](https://pnpm.io/)、[Wails v3 CLI](https://v3.wails.io/)、[Task](https://taskfile.dev/)。

由于内嵌挂载实现使用 cgo，Windows 上还需要 **MinGW-w64 gcc**（cgo 需要的 C 工具链）：

```powershell
winget install -e --id BrechtSanders.WinLibs.POSIX.UCRT
```

无需手动把它加入 `PATH`：构建时按「`PATH` 中的 gcc → winget 安装的 WinLibs」顺序自动解析（见 `build/windows/find-cc.ps1`），也可用 `MINGW_GCC` 环境变量指定。

WinFsp 的 SDK 头文件已随仓库提供（`third_party/winfsp/inc`，取自 [winfsp](https://github.com/winfsp/winfsp) 的 `ebd50e1`，即 `v2.2B4` 之后 6 个提交，许可证见同目录 `License.txt`），**无需另外安装 SDK**：`task build` / `task dev` 会自动把 `CPATH` 指向 `third_party/winfsp/inc/fuse`。

需要覆盖默认配置时使用环境变量：

| 变量 | 说明 |
| --- | --- |
| `MINGW_GCC` | 指定要使用的 gcc（覆盖自动解析），如 `MINGW_GCC="C:/path/to/mingw64/bin/gcc.exe"` |
| `WINFSP_INC` | 改用外部 WinFsp SDK 的 `inc/fuse` 目录，如 `WINFSP_INC="D:/sdk/inc/fuse"` |
| `CGO_ENABLED=0` | 关闭 cgo，构建出的应用可管理驱动器配置但无法挂载 |

常用命令：

```bash
# 安装前端依赖并启动开发模式（前端热更新 + 绑定自动生成）
task install:frontend:deps
task dev

# 仅生成前端绑定
task generate:bindings

# 构建前端
cd frontend && pnpm build

# 构建应用（当前平台，Windows 默认 -tags production,cmount）
task build

# 打包安装包
task package
```

> 无 cgo 环境下可用 `task build CGO_ENABLED=0` 构建，此时应用可正常管理驱动器配置，但挂载功能不可用（界面会给出提示）。

Windows 安装包（NSIS）创建开始菜单 / 桌面快捷方式后会调用 `build/windows/nsis/set-shortcut-aumid.ps1` 给它们写入 AppUserModelID，固定到任务栏的图标才能与运行中的窗口合并成一个按钮。该脚本与 `project.nsi` 都必须保持纯 ASCII —— PowerShell 5.1 与 makensis 都以 ANSI 读取无 BOM 的脚本文件。

### 图标

应用图标与界面 logo 由仓库内的生成器产出（仅标准库，改完 `build/tools/icon/main.go` 里的几何或配色后重跑即可）：

```bash
go run ./build/tools/icon build/appicon.png           # 应用图标 1024
go run ./build/tools/icon frontend/public/logo.png 256 # 界面 logo / favicon
go run ./build/tools/icon build/windows/tray.png 32    # 托盘图标
```

改完 `build/appicon.png` 后运行 `task build`，会自动重新生成 `windows/icon.ico`、`darwin/icons.icns` 并嵌入可执行文件。

## 配置存储

- rclone 配置文件：`<用户配置目录>/RcloneDrive/rclone.conf`（标准 rclone INI 格式）
- 密码类字段由 rclone 自身的混淆算法加密后写入
- 应用元数据（挂载点、只读等偏好）：`<用户配置目录>/RcloneDrive/remotes.json`

## 目录结构

```
├── main.go                 # 应用入口、窗口与服务注册
├── remoteservice.go        # 虚拟驱动器 CRUD、连接测试
├── mountservice.go         # 挂载 / 卸载 / 状态管理
├── systemservice.go        # 引擎状态、平台信息
├── internal/
│   ├── backend/            # 后端协议 schema（WebDAV / SFTP / FTP 字段定义）
│   └── rclone/             # 内嵌 rclone：后端注册、配置读写、挂载生命周期
├── frontend/
│   ├── src/                # Vue 3 源码
│   └── bindings/           # Wails 自动生成的 TS 绑定
├── third_party/
│   └── winfsp/             # WinFsp SDK 头文件（构建期使用，见 License.txt）
└── build/                  # 各平台构建配置
```

## 协议支持 TODO

> 所有协议通过内嵌的 rclone 后端实现，添加新协议 = 注册后端 + 定义字段 schema + 图标，欢迎 PR。

- [x] WebDAV
- [x] SFTP
- [x] FTP
- [ ] Amazon S3
- [ ] Google Drive
- [ ] OneDrive
- [ ] Dropbox
- [ ] Box
- [ ] Nextcloud / ownCloud
- [ ] 阿里云 OSS / 腾讯云 COS / 七牛 Kodo

## Roadmap

- [ ] 传输速度与缓存监控
- [ ] 应用内自动下载并安装更新（目前「检查更新」只提示并跳转 Releases 页面）
- [ ] 其余 70+ rclone 后端（见上方 TODO）

## 许可证

[LICENSE](LICENSE)

; RcloneDrive 安装器（管理员级）。
;
; mygo 生成的 NSIS 脚本没有定制注入点，且是按用户级安装（%LOCALAPPDATA%，
; 不提权），无法在 setup 阶段安装 WinFsp。这里按 mygo 安装器的结构重写为
; 管理员级：setup 启动时一次 UAC，随后应用文件 + WinFsp（如未安装）一并
; 静默装完。输出文件名与 mygo 约定一致（更新器按此命名匹配资产）。
;
; 编译：makensis /DVERSION=x.y.z installer.nsi（由 Taskfile 的 package 任务
; 在 go tool mygo build 之后调用，输出覆盖 mygo 自带的安装包）。
; 路径约定：相对路径以本脚本所在目录（build/windows）为基准。

Unicode true
ManifestDPIAware true
SetCompressor /SOLID lzma

!ifndef VERSION
  !error "用法: makensis /DVERSION=x.y.z installer.nsi"
!endif
!define ROOT "..\.."

!define STAGE "${ROOT}\build\windows-amd64"
!define WINFSP_MSI "${ROOT}\third_party\winfsp\winfsp-2.1.25156.msi"

Name "RcloneDrive"
OutFile "${STAGE}\RcloneDrive Setup ${VERSION} amd64.exe"
InstallDir "$PROGRAMFILES64\RcloneDrive"
RequestExecutionLevel admin
BrandingText " "

!define MUI_ICON "${ROOT}\build\windows\appicon.ico"
!define MUI_UNICON "${ROOT}\build\windows\appicon.ico"
!define MUI_FINISHPAGE_RUN "$INSTDIR\RcloneDrive.exe"
!define MUI_FINISHPAGE_SHOWREADME
!define MUI_FINISHPAGE_SHOWREADME_TEXT "创建桌面快捷方式"
!define MUI_FINISHPAGE_SHOWREADME_FUNCTION CreateDesktopShortcut
!include "MUI2.nsh"
!insertmacro MUI_PAGE_DIRECTORY
!insertmacro MUI_PAGE_INSTFILES
!insertmacro MUI_PAGE_FINISH
!insertmacro MUI_UNPAGE_CONFIRM
!insertmacro MUI_UNPAGE_INSTFILES
!insertmacro MUI_LANGUAGE "SimpChinese"
!insertmacro MUI_LANGUAGE "English"

Function .onInit
  SetRegView 64
FunctionEnd

Section "RcloneDrive" SEC_APP
  SetOutPath "$INSTDIR"
  ; 暂存目录里可能留有本安装器自身的旧产物（RcloneDrive Setup*.exe），排除
  File /r /x "RcloneDrive Setup*.exe" "${STAGE}\*.*"
  WriteUninstaller "$INSTDIR\Uninstall.exe"
  CreateShortCut "$SMPROGRAMS\RcloneDrive.lnk" "$INSTDIR\RcloneDrive.exe"
  WriteRegStr HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\com.younland.rclonedrive" "DisplayName" "RcloneDrive"
  WriteRegStr HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\com.younland.rclonedrive" "DisplayVersion" "${VERSION}"
  WriteRegStr HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\com.younland.rclonedrive" "DisplayIcon" "$INSTDIR\RcloneDrive.exe"
  WriteRegStr HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\com.younland.rclonedrive" "InstallLocation" "$INSTDIR"
  WriteRegStr HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\com.younland.rclonedrive" "UninstallString" '"$INSTDIR\Uninstall.exe"'
  WriteRegStr HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\com.younland.rclonedrive" "QuietUninstallString" '"$INSTDIR\Uninstall.exe" /S'
  WriteRegDWORD HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\com.younland.rclonedrive" "NoModify" 1
  WriteRegDWORD HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\com.younland.rclonedrive" "NoRepair" 1
SectionEnd

Section "WinFsp（未安装时才执行）" SEC_WINFSP
  ; 已安装（驱动服务已注册）则跳过
  ReadRegStr $0 HKLM "SYSTEM\CurrentControlSet\Services\WinFsp" "ImagePath"
  StrCmp $0 "" do_install
    DetailPrint "WinFsp 已安装，跳过"
    Goto done
  do_install:
    SetOutPath $PLUGINSDIR
    File /oname=winfsp.msi "${WINFSP_MSI}"
    DetailPrint "正在静默安装 WinFsp…"
    ExecWait 'msiexec /i "$PLUGINSDIR\winfsp.msi" /qn /norestart' $1
    IntCmp $1 0 done
    IntCmp $1 3010 need_reboot
      MessageBox MB_OK|MB_ICONEXCLAMATION "WinFsp 安装失败（msiexec 退出码 $1）。挂载云盘需要 WinFsp，可稍后重试或在首次挂载时由应用自动安装。" /SD IDOK
      Goto done
    need_reboot:
      SetRebootFlag true
  done:
SectionEnd

Function CreateDesktopShortcut
  CreateShortCut "$DESKTOP\RcloneDrive.lnk" "$INSTDIR\RcloneDrive.exe"
FunctionEnd

; Explorer 会在快捷方式出现后几秒才打开它，删除需重试。
Function un.DeleteShortcut
  Exch $0
  Push $1
  StrCpy $1 50
  retry:
    ClearErrors
    Delete $0
    IfErrors 0 done
    IntOp $1 $1 - 1
    IntCmp $1 0 done
    Sleep 100
    Goto retry
  done:
  Pop $1
  Pop $0
FunctionEnd

Section "Uninstall"
  Push "$SMPROGRAMS\RcloneDrive.lnk"
  Call un.DeleteShortcut
  Push "$DESKTOP\RcloneDrive.lnk"
  Call un.DeleteShortcut
  DeleteRegKey HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\com.younland.rclonedrive"
  ; WinFsp 保留不卸：系统组件，其他应用可能也在使用。
  RMDir /r "$INSTDIR"
SectionEnd

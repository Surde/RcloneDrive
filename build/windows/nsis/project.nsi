Unicode true

####
## Please note: Template replacements don't work in this file. They are provided with default defines like
## mentioned underneath.
## If the keyword is not defined, "wails_tools.nsh" will populate them.
## If they are defined here, "wails_tools.nsh" will not touch them. This allows you to use this project.nsi manually
## from outside of Wails for debugging and development of the installer.
## 
## For development first make a wails nsis build to populate the "wails_tools.nsh":
## > wails build --target windows/amd64 --nsis
## Then you can call makensis on this file with specifying the path to your binary:
## For a AMD64 only installer:
## > makensis -DARG_WAILS_AMD64_BINARY=..\..\bin\app.exe
## For a ARM64 only installer:
## > makensis -DARG_WAILS_ARM64_BINARY=..\..\bin\app.exe
## For a installer with both architectures:
## > makensis -DARG_WAILS_AMD64_BINARY=..\..\bin\app-amd64.exe -DARG_WAILS_ARM64_BINARY=..\..\bin\app-arm64.exe
####
## The following information is taken from the wails_tools.nsh file, but they can be overwritten here.
####
## !define INFO_PROJECTNAME    "my-project" # Default "RcloneDrive"
## !define INFO_COMPANYNAME    "My Company" # Default "younland"
## !define INFO_PRODUCTNAME    "My Product Name" # Default "My Product"
## !define INFO_PRODUCTVERSION "1.0.0"     # Default "0.1.0"
## !define INFO_COPYRIGHT      "(c) Now, My Company" # Default "© 2026, younland"
###
## !define PRODUCT_EXECUTABLE  "Application.exe"      # Default "${INFO_PROJECTNAME}.exe"
## !define UNINST_KEY_NAME     "UninstKeyInRegistry"  # Default "${INFO_COMPANYNAME}${INFO_PRODUCTNAME}"
####
## !define REQUEST_EXECUTION_LEVEL "admin"            # Default "admin"  see also https://nsis.sourceforge.io/Docs/Chapter4.html
## !define WAILS_INSTALL_SCOPE     "user"             # Default "machine" - set to "user" for per-user install ($LOCALAPPDATA) without UAC prompt
####
## Include the wails tools
####
!include "wails_tools.nsh"
!include "LogicLib.nsh"

## Must match the AUMID declared at runtime in appid_windows.go (appinfo.go appID).
## The shortcuts carry it too, so an icon pinned to the taskbar from a shortcut
## merges with the running window instead of showing up as a second button.
## NOTE: keep this file ASCII-only, makensis reads BOM-less scripts as ANSI.
!define APP_USER_MODEL_ID "com.younland.rclonedrive"

# The version information for this two must consist of 4 parts
VIProductVersion "${INFO_PRODUCTVERSION}.0"
VIFileVersion    "${INFO_PRODUCTVERSION}.0"

VIAddVersionKey "CompanyName"     "${INFO_COMPANYNAME}"
VIAddVersionKey "FileDescription" "${INFO_PRODUCTNAME} Installer"
VIAddVersionKey "ProductVersion"  "${INFO_PRODUCTVERSION}"
VIAddVersionKey "FileVersion"     "${INFO_PRODUCTVERSION}"
VIAddVersionKey "LegalCopyright"  "${INFO_COPYRIGHT}"
VIAddVersionKey "ProductName"     "${INFO_PRODUCTNAME}"

# Enable HiDPI support. https://nsis.sourceforge.io/Reference/ManifestDPIAware
ManifestDPIAware true

!include "MUI.nsh"

!define MUI_ICON "..\icon.ico"
!define MUI_UNICON "..\icon.ico"
# !define MUI_WELCOMEFINISHPAGE_BITMAP "resources\leftimage.bmp" #Include this to add a bitmap on the left side of the Welcome Page. Must be a size of 164x314
!define MUI_FINISHPAGE_NOAUTOCLOSE # Wait on the INSTFILES page so the user can take a look into the details of the installation steps
!define MUI_ABORTWARNING # This will warn the user if they exit from the installer.

!insertmacro MUI_PAGE_WELCOME # Welcome to the installer page.
# !insertmacro MUI_PAGE_LICENSE "resources\eula.txt" # Adds a EULA page to the installer
!insertmacro MUI_PAGE_DIRECTORY # In which folder install page.
!insertmacro MUI_PAGE_INSTFILES # Installing page.
!insertmacro MUI_PAGE_FINISH # Finished installation page.

!insertmacro MUI_UNPAGE_INSTFILES # Uninstalling page

!insertmacro MUI_LANGUAGE "English" # Set the Language of the installer

## The following two statements can be used to sign the installer and the uninstaller. The path to the binaries are provided in %1
#!uninstfinalize 'signtool --file "%1"'
#!finalize 'signtool --file "%1"'

Name "${INFO_PRODUCTNAME}"
OutFile "..\..\..\bin\${INFO_PROJECTNAME}-${ARCH}-installer.exe" # Name of the installer's file.
!if "${WAILS_INSTALL_SCOPE}" == "user"
    InstallDir "$LOCALAPPDATA\Programs\${INFO_PRODUCTNAME}"
!else
    InstallDir "$PROGRAMFILES64\${INFO_COMPANYNAME}\${INFO_PRODUCTNAME}"
!endif
ShowInstDetails show # This will always show the installation details.

Function .onInit
   !insertmacro wails.checkArchitecture
FunctionEnd

Section
    !insertmacro wails.setShellContext

    !insertmacro wails.webview2runtime

    # WinFsp runtime (required for mounting): at runtime cgofuse falls back to the
    # HKLM\SOFTWARE\WOW6432Node\WinFsp InstallDir registry value when it cannot load
    # winfsp-x64.dll directly, so the presence of that value is our "installed" test.
    # Only the x64 MSI is bundled here; on non-x64 Windows users install WinFsp themselves.
    ${If} ${IsNativeAMD64}
        SetRegView 32
        ReadRegStr $0 HKLM "SOFTWARE\WinFsp" "InstallDir"
        ${If} $0 == ""
            SetDetailsPrint both
            DetailPrint "Installing: WinFsp (required for drive mounting)"
            SetDetailsPrint listonly
            InitPluginsDir
            SetOutPath "$PLUGINSDIR\winfsp"
            File "winfsp-2.1.25156.msi"
            # The WinFsp MSI is a per-machine install: run it directly when already
            # elevated; in a per-user install elevate via runas (UAC prompt) first.
            UserInfo::GetAccountType
            Pop $1
            ${If} $1 == "Admin"
                ExecWait 'msiexec /i "$PLUGINSDIR\winfsp\winfsp-2.1.25156.msi" /qn /norestart'
            ${Else}
                ExecShellWait "runas" "msiexec.exe" '/i "$PLUGINSDIR\winfsp\winfsp-2.1.25156.msi" /qn /norestart'
            ${EndIf}
            # Verify the install; warn on failure (no popup in silent mode)
            ReadRegStr $0 HKLM "SOFTWARE\WinFsp" "InstallDir"
            ${If} $0 == ""
                IfSilent silentWinFsp notSilentWinFsp
                notSilentWinFsp:
                    MessageBox MB_ICONEXCLAMATION|MB_OK "WinFsp could not be installed, mounting will not work.$\nPlease install it manually from https://winfsp.dev/rel/."
                silentWinFsp:
            ${EndIf}
        ${EndIf}
        SetRegView 64
    ${Else}
        DetailPrint "Note: WinFsp runtime is not bundled for non-x64 Windows; install it from https://winfsp.dev/rel/ to enable mounting"
    ${EndIf}

    SetOutPath $INSTDIR
    
    !insertmacro wails.files

    CreateShortcut "$SMPROGRAMS\${INFO_PRODUCTNAME}.lnk" "$INSTDIR\${PRODUCT_EXECUTABLE}"
    CreateShortCut "$DESKTOP\${INFO_PRODUCTNAME}.lnk" "$INSTDIR\${PRODUCT_EXECUTABLE}"

    ## Give both shortcuts the AppUserModelID used at runtime
    ## (see set-shortcut-aumid.ps1). This only affects taskbar button merging,
    ## so failures are reported but never abort the installation.
    InitPluginsDir
    SetOutPath "$PLUGINSDIR"
    File "/oname=set-shortcut-aumid.ps1" "set-shortcut-aumid.ps1"
    SetOutPath "$INSTDIR"

    nsExec::ExecToLog 'powershell.exe -NoProfile -ExecutionPolicy Bypass -File "$PLUGINSDIR\set-shortcut-aumid.ps1" -Shortcut "$SMPROGRAMS\${INFO_PRODUCTNAME}.lnk" -AppUserModelID "${APP_USER_MODEL_ID}"'
    Pop $0
    ${If} $0 != "0"
        DetailPrint "Warning: could not set the AppUserModelID of the Start Menu shortcut ($0)"
    ${EndIf}

    nsExec::ExecToLog 'powershell.exe -NoProfile -ExecutionPolicy Bypass -File "$PLUGINSDIR\set-shortcut-aumid.ps1" -Shortcut "$DESKTOP\${INFO_PRODUCTNAME}.lnk" -AppUserModelID "${APP_USER_MODEL_ID}"'
    Pop $0
    ${If} $0 != "0"
        DetailPrint "Warning: could not set the AppUserModelID of the Desktop shortcut ($0)"
    ${EndIf}

    !insertmacro wails.associateFiles
    !insertmacro wails.associateCustomProtocols
    
    !insertmacro wails.writeUninstaller
SectionEnd

Section "uninstall" 
    !insertmacro wails.setShellContext

    RMDir /r "$AppData\${PRODUCT_EXECUTABLE}" # Remove the WebView2 DataPath

    RMDir /r $INSTDIR

    Delete "$SMPROGRAMS\${INFO_PRODUCTNAME}.lnk"
    Delete "$DESKTOP\${INFO_PRODUCTNAME}.lnk"

    ## Notification identity registered by the application at runtime (HKCU)
    DeleteRegKey HKCU "Software\Classes\AppUserModelId\${APP_USER_MODEL_ID}"

    !insertmacro wails.unassociateFiles
    !insertmacro wails.unassociateCustomProtocols

    !insertmacro wails.deleteUninstaller
SectionEnd

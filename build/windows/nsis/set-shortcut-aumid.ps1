# Sets the AppUserModelID (PKEY_AppUserModel_ID) of a shortcut.
#
# The installer calls this after creating the Start Menu / Desktop shortcuts.
# The application declares the same AUMID at runtime with
# SetCurrentProcessExplicitAppUserModelID (see appid_windows.go). The shortcut
# must carry it as well, otherwise an icon pinned to the taskbar from the
# shortcut does not merge with the running window and shows up as a second
# taskbar button.
#
# Usage:
#   powershell -NoProfile -ExecutionPolicy Bypass -File set-shortcut-aumid.ps1 `
#     -Shortcut "C:\...\RcloneDrive.lnk" -AppUserModelID "com.younland.rclonedrive"
#
# Exit code: 0 on success, non-zero on failure (the installer only logs it and
# does not abort the installation).
#
# NOTE: keep this file ASCII-only. Windows PowerShell 5.1 reads BOM-less
# scripts using the system ANSI code page, so non-ASCII text would be mangled.

param(
    [Parameter(Mandatory = $true)][string]$Shortcut,
    [Parameter(Mandatory = $true)][string]$AppUserModelID
)

$ErrorActionPreference = 'Stop'

if (-not (Test-Path -LiteralPath $Shortcut -PathType Leaf)) {
    Write-Output "shortcut not found: $Shortcut"
    exit 1
}

# PROPVARIANT below is declared with the 64-bit layout and the installer is
# 64-bit only.
if ([IntPtr]::Size -ne 8) {
    Write-Output "64-bit PowerShell required"
    exit 2
}

$source = @'
using System;
using System.Runtime.InteropServices;
using System.Runtime.InteropServices.ComTypes;
using System.Text;

namespace RcloneDrive
{
    [ComImport, Guid("00021401-0000-0000-C000-000000000046")]
    internal class ShellLinkObject { }

    // Member order is the vtable order, do not change it.
    [ComImport, InterfaceType(ComInterfaceType.InterfaceIsIUnknown), Guid("000214F9-0000-0000-C000-000000000046")]
    internal interface IShellLinkW
    {
        void GetPath([Out, MarshalAs(UnmanagedType.LPWStr)] StringBuilder pszFile, int cch, IntPtr pfd, int fFlags);
        void GetIDList(out IntPtr ppidl);
        void SetIDList(IntPtr pidl);
        void GetDescription([Out, MarshalAs(UnmanagedType.LPWStr)] StringBuilder pszName, int cch);
        void SetDescription([MarshalAs(UnmanagedType.LPWStr)] string pszName);
        void GetWorkingDirectory([Out, MarshalAs(UnmanagedType.LPWStr)] StringBuilder pszDir, int cch);
        void SetWorkingDirectory([MarshalAs(UnmanagedType.LPWStr)] string pszDir);
        void GetArguments([Out, MarshalAs(UnmanagedType.LPWStr)] StringBuilder pszArgs, int cch);
        void SetArguments([MarshalAs(UnmanagedType.LPWStr)] string pszArgs);
        void GetHotkey(out short pwHotkey);
        void SetHotkey(short wHotkey);
        void GetShowCmd(out int piShowCmd);
        void SetShowCmd(int iShowCmd);
        void GetIconLocation([Out, MarshalAs(UnmanagedType.LPWStr)] StringBuilder pszIconPath, int cch, out int piIcon);
        void SetIconLocation([MarshalAs(UnmanagedType.LPWStr)] string pszIconPath, int iIcon);
        void SetRelativePath([MarshalAs(UnmanagedType.LPWStr)] string pszPathRel, int dwReserved);
        void Resolve(IntPtr hwnd, int fFlags);
        void SetPath([MarshalAs(UnmanagedType.LPWStr)] string pszFile);
    }

    // Member order is the vtable order, do not change it.
    [ComImport, InterfaceType(ComInterfaceType.InterfaceIsIUnknown), Guid("886D8EEB-8CF2-4446-8D02-CDBA1DBDCF99")]
    internal interface IPropertyStore
    {
        void GetCount(out uint cProps);
        void GetAt(uint iProp, out PROPERTYKEY pkey);
        void GetValue(ref PROPERTYKEY key, out PROPVARIANT pv);
        void SetValue(ref PROPERTYKEY key, ref PROPVARIANT pv);
        void Commit();
    }

    [StructLayout(LayoutKind.Sequential, Pack = 4)]
    internal struct PROPERTYKEY
    {
        public Guid fmtid;
        public uint pid;
    }

    // Only VT_LPWSTR is used here. The native 64-bit PROPVARIANT is 16 bytes:
    // an 8 byte header (vt plus reserved bytes) followed by an 8 byte pointer.
    [StructLayout(LayoutKind.Explicit)]
    internal struct PROPVARIANT
    {
        [FieldOffset(0)] public ushort vt;
        [FieldOffset(8)] public IntPtr pointerValue;
    }

    public static class ShortcutAppUserModelId
    {
        private const ushort VtLpwstr = 31;
        private const int StgmReadWrite = 2;

        public static void Set(string shortcutPath, string appUserModelId)
        {
            // PKEY_AppUserModel_ID = {9F4C2855-9F79-4B39-A8D0-E1D42DE1D5F3}, 5
            PROPERTYKEY key = new PROPERTYKEY();
            key.fmtid = new Guid("9F4C2855-9F79-4B39-A8D0-E1D42DE1D5F3");
            key.pid = 5;

            Type shellLinkType = Type.GetTypeFromCLSID(new Guid("00021401-0000-0000-C000-000000000046"), true);
            object shellLink = Activator.CreateInstance(shellLinkType);

            IPersistFile persistFile = (IPersistFile)shellLink;
            persistFile.Load(shortcutPath, StgmReadWrite);

            IPropertyStore store = (IPropertyStore)shellLink;
            PROPVARIANT value = new PROPVARIANT();
            value.vt = VtLpwstr;
            value.pointerValue = Marshal.StringToCoTaskMemUni(appUserModelId);
            try
            {
                store.SetValue(ref key, ref value);
                store.Commit();
                persistFile.Save(shortcutPath, true);
            }
            finally
            {
                Marshal.FreeCoTaskMem(value.pointerValue);
            }
        }
    }
}
'@

Add-Type -TypeDefinition $source -Language CSharp
[RcloneDrive.ShortcutAppUserModelId]::Set($Shortcut, $AppUserModelID)
Write-Output "AppUserModelID=$AppUserModelID -> $Shortcut"

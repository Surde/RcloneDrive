# Resolves the gcc used for cgo: gcc on PATH first, then WinLibs installed by winget.
# Prints a value usable as CC (falls back to "gcc" so the precondition reports a clear error).
$ErrorActionPreference = "SilentlyContinue"

if (Get-Command gcc -CommandType Application -ErrorAction SilentlyContinue | Select-Object -First 1) {
    Write-Output "gcc"
    exit 0
}

$packages = Join-Path $env:LOCALAPPDATA "Microsoft\WinGet\Packages"
$found = Get-ChildItem -LiteralPath $packages -Directory |
    Where-Object { $_.Name -like "BrechtSanders.WinLibs.*" } |
    ForEach-Object { Join-Path $_.FullName "mingw64\bin\gcc.exe" } |
    Where-Object { Test-Path -LiteralPath $_ } |
    Select-Object -First 1

if ($found) {
    Write-Output $found
    exit 0
}

Write-Output "gcc"

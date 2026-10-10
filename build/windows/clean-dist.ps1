# Removes the previous frontend build output.
# dev and production builds share frontend/dist; without cleaning, Task's up-to-date
# check reuses assets built in the other mode (and Go embed may fail on stale names).
# If removal fails (files locked by a running dev server), the source file's mtime is
# bumped so Task reruns the frontend build instead.
# The script always exits 0 so a failed clean never breaks the whole build.
param(
    [Parameter(Mandatory = $true)][string]$Dist,
    [Parameter(Mandatory = $true)][string]$TouchFile
)
$ErrorActionPreference = "SilentlyContinue"

Remove-Item -Recurse -Force -LiteralPath $Dist
if (Test-Path -LiteralPath $Dist) {
    (Get-Item -LiteralPath $TouchFile).LastWriteTime = Get-Date
}
exit 0

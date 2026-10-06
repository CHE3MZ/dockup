$ErrorActionPreference = 'Stop'
# Smoke: no WSL mutation, only version/doctor/ps.
$exe = Join-Path $PSScriptRoot '..\dist\dockup-windows-amd64.exe'
if (!(Test-Path $exe)) { Write-Error "missing $exe — run scripts/build.ps1 first"; exit 1 }
& $exe version
& $exe doctor
& $exe ps
& $exe daemon status

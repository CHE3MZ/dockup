#Requires -Version 7.0
# Manual smoke test (needs WSL + optional docker CLI).
$ErrorActionPreference = 'Continue'
.\dockup.exe version
.\dockup.exe doctor
.\dockup.exe list

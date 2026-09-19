$ErrorActionPreference = 'Stop'
# Smoke: no WSL mutation, only version/doctor/ps.
.\dockup.exe version
.\dockup.exe doctor
.\dockup.exe ps
.\dockup.exe daemon status

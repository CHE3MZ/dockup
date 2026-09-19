$ErrorActionPreference = 'Stop'
$env:GOOS = 'windows'
$env:GOARCH = 'amd64'
if ($env:OS -ne 'Windows_NT' -and -not $IsWindows) { Write-Error 'build.ps1 is Windows-only'; exit 1 }
go vet ./...
go build -o dockup.exe ./cmd/dockup
Write-Host 'built dockup.exe'

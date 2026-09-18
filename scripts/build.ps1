#Requires -Version 7.0
$ErrorActionPreference = 'Stop'
if ($env:OS -notlike '*Windows*') { throw 'build.ps1 is Windows-only' }
$env:GOOS = 'windows'
$env:GOARCH = 'amd64'
go vet ./...
go build -o dockup.exe ./cmd/dockup
Write-Host 'built dockup.exe'

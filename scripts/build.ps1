#Requires -Version 7.0
$ErrorActionPreference = 'Stop'
if ($env:OS -notlike '*Windows*') { throw 'build.ps1 is Windows-only' }
$env:GOOS = 'windows'
$env:GOARCH = 'amd64'
go vet ./...
$sha = git rev-parse --short HEAD
go build -ldflags "-X main.version=v0.1.0-dev -X main.commit=$sha" -o dockup.exe ./cmd/dockup
Write-Host 'built dockup.exe'

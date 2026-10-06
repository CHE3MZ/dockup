$ErrorActionPreference = 'Stop'
if ($env:OS -ne 'Windows_NT' -and -not $IsWindows) { Write-Error 'build.ps1 is Windows-only'; exit 1 }

$root = Split-Path -Parent $PSScriptRoot
Push-Location $root
try {
  $version = 'dev'
  try {
    $v = (git rev-parse --short HEAD 2>$null).Trim()
    if ($v) { $version = $v }
  } catch { }

  $env:GOOS = 'windows'
  go vet ./...

  New-Item -ItemType Directory -Force dist | Out-Null
  foreach ($arch in @('amd64', 'arm64')) {
    $out = "dist/dockup-windows-$arch.exe"
    if (Test-Path $out) { Remove-Item $out -Force }
    $env:GOARCH = $arch
    go build -ldflags "-X github.com/CHE3MZ/dockup/internal/config.Version=$version" -o $out ./cmd/dockup
    $mb = [math]::Round((Get-Item $out).Length / 1MB, 1)
    Write-Host "built $out ($version, $mb MB)"
  }
} finally {
  Pop-Location
}

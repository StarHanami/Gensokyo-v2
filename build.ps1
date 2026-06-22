# Gensokyo local build script
# Uses goproxy.cn for fast dependency downloads in China

$ErrorActionPreference = 'Stop'

# Set Go module proxy (fast China mirror)
$env:GOPROXY = 'https://goproxy.cn,direct'
$env:GOFLAGS = '-mod=mod'

Write-Host "=== Gensokyo Build ===" -ForegroundColor Cyan
Write-Host "Go Proxy: $env:GOPROXY" -ForegroundColor Gray

# Parameters
$targetOS = if ($args[0]) { $args[0] } else { (go env GOOS) }
$targetArch = if ($args[1]) { $args[1] } else { (go env GOARCH) }
$upxLevel = if ($args[2]) { $args[2] } else { "7" }

$ext = ""
if ($targetOS -eq "windows") { $ext = ".exe" }

$env:GOOS = $targetOS
$env:GOARCH = $targetArch
$env:CGO_ENABLED = "0"

$output = "gensokyo-$targetOS-$targetArch$ext"

Write-Host "Target: $targetOS/$targetArch"
Write-Host "Output: $output"

# 下载依赖
Write-Host "`n[1/3] Downloading deps..." -ForegroundColor Yellow
go mod tidy

# 编译
Write-Host "[2/3] Building..." -ForegroundColor Yellow
go build -trimpath -ldflags="-s -w" -v -o $output .

if ($LASTEXITCODE -ne 0) {
    Write-Host 'Build failed!' -ForegroundColor Red
    exit 1
}

Write-Host ('Build success: ' + $output) -ForegroundColor Green

# UPX compress (fixed level 7)
Write-Host '[3/3] UPX compress...' -ForegroundColor Yellow
$upx = Get-Command "upx" -ErrorAction SilentlyContinue
if ($upx) {
    & $upx.Source "-7" $output
    Write-Host 'UPX done' -ForegroundColor Green
} else {
    Write-Host 'UPX not found, skip compression.' -ForegroundColor Gray
    Write-Host 'Install UPX: winget install upx'
}

Write-Host '=== Build complete ===' -ForegroundColor Cyan

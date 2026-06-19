param(
    [string]$OS = "",
    [string]$Arch = "",
    [string]$Output = "gensokyo",
    [int]$UpxLevel = 7
)

if ($OS -eq "") { $OS = (go env GOOS) }
if ($Arch -eq "") { $Arch = (go env GOARCH) }

$ext = ""
if ($OS -eq "windows") { $ext = ".exe" }

Write-Host "Building gensokyo for $OS/$Arch ..."
$env:GOOS = $OS
$env:GOARCH = $Arch
$env:CGO_ENABLED = "0"

$binary = "$Output-$OS-$Arch$ext"
go build -ldflags="-s -w" -o $binary .

if ($LASTEXITCODE -eq 0) {
    Write-Host "Build success: $binary"
    # 检测 upx 是否可用
    $upxPath = (Get-Command "upx" -ErrorAction SilentlyContinue).Source
    if ($upxPath) {
        Write-Host "Compressing with UPX -$UpxLevel ..."
        & $upxPath "-$UpxLevel" $binary
    } else {
        Write-Host "UPX not found, skip compression."
        Write-Host "Install UPX: winget install upx"
    }
} else {
    Write-Host "Build failed."
    exit 1
}

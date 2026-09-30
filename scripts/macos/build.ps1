$ErrorActionPreference = "Stop"

Write-Host "=== Compiling SlimBox for macOS (ARM64 and AMD64) ==="

$distDir = "dist/macos"
if (!(Test-Path $distDir)) {
    New-Item -ItemType Directory -Path $distDir -Force | Out-Null
}

# 1. ARM64 (Apple Silicon M1/M2/M3/M4)
Write-Host "-> Building darwin/arm64 binaries..."
$env:CGO_ENABLED = "0"
$env:GOOS = "darwin"
$env:GOARCH = "arm64"
go build -ldflags="-s -w" -o "$distDir/slimbox-core-arm64" ./cmd/slimbox
go build -ldflags="-s -w" -o "$distDir/slimbox-cli-arm64" ./cmd/slimbox-cli

# Keep slimbox-core as arm64 for compatibility with deploy_mac_app.go
Copy-Item -Path "$distDir/slimbox-core-arm64" -Destination "$distDir/slimbox-core" -Force

# 2. AMD64 (Intel Mac)
Write-Host "-> Building darwin/amd64 binaries..."
$env:GOARCH = "amd64"
go build -ldflags="-s -w" -o "$distDir/slimbox-core-amd64" ./cmd/slimbox
go build -ldflags="-s -w" -o "$distDir/slimbox-cli-amd64" ./cmd/slimbox-cli

Write-Host "=== Assembling Headless Packages ==="
# Stage arm64 tar.gz / zip
$stageArm = "$distDir/stage_arm64"
if (Test-Path $stageArm) { Remove-Item -Recurse -Force $stageArm }
New-Item -ItemType Directory -Path $stageArm -Force | Out-Null

Copy-Item -Path "$distDir/slimbox-core-arm64" -Destination "$stageArm/slimbox"
Copy-Item -Path "$distDir/slimbox-cli-arm64" -Destination "$stageArm/slimbox-cli"
Copy-Item -Path "scripts/macos/install.sh" -Destination "$stageArm/"
Copy-Item -Path "scripts/macos/uninstall.sh" -Destination "$stageArm/"
Copy-Item -Path "scripts/macos/com.slimbox.server.plist" -Destination "$stageArm/"
Copy-Item -Path "README.md" -Destination "$stageArm/"

Compress-Archive -Path "$stageArm/*" -DestinationPath "$distDir/slimbox-macos-arm64.zip" -Force
Remove-Item -Recurse -Force $stageArm

# Stage amd64 zip
$stageAmd = "$distDir/stage_amd64"
if (Test-Path $stageAmd) { Remove-Item -Recurse -Force $stageAmd }
New-Item -ItemType Directory -Path $stageAmd -Force | Out-Null

Copy-Item -Path "$distDir/slimbox-core-amd64" -Destination "$stageAmd/slimbox"
Copy-Item -Path "$distDir/slimbox-cli-amd64" -Destination "$stageAmd/slimbox-cli"
Copy-Item -Path "scripts/macos/install.sh" -Destination "$stageAmd/"
Copy-Item -Path "scripts/macos/uninstall.sh" -Destination "$stageAmd/"
Copy-Item -Path "scripts/macos/com.slimbox.server.plist" -Destination "$stageAmd/"
Copy-Item -Path "README.md" -Destination "$stageAmd/"

Compress-Archive -Path "$stageAmd/*" -DestinationPath "$distDir/slimbox-macos-amd64.zip" -Force
Remove-Item -Recurse -Force $stageAmd

Write-Host "=== macOS Binaries & Archives Built Successfully ==="
Get-ChildItem -Path $distDir

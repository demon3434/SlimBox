# SlimBox Windows Standalone Edition Packaging Script
$ErrorActionPreference = "Stop"

$RootDir = (Resolve-Path "$PSScriptRoot\..\..").Path
$DistDir = "$RootDir\dist\windows"
$StageDir = "$DistDir\stage"

Write-Host "==========================================================" -ForegroundColor Cyan
Write-Host "   Building SlimBox Windows Standalone Edition (x64)      " -ForegroundColor Cyan
Write-Host "==========================================================" -ForegroundColor Cyan

# 1. Clean staging directory
if (Test-Path $StageDir) {
    Remove-Item -Recurse -Force $StageDir
}
New-Item -ItemType Directory -Force -Path "$StageDir\bin" | Out-Null
New-Item -ItemType Directory -Force -Path "$StageDir\web" | Out-Null

# 2. Compile executables
Write-Host "[1/4] Compiling Windows executables (slimbox.exe, slimbox-cli.exe)..." -ForegroundColor Yellow
$env:GOOS = "windows"
$env:GOARCH = "amd64"

Push-Location $RootDir
try {
    # Main server binary compiled with Windows GUI subsystem (no flashing cmd black window, native system tray icon)
    go build -ldflags "-s -w -H=windowsgui" -o "$StageDir\slimbox.exe" ./cmd/slimbox
    # CLI tool compiled for console automation
    go build -ldflags "-s -w" -o "$StageDir\slimbox-cli.exe" ./cmd/slimbox-cli
} finally {
    Pop-Location
}

# 3. Stage dependencies & external Web assets
Write-Host "[2/4] Staging external web assets & dependency folders..." -ForegroundColor Yellow
Copy-Item -Recurse -Force "$RootDir\web\*" "$StageDir\web\"

# If local bin/ffmpeg.exe exists, bundle it; otherwise provide auto-setup instructions
if (Test-Path "$RootDir\bin\ffmpeg.exe") {
    Copy-Item -Force "$RootDir\bin\ffmpeg.exe" "$StageDir\bin\ffmpeg.exe"
    Write-Host "  -> Bundled single optimized ffmpeg.exe (redundant 227MB ffprobe.exe eliminated)" -ForegroundColor Green
} else {
    $ffmpegNotice = "SlimBox depends on FFmpeg for video hardware acceleration and transcoding.`r`nIf you do not have FFmpeg installed on system PATH, please download ffmpeg.exe and ffprobe.exe from gyan.dev or BtbN and place them inside this bin/ folder."
    Set-Content -Path "$StageDir\bin\README-FFmpeg.txt" -Value $ffmpegNotice -Encoding UTF8
    Write-Host "  -> Created bin/ directory with placement notice" -ForegroundColor DarkGray
}

# 4. Copy user documentation
Write-Host "[3/4] Copying documentation..." -ForegroundColor Yellow
Copy-Item "$PSScriptRoot\README-Windows.txt" "$StageDir\README-Windows.txt"

# 5. Archive to ZIP
Write-Host "[4/4] Creating ZIP archive..." -ForegroundColor Yellow
$ZipFile = "$DistDir\slimbox-windows-amd64.zip"
if (Test-Path $ZipFile) {
    Remove-Item -Force $ZipFile
}
Compress-Archive -Path "$StageDir\*" -DestinationPath $ZipFile -CompressionLevel Optimal

$zipSize = (Get-Item $ZipFile).Length / 1MB
Write-Host "==========================================================" -ForegroundColor Green
Write-Host "  Build Succeeded! Deliverable created:" -ForegroundColor Green
Write-Host ("     $ZipFile ({0:N2} MB)" -f $zipSize) -ForegroundColor Green
Write-Host "==========================================================" -ForegroundColor Green

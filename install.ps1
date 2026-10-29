# FileX Installation Script for Windows
# Usage: irm https://raw.githubusercontent.com/BaixuanZhu/FileX/main/install.ps1 | iex

$ErrorActionPreference = "Stop"

# Configuration
$RepoOwner = "BaixuanZhu"
$RepoName = "FileX"
$BinaryName = "filex.exe"
$InstallDir = Join-Path $env:LOCALAPPDATA "FileX"

function Write-Info {
    param([string]$Message)
    Write-Host "[INFO] " -ForegroundColor Cyan -NoNewline
    Write-Host $Message
}

function Write-Success {
    param([string]$Message)
    Write-Host "[OK] " -ForegroundColor Green -NoNewline
    Write-Host $Message
}

function Write-Error {
    param([string]$Message)
    Write-Host "[ERROR] " -ForegroundColor Red -NoNewline
    Write-Host $Message
}

function Get-LatestRelease {
    try {
        $url = "https://api.github.com/repos/$RepoOwner/$RepoName/releases/latest"
        $release = Invoke-RestMethod -Uri $url -Headers @{"User-Agent"="FileX-Installer"}
        return $release
    }
    catch {
        Write-Error "Failed to fetch latest release: $_"
        exit 1
    }
}

function Download-File {
    param(
        [string]$Url,
        [string]$OutputPath
    )
    
    try {
        Write-Info "Downloading from $Url..."
        Invoke-WebRequest -Uri $Url -OutFile $OutputPath -UseBasicParsing
        return $true
    }
    catch {
        Write-Error "Download failed: $_"
        return $false
    }
}

function Add-ToPath {
    param([string]$Directory)
    
    $currentPath = [Environment]::GetEnvironmentVariable("Path", "User")
    
    if ($currentPath -split ";" -contains $Directory) {
        Write-Info "$Directory is already in PATH"
        return
    }
    
    try {
        $newPath = "$currentPath;$Directory"
        [Environment]::SetEnvironmentVariable("Path", $newPath, "User")
        Write-Success "Added $Directory to user PATH"
        Write-Info "You may need to restart your terminal for PATH changes to take effect"
    }
    catch {
        Write-Error "Failed to add to PATH: $_"
        Write-Info "You can manually add $Directory to your PATH"
    }
}

function Install-FileX {
    Write-Info "Installing FileX..."
    
    # Get latest release
    $release = Get-LatestRelease
    $version = $release.tag_name
    $versionNumber = $version -replace '^v', ''
    Write-Info "Latest version: $version"
    
    # Find the Windows ZIP archive
    $archiveName = "filex-$version-windows-x64.zip"
    $asset = $release.assets | Where-Object { $_.name -eq $archiveName }
    if (-not $asset) {
        Write-Error "Archive asset '$archiveName' not found in release"
        exit 1
    }
    
    $downloadUrl = $asset.browser_download_url
    
    # Create install directory
    if (-not (Test-Path $InstallDir)) {
        New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null
        Write-Info "Created install directory: $InstallDir"
    }
    
    # Download archive
    $tempArchive = Join-Path $env:TEMP $archiveName
    if (-not (Download-File -Url $downloadUrl -OutputPath $tempArchive)) {
        exit 1
    }
    
    # Download checksums
    $checksumAsset = $release.assets | Where-Object { $_.name -eq "checksums.txt" }
    $expectedChecksum = $null
    if ($checksumAsset) {
        $checksumFile = Join-Path $env:TEMP "checksums.txt"
        if (Download-File -Url $checksumAsset.browser_download_url -OutputPath $checksumFile) {
            $checksumContent = Get-Content $checksumFile
            foreach ($line in $checksumContent) {
                if ($line -match "^(\w+)\s+$archiveName$") {
                    $expectedChecksum = $matches[1]
                    break
                }
            }
            Remove-Item $checksumFile -Force
        }
    }
    
    # Verify checksum if available
    if ($expectedChecksum) {
        Write-Info "Verifying checksum..."
        $actualChecksum = (Get-FileHash -Path $tempArchive -Algorithm SHA256).Hash.ToLower()
        if ($actualChecksum -ne $expectedChecksum) {
            Write-Error "Checksum mismatch!"
            Write-Error "Expected: $expectedChecksum"
            Write-Error "Actual: $actualChecksum"
            Remove-Item $tempArchive -Force
            exit 1
        }
        Write-Success "Checksum verified"
    }
    
    # Extract archive to temporary directory
    $tempExtractDir = Join-Path $env:TEMP "filex-extract-$versionNumber"
    if (Test-Path $tempExtractDir) {
        Remove-Item $tempExtractDir -Recurse -Force
    }
    
    Write-Info "Extracting archive..."
    try {
        Expand-Archive -Path $tempArchive -DestinationPath $tempExtractDir
    }
    catch {
        Write-Error "Failed to extract archive: $_"
        Remove-Item $tempArchive -Force
        exit 1
    }
    Remove-Item $tempArchive -Force
    
    # Find extracted directory
    $extractedDir = Get-ChildItem -Path $tempExtractDir -Directory | Select-Object -First 1
    if (-not $extractedDir) {
        Write-Error "Extracted directory not found"
        Remove-Item $tempExtractDir -Recurse -Force
        exit 1
    }
    
    # Create versioned directory structure
    $versionDir = Join-Path $InstallDir "versions\v$versionNumber"
    if (Test-Path $versionDir) {
        Remove-Item $versionDir -Recurse -Force
    }
    New-Item -ItemType Directory -Path $versionDir -Force | Out-Null
    
    # Move binary and docs
    Write-Info "Installing to $versionDir..."
    Move-Item -Path (Join-Path $extractedDir.FullName $BinaryName) -Destination $versionDir -Force
    if (Test-Path (Join-Path $extractedDir.FullName "docs")) {
        Copy-Item -Path (Join-Path $extractedDir.FullName "docs") -Destination $versionDir -Recurse -Force
    }
    if (Test-Path (Join-Path $extractedDir.FullName "README.md")) {
        Copy-Item -Path (Join-Path $extractedDir.FullName "README.md") -Destination $versionDir -Force
    }
    if (Test-Path (Join-Path $extractedDir.FullName "LICENSE")) {
        Copy-Item -Path (Join-Path $extractedDir.FullName "LICENSE") -Destination $versionDir -Force
    }
    
    # Cleanup extraction directory
    Remove-Item $tempExtractDir -Recurse -Force
    
    # Create current junction
    $currentLink = Join-Path $InstallDir "current"
    if (Test-Path $currentLink) {
        # Remove existing junction
        cmd /c rmdir "$currentLink" 2>$null
        if (Test-Path $currentLink) {
            Remove-Item $currentLink -Recurse -Force
        }
    }
    
    Write-Info "Creating current link..."
    cmd /c mklink /J "$currentLink" "$versionDir" | Out-Null
    if ($LASTEXITCODE -ne 0) {
        Write-Error "Failed to create junction"
        exit 1
    }
    Write-Success "Current version linked"
    
    # Create wrapper script
    $wrapperPath = Join-Path $InstallDir "filex.bat"
    $wrapperContent = @"
@echo off
setlocal

set FILEX_HOME=$InstallDir
set FILEX_CURRENT=%FILEX_HOME%\current

if not exist "%FILEX_CURRENT%\$BinaryName" (
    echo Error: FileX current version not found
    echo Please run: filex upgrade
    exit /b 1
)

"%FILEX_CURRENT%\$BinaryName" %*
"@
    Set-Content -Path $wrapperPath -Value $wrapperContent -Encoding ASCII
    Write-Success "Wrapper script created"
    
    # Add wrapper location to PATH
    Add-ToPath -Directory $InstallDir
    
    # Verify installation
    Write-Info "Verifying installation..."
    try {
        $installedBinary = Join-Path $currentLink $BinaryName
        $versionOutput = & $installedBinary version 2>&1 | ConvertFrom-Json
        if ($versionOutput.ok) {
            Write-Success "FileX $($versionOutput.data.version) installed successfully!"
        }
    }
    catch {
        Write-Error "Installation verification failed"
        exit 1
    }
    
    Write-Host ""
    Write-Success "Installation complete!"
    Write-Host ""
    Write-Host "Installation structure:"
    Write-Host "  $InstallDir\"
    Write-Host "  ├── filex.bat (wrapper)"
    Write-Host "  ├── current\ -> versions\v$versionNumber\"
    Write-Host "  └── versions\"
    Write-Host "      └── v$versionNumber\"
    Write-Host "          ├── $BinaryName"
    Write-Host "          ├── README.md"
    Write-Host "          ├── LICENSE"
    Write-Host "          └── docs\"
    Write-Host ""
    Write-Host "Usage:"
    Write-Host "  filex help          Show help"
    Write-Host "  filex version       Show version"
    Write-Host "  filex upgrade       Upgrade to latest version"
    Write-Host "  filex upgrade --list    List installed versions"
    Write-Host "  filex upgrade --use v$versionNumber    Switch to version"
    Write-Host "  filex upgrade --cleanup    Remove old versions"
    Write-Host ""
    Write-Host "Documentation: https://github.com/$RepoOwner/$RepoName"
    Write-Host ""
}

# Run installation
Install-FileX

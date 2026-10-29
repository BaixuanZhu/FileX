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
    Write-Info "Latest version: $version"
    
    # Find the binary asset
    $asset = $release.assets | Where-Object { $_.name -eq $BinaryName }
    if (-not $asset) {
        Write-Error "Binary asset '$BinaryName' not found in release"
        exit 1
    }
    
    $downloadUrl = $asset.browser_download_url
    
    # Create install directory
    if (-not (Test-Path $InstallDir)) {
        New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null
        Write-Info "Created install directory: $InstallDir"
    }
    
    # Download binary
    $tempFile = Join-Path $env:TEMP $BinaryName
    if (-not (Download-File -Url $downloadUrl -OutputPath $tempFile)) {
        exit 1
    }
    
    # Download checksums
    $checksumAsset = $release.assets | Where-Object { $_.name -eq "checksums.txt" }
    $expectedChecksum = $null
    if ($checksumAsset) {
        $checksumFile = Join-Path $env:TEMP "checksums.txt"
        if (Download-File -Url $checksumAsset.browser_download_url -OutputFile $checksumFile) {
            $checksumContent = Get-Content $checksumFile
            $expectedChecksum = ($checksumContent -split "\s+")[0]
            Remove-Item $checksumFile -Force
        }
    }
    
    # Verify checksum if available
    if ($expectedChecksum) {
        Write-Info "Verifying checksum..."
        $actualChecksum = (Get-FileHash -Path $tempFile -Algorithm SHA256).Hash.ToLower()
        if ($actualChecksum -ne $expectedChecksum) {
            Write-Error "Checksum mismatch!"
            Write-Error "Expected: $expectedChecksum"
            Write-Error "Actual: $actualChecksum"
            Remove-Item $tempFile -Force
            exit 1
        }
        Write-Success "Checksum verified"
    }
    
    # Install binary
    $installPath = Join-Path $InstallDir $BinaryName
    
    # Backup existing installation
    if (Test-Path $installPath) {
        $backupPath = "$installPath.backup"
        Move-Item -Path $installPath -Destination $backupPath -Force
        Write-Info "Backed up existing installation"
    }
    
    # Move new binary to install location
    try {
        Move-Item -Path $tempFile -Destination $installPath -Force
        Write-Success "Installed to $installPath"
    }
    catch {
        Write-Error "Failed to install: $_"
        # Restore backup if it exists
        if (Test-Path "$installPath.backup") {
            Move-Item -Path "$installPath.backup" -Destination $installPath -Force
            Write-Info "Restored previous installation"
        }
        exit 1
    }
    
    # Add to PATH
    Add-ToPath -Directory $InstallDir
    
    # Clean up backup
    if (Test-Path "$installPath.backup") {
        Remove-Item "$installPath.backup" -Force
    }
    
    # Verify installation
    Write-Info "Verifying installation..."
    try {
        $versionOutput = & $installPath version 2>&1 | ConvertFrom-Json
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
    Write-Host "Usage:"
    Write-Host "  filex help          Show help"
    Write-Host "  filex version       Show version"
    Write-Host "  filex upgrade       Upgrade to latest version"
    Write-Host ""
    Write-Host "Documentation: https://github.com/$RepoOwner/$RepoName"
    Write-Host ""
}

# Run installation
Install-FileX

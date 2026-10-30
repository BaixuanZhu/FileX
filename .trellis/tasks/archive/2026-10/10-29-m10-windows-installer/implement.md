# M10: Windows Installer - Implementation Plan

## Prerequisites

- [ ] NSIS 3.x installed locally for testing
- [ ] EnVar plugin for NSIS downloaded
- [ ] Test Windows VM or sandbox available

## Phase 1: NSIS Script Development

### 1.1 Create Installer Directory Structure

```bash
mkdir -p installer/
mkdir -p installer/plugins
```

### 1.2 Download and Configure EnVar Plugin

- Download EnVar plugin from https://nsis.sourceforge.io/EnVar_plug-in
- Extract to `installer/plugins/`
- Verify plugin files:
  - `Plugins/x86-unicode/EnVar.dll`

### 1.3 Write NSIS Script (`installer/filex.nsi`)

**Structure**:
```nsi
; Basic info
Name "FileX"
OutFile "filex-setup.exe"
InstallDir "$LOCALAPPDATA\FileX"

; Includes
!include "MUI2.nsh"
!include "EnVar.nsh"

; Pages
!insertmacro MUI_PAGE_WELCOME
!insertmacro MUI_PAGE_DIRECTORY
!insertmacro MUI_PAGE_COMPONENTS
!insertmacro MUI_PAGE_INSTFILES
!insertmacro MUI_PAGE_FINISH

; Sections
Section "Core Files" SecCore
  ; Extract archive
  ; Create directory structure
  ; Create current junction
  ; Install wrapper script
SectionEnd

Section "Add to PATH" SecPath
  ; Add to user PATH
SectionEnd

Section "Start Menu Shortcut" SecStartMenu
  ; Create shortcut
SectionEnd

Section "Desktop Shortcut" SecDesktop
  ; Create shortcut
SectionEnd

; Uninstaller section
Section "Uninstall"
  ; Remove from PATH
  ; Delete files
  ; Remove shortcuts
  ; Remove registry keys
SectionEnd
```

**Key Implementation Details**:

1. **Archive Extraction**:
   - Bundle pre-built ZIP archive (`filex-v{version}-windows-x64.zip`)
   - Extract to `$INSTDIR\versions\v{version}\`
   - Create junction: `current -> versions\v{version}`

2. **Wrapper Script**:
   - Create `filex.bat` in `$INSTDIR`
   - Content: `@"%~dp0current\filex.exe" %*`

3. **PATH Management**:
   - Use `EnVar::AddToPath` with `$INSTDIR`
   - Idempotent: plugin checks for duplicates

4. **Registry Keys**:
   - `HKCU\Software\FileX\InstallPath` = `$INSTDIR`
   - `HKCU\Software\FileX\Version` = `{version}`

### 1.4 Test Installer Locally

**Test Cases**:
- [ ] Fresh installation
  - Run installer
  - Verify files extracted to `%LOCALAPPDATA%\FileX\versions\v{version}\`
  - Verify `current` junction created
  - Verify `filex.bat` wrapper installed
  - Verify PATH updated (`echo %PATH%` includes install dir)
  - Verify `filex --version` works
  - Verify Start Menu shortcut created

- [ ] Uninstallation
  - Run uninstaller
  - Verify files removed
  - Verify PATH entry removed
  - Verify shortcuts removed
  - Verify registry keys removed

- [ ] Silent installation
  - Run `filex-setup.exe /S`
  - Verify installation completes without UI
  - Verify all components installed

- [ ] Custom directory
  - Run installer with custom path (e.g., `D:\Tools\FileX`)
  - Verify installation to custom path
  - Verify PATH updated correctly

## Phase 2: CI/CD Integration

### 2.1 Update Release Workflow

**File**: `.github/workflows/release.yml`

**Add Steps**:
```yaml
- name: Install NSIS
  run: |
    choco install nsis -y
    # Download EnVar plugin
    curl -L -o envar-plugin.zip https://nsis.sourceforge.io/...
    # Extract to NSIS plugins directory

- name: Build Installer
  shell: bash
  run: |
    # Download Windows ZIP archive from artifacts
    # Run NSIS to build installer
    makensis -DVERSION=${{ steps.version.outputs.tag }} installer/filex.nsi
    # Output: filex-setup.exe

- name: Upload Installer
  uses: actions/upload-artifact@v4
  with:
    name: installer
    path: filex-setup.exe

- name: Add Installer to Release
  uses: softprops/action-gh-release@v2
  with:
    files: filex-setup.exe
```

### 2.2 Test Installer in CI

**Add Job**:
```yaml
test-installer:
  needs: build
  runs-on: windows-latest
  steps:
    - name: Download installer
      uses: actions/download-artifact@v4
    
    - name: Run installer
      run: |
        Start-Process -FilePath ".\filex-setup.exe" -ArgumentList "/S" -Wait
    
    - name: Verify installation
      run: |
        # Verify filex command works
        filex --version
        # Verify installation directory
        Test-Path "$env:LOCALAPPDATA\FileX\filex.bat"
```

### 2.3 Update Release Notes

**Add Section**:
```markdown
## Installation

### Windows Installer (Recommended)
Download `filex-setup.exe` and run it. The installer will guide you through the installation process.

### PowerShell (Alternative)
```powershell
irm https://raw.githubusercontent.com/BaixuanZhu/FileX/main/install.ps1 | iex
```
```

## Phase 3: Documentation

### 3.1 Update README

**Add Section**:
```markdown
## Installation

### Windows Installer (Recommended)

1. Download `filex-setup.exe` from [Releases](https://github.com/BaixuanZhu/FileX/releases)
2. Run the installer
3. Follow the installation wizard
4. Open a new terminal and run `filex --version`

The installer will:
- Install FileX to `%LOCALAPPDATA%\FileX`
- Add FileX to your PATH
- Create a Start Menu shortcut

### Alternative Installation Methods

<details>
<summary>PowerShell Script</summary>

```powershell
irm https://raw.githubusercontent.com/BaixuanZhu/FileX/main/install.ps1 | iex
```
</details>

<details>
<summary>Go Install</summary>

```bash
go install github.com/BaixuanZhu/FileX/cmd/filex@latest
```
</details>
```

### 3.2 Create Installer Documentation

**File**: `docs/INSTALLER.md`

**Content**:
- Installation instructions
- Silent installation guide
- Uninstallation instructions
- Troubleshooting common issues
- Custom installation directory

## Phase 4: Testing and Validation

### 4.1 Manual Testing Checklist

**Fresh Installation**:
- [ ] Installer runs without errors
- [ ] Files extracted to correct location
- [ ] PATH updated correctly
- [ ] `filex --version` works in new terminal
- [ ] Start Menu shortcut works
- [ ] Desktop shortcut works (if selected)

**Upgrade**:
- [ ] Installer detects existing installation
- [ ] Upgrade completes without data loss
- [ ] Old versions cleaned up
- [ ] New version works correctly

**Uninstallation**:
- [ ] Uninstaller runs without errors
- [ ] All files removed
- [ ] PATH entry removed
- [ ] Shortcuts removed
- [ ] Registry keys removed

**Edge Cases**:
- [ ] Installation with non-ASCII characters in path
- [ ] Installation with spaces in path
- [ ] Installation on network drive
- [ ] Installation with antivirus running

### 4.2 Automated Testing

**CI Tests**:
- [ ] Installer builds successfully
- [ ] Installer size < 5 MB
- [ ] Silent installation works
- [ ] `filex --version` works after installation

## Phase 5: Release

### 5.1 Pre-Release Checklist

- [ ] All tests passing
- [ ] Installer tested on clean Windows VM
- [ ] Documentation updated
- [ ] Release notes drafted

### 5.2 Release Process

1. Create PR with installer changes
2. Review and merge PR
3. Create new tag (e.g., `v0.3.0`)
4. Monitor CI build
5. Verify installer uploaded to release
6. Test installer from release page
7. Update release notes if needed

### 5.3 Post-Release

- [ ] Announce new installer in release notes
- [ ] Update installation documentation
- [ ] Monitor for issues/bugs
- [ ] Collect user feedback

## Rollback Plan

If installer has critical issues:
1. Remove installer from release page
2. Keep ZIP archive as fallback
3. Fix issues in installer script
4. Rebuild and re-upload installer
5. Update release notes

## Success Criteria

- [ ] Installer builds in CI without errors
- [ ] Installer size < 5 MB
- [ ] Fresh installation works on Windows 10/11
- [ ] PATH updated correctly
- [ ] `filex --version` works immediately
- [ ] Uninstaller removes all components
- [ ] Silent installation works
- [ ] Installer uploaded to GitHub Releases
- [ ] README updated with installer instructions

## Timeline Estimate

- Phase 1 (NSIS Development): 4-6 hours
- Phase 2 (CI/CD Integration): 2-3 hours
- Phase 3 (Documentation): 1-2 hours
- Phase 4 (Testing): 2-3 hours
- Phase 5 (Release): 1 hour

**Total**: 10-15 hours

## Risks and Mitigations

| Risk | Impact | Mitigation |
|------|--------|------------|
| EnVar plugin not available in CI | High | Bundle plugin with installer source |
| PATH update fails | High | Test on multiple Windows versions, provide manual instructions |
| Installer blocked by SmartScreen | Medium | Document workaround, consider code signing in future |
| Upgrade detection fails | Medium | Provide uninstall-first instructions |
| NSIS script complexity | Medium | Start with minimal installer, add features incrementally |

## Dependencies

- NSIS 3.x (CI: `choco install nsis`)
- EnVar plugin for NSIS
- Existing ZIP archive from build step
- GitHub Actions Windows runner

## Notes

- Installer will be unsigned initially (SmartScreen warning expected)
- User-level installation only (no admin required)
- English-only for MVP (multi-language can be added later)
- Installer is complementary to existing installation methods (not a replacement)

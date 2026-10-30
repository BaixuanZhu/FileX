# M10: Windows Installer

## Overview

Create a professional Windows installer for FileX using NSIS or Inno Setup. The installer should provide a guided installation experience, handle PATH configuration, and integrate with the existing release workflow.

## Motivation

Current installation methods:
- PowerShell script (manual, requires user to run command)
- Go install (requires Go toolchain)
- Manual binary download (no PATH setup, no shortcuts)

A proper installer improves user experience and adoption on Windows.

## Requirements

### Functional Requirements

1. **Installation Directory**
   - Default: `%LOCALAPPDATA%\FileX` (consistent with current structure)
   - Allow user to choose custom directory
   - Support both "Install for me" (user-level) and "Install for all users" (system-level)

2. **PATH Configuration**
   - Automatically add installation directory to user PATH
   - Handle PATH updates idempotently (no duplicates)
   - Broadcast environment change notification

3. **Shortcuts**
   - Create Start Menu shortcut (optional, checked by default)
   - Create Desktop shortcut (optional, unchecked by default)

4. **Uninstallation**
   - Remove binaries and installation directory
   - Remove from PATH (restore original PATH)
   - Remove shortcuts
   - Optional: keep configuration directory (`~/.filex/`)

5. **Upgrade Support**
   - Detect existing installation
   - Offer to upgrade or uninstall first
   - Preserve configuration during upgrade

6. **Silent Installation**
   - Support `/S` flag for silent install
   - Support `/D=<path>` for custom directory

### Non-Functional Requirements

1. **Installer Size**: < 5 MB (compressed binaries + installer overhead)
2. **Signing**: Support code signing (optional, requires certificate)
3. **Localization**: English (primary), Chinese (optional)
4. **Compatibility**: Windows 10/11 (x64)

## Acceptance Criteria

- [ ] Installer can be built locally with NSIS or Inno Setup
- [ ] Installer installs FileX to `%LOCALAPPDATA%\FileX` by default
- [ ] Installer adds installation directory to user PATH
- [ ] `filex --version` works immediately after installation (no restart required)
- [ ] Installer creates Start Menu shortcut
- [ ] Uninstaller removes all files and PATH entries
- [ ] Installer supports silent installation (`/S`)
- [ ] Release workflow generates `.exe` installer artifact
- [ ] Installer is uploaded to GitHub Releases alongside ZIP archive

## Scope

### In Scope
- Windows x64 installer (NSIS or Inno Setup)
- PATH management
- Shortcut creation
- Uninstaller
- Release workflow integration

### Out of Scope
- Code signing (can be added later if certificate is available)
- Multi-language support (English only for MVP)
- System-level installation (user-level only for MVP)
- Linux/macOS installers (out of scope for M10)

## Technical Decisions

### NSIS vs Inno Setup

**Recommendation: NSIS**

Rationale:
- More flexible and scriptable
- Better PATH management plugins (EnVar plugin)
- Widely used in open-source projects
- Can be automated in CI/CD
- Smaller installer overhead

Alternative: Inno Setup
- Easier to learn (Pascal scripting)
- Better documentation
- Less flexible for complex scenarios

### Integration with Existing Structure

The installer will work with the JVM-style version management:
- Install to `%LOCALAPPDATA%\FileX` (root directory)
- Extract versioned archive to `versions\v{version}\`
- Create `current` junction pointing to active version
- Add root directory to PATH (wrapper script handles resolution)

## Dependencies

- NSIS 3.x (installed in CI)
- EnVar plugin for NSIS (PATH management)
- Existing release workflow (`.github/workflows/release.yml`)

## Rollout Plan

1. Create NSIS script (`installer/filex.nsi`)
2. Test installer locally
3. Add NSIS build step to release workflow
4. Upload installer to GitHub Releases
5. Update README with installer download link
6. Deprecate PowerShell install script (keep for backward compatibility)

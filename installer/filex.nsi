; FileX Windows Installer
; NSIS script for installing FileX with PATH management

!include "MUI2.nsh"
!include "FileFunc.nsh"
!include "WinMessages.nsh"
!insertmacro GetSize
!insertmacro GetOptions

; Version information (passed via command line)
!define /ifndef VERSION "0.0.0"

; Basic information
Name "FileX"
BrandingText "FileX - Deterministic File Operations for AI Agents"
; Output name matches the release archive naming (filex-vX.Y.Z-windows-x64.zip)
OutFile "filex-v${VERSION}-windows-x64.exe"

; User-level installation (no admin required)
RequestExecutionLevel user

; Compression
SetCompressor /SOLID lzma

; Installation directory
InstallDir "$LOCALAPPDATA\FileX"
InstallDirRegKey HKCU "Software\FileX" "InstallPath"

; MUI2 configuration
!define MUI_ABORTWARNING

; Pages
; Re-add a license page (!insertmacro MUI_PAGE_LICENSE "LICENSE") once the
; project ships a real LICENSE file; makensis is expected to run from the
; repository root so paths here are root-relative.
!insertmacro MUI_PAGE_WELCOME
!insertmacro MUI_PAGE_COMPONENTS
!insertmacro MUI_PAGE_DIRECTORY
!insertmacro MUI_PAGE_INSTFILES
!insertmacro MUI_PAGE_FINISH

; Uninstaller pages
!insertmacro MUI_UNPAGE_CONFIRM
!insertmacro MUI_UNPAGE_INSTFILES

; Language
!insertmacro MUI_LANGUAGE "English"

; Version metadata shown in the installer's file properties
; VIProductVersion requires four numeric components (X.X.X.X)
VIProductVersion "${VERSION}.0"
VIAddVersionKey /LANG=${LANG_ENGLISH} "ProductName" "FileX"
VIAddVersionKey /LANG=${LANG_ENGLISH} "LegalCopyright" "Copyright (c) BaixuanZhu"
VIAddVersionKey /LANG=${LANG_ENGLISH} "FileDescription" "FileX Installer"
VIAddVersionKey /LANG=${LANG_ENGLISH} "FileVersion" "${VERSION}"
VIAddVersionKey /LANG=${LANG_ENGLISH} "ProductVersion" "${VERSION}"

; Installer sections
Section "Core Files" SecCore
  SectionIn RO ; Required
  
  ; Set output directory
  SetOutPath "$INSTDIR\versions\v${VERSION}"
  
  ; Copy bundled files (extracted from ZIP in CI); NSIS resolves File paths
  ; relative to this script's directory
  File /r "bundled\*.*"
  
  ; Switch to install directory
  SetOutPath "$INSTDIR"
  
  ; Drop any junction from a previous install so it re-points to this version;
  ; RMDir on a junction removes the link itself, not the target contents
  RMDir "$INSTDIR\current"
  
  ; Create current junction
  ExecWait 'cmd /c mklink /J "current" "versions\v${VERSION}"' $0
  
  ; Create wrapper script
  FileOpen $0 "$INSTDIR\filex.bat" w
  FileWrite $0 '@echo off$\r$\n'
  FileWrite $0 'setlocal$\r$\n'
  FileWrite $0 'set "FILEX_HOME=%~dp0"$\r$\n'
  FileWrite $0 '"%FILEX_HOME%current\filex.exe" %*$\r$\n'
  FileWrite $0 'endlocal$\r$\n'
  FileClose $0
  
  ; Write registry keys
  WriteRegStr HKCU "Software\FileX" "InstallPath" "$INSTDIR"
  WriteRegStr HKCU "Software\FileX" "Version" "${VERSION}"
  
  ; Create uninstaller
  WriteUninstaller "$INSTDIR\uninstaller.exe"
  
  ; Register in Add/Remove Programs
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\FileX" \
    "DisplayName" "FileX"
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\FileX" \
    "UninstallString" "$\"$INSTDIR\uninstaller.exe$\""
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\FileX" \
    "InstallLocation" "$INSTDIR"
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\FileX" \
    "Publisher" "BaixuanZhu"
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\FileX" \
    "DisplayVersion" "${VERSION}"
  WriteRegDWORD HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\FileX" \
    "NoModify" 1
  WriteRegDWORD HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\FileX" \
    "NoRepair" 1
  
  ; Calculate and store installed size
  ${GetSize} "$INSTDIR" "/S=0K" $0 $1 $2
  IntFmt $0 "0x%08X" $0
  WriteRegDWORD HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\FileX" \
    "EstimatedSize" "$0"
SectionEnd

Section "Add to PATH" SecPath
  Call AddToUserPath
SectionEnd

Section "Start Menu Shortcut" SecStartMenu
  CreateDirectory "$SMPROGRAMS\FileX"
  CreateShortcut "$SMPROGRAMS\FileX\FileX.lnk" "$INSTDIR\filex.bat"
  CreateShortcut "$SMPROGRAMS\FileX\Uninstall FileX.lnk" "$INSTDIR\uninstaller.exe"
SectionEnd

Section /o "Desktop Shortcut" SecDesktop
  CreateShortcut "$DESKTOP\FileX.lnk" "$INSTDIR\filex.bat"
SectionEnd

; Uninstaller section
Section "Uninstall"
  ; Remove from PATH
  Call un.RemoveFromUserPath
  
  ; Delete installation directory
  RMDir /r "$INSTDIR"
  
  ; Delete shortcuts
  RMDir /r "$SMPROGRAMS\FileX"
  Delete "$DESKTOP\FileX.lnk"
  
  ; Delete registry keys
  DeleteRegKey HKCU "Software\FileX"
  DeleteRegKey HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\FileX"
SectionEnd

; Section descriptions
!insertmacro MUI_FUNCTION_DESCRIPTION_BEGIN
  !insertmacro MUI_DESCRIPTION_TEXT ${SecCore} "Core FileX files (required)"
  !insertmacro MUI_DESCRIPTION_TEXT ${SecPath} "Add FileX to user PATH (recommended)"
  !insertmacro MUI_DESCRIPTION_TEXT ${SecStartMenu} "Create Start Menu shortcuts"
  !insertmacro MUI_DESCRIPTION_TEXT ${SecDesktop} "Create Desktop shortcut"
!insertmacro MUI_FUNCTION_DESCRIPTION_END

; Function to add directory to user PATH
Function AddToUserPath
  Push $0
  Push $1
  
  ; Read current user PATH
  ReadRegStr $0 HKCU "Environment" "PATH"
  
  ; Check if already in PATH (case-insensitive)
  Push "$INSTDIR"
  Push "$0"
  Call StrStr
  Pop $1
  StrCmp $1 "" 0 PathAlreadySet
  
  ; Add to PATH
  StrCmp $0 "" 0 AddSemicolon
  StrCpy $0 "$INSTDIR"
  Goto WritePath
  
  AddSemicolon:
  StrCpy $0 "$0;$INSTDIR"
  
  WritePath:
  WriteRegExpandStr HKCU "Environment" "PATH" $0
  
  ; Broadcast WM_SETTINGCHANGE to notify applications
  SendMessage ${HWND_BROADCAST} ${WM_SETTINGCHANGE} 0 "STR:Environment" /TIMEOUT=5000
  
  PathAlreadySet:
  Pop $1
  Pop $0
FunctionEnd

; Helper function: Find substring in string
; Input: Stack[0] = haystack, Stack[1] = needle
; Output: Stack[0] = result (empty if not found)
Function StrStr
  Exch $R0 ; haystack
  Exch
  Exch $R1 ; needle
  Push $R2
  Push $R3
  Push $R4
  Push $R5
  
  StrLen $R2 $R1
  StrLen $R3 $R0
  
  IntCmp $R2 0 0 0 StrStrNotFound
  IntCmp $R3 0 StrStrNotFound StrStrNotFound 0
  
  StrCpy $R4 0
  
  StrStrLoop:
  IntCmp $R4 $R3 StrStrNotFound StrStrNotFound 0
  StrCpy $R5 $R0 $R2 $R4
  StrCmp $R5 $R1 StrStrFound 0
  IntOp $R4 $R4 + 1
  Goto StrStrLoop
  
  StrStrFound:
  StrCpy $R0 $R0 "" $R4
  Goto StrStrDone
  
  StrStrNotFound:
  StrCpy $R0 ""
  
  StrStrDone:
  Pop $R5
  Pop $R4
  Pop $R3
  Pop $R2
  Pop $R1
  Exch $R0
FunctionEnd

; Uninstaller functions
; Removes the install dir whether it sits at the start, middle, or end of
; PATH, or is PATH's only entry: replace ";X", then "X;", then "X"
Function un.RemoveFromUserPath
  Push $0
  
  ReadRegStr $0 HKCU "Environment" "PATH"
  StrCmp $0 "" Done
  
  Push $0
  Push ";$INSTDIR"
  Push ""
  Call un.StrRep
  Pop $0
  
  Push $0
  Push "$INSTDIR;"
  Push ""
  Call un.StrRep
  Pop $0
  
  Push $0
  Push "$INSTDIR"
  Push ""
  Call un.StrRep
  Pop $0
  
  StrCmp $0 "" 0 WritePath
  ; Avoid leaving an empty PATH value behind when it had no other entries
  DeleteRegValue HKCU "Environment" "PATH"
  Goto Broadcast
  
  WritePath:
  WriteRegExpandStr HKCU "Environment" "PATH" $0
  
  Broadcast:
  SendMessage ${HWND_BROADCAST} ${WM_SETTINGCHANGE} 0 "STR:Environment" /TIMEOUT=5000
  
  Done:
  Pop $0
FunctionEnd

Function un.StrRep
  Exch $R0
  Exch
  Exch $R1
  Exch 2
  Exch $R2
  Push $R3
  Push $R4
  Push $R5
  Push $R6
  
  StrCpy $R3 ""
  StrLen $R4 $R1
  
  StrRepLoop:
  StrCpy $R5 $R2 $R4
  StrCmp $R5 $R1 0 StrRepNoMatch
  StrCpy $R5 $R2 "" $R4
  StrCpy $R3 "$R3$R0"
  StrCpy $R2 $R5
  StrLen $R4 $R1
  Goto StrRepLoop
  
  StrRepNoMatch:
  StrCmp $R2 "" StrRepDone 0
  StrCpy $R5 $R2 1
  StrCpy $R3 "$R3$R5"
  StrCpy $R2 $R2 "" 1
  Goto StrRepLoop
  
  StrRepDone:
  StrCpy $R0 $R3
  
  Pop $R6
  Pop $R5
  Pop $R4
  Pop $R3
  Pop $R2
  Pop $R1
  Exch $R0
FunctionEnd

; Installer initialization
Function .onInit
  ; An explicit /D= on the command line is a deliberate location choice and
  ; wins over the previous installation's directory. NSIS processes /D= first,
  ; but InstallDirRegKey (line 27) overrides it with the registry value.
  ; We must detect /D= here and restore it.
  Push $R0
  ClearErrors
  ${GetOptions} "$CMDLINE" "/D=" $R0
  IfErrors +3
    StrCpy $INSTDIR $R0
    Goto NoExisting
  Pop $R0
  
  ; Check for existing installation
  ReadRegStr $0 HKCU "Software\FileX" "InstallPath"
  StrCmp $0 "" NoExisting
  
  ; Existing installation found
  ; /SD IDYES lets silent (/S) upgrades proceed instead of aborting
  MessageBox MB_YESNO "FileX is already installed at $0.$\n$\nDo you want to upgrade to version ${VERSION}?" \
    /SD IDYES IDYES Upgrade IDNO Abort
  
  Abort:
  Abort
  
  Upgrade:
  StrCpy $INSTDIR $0
  
  NoExisting:
FunctionEnd

@echo off
setlocal

set FILEX_HOME=%LOCALAPPDATA%\FileX
set FILEX_CURRENT=%FILEX_HOME%\current

if not exist "%FILEX_CURRENT%\filex.exe" (
    echo Error: FileX current version not found
    echo Please run: filex upgrade
    exit /b 1
)

"%FILEX_CURRENT%\filex.exe" %*

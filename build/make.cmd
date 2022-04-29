@echo off

setlocal

set RegKey="HKLM\SOFTWARE\Microsoft\Windows Kits\Installed Roots"
set RegVal="KitsRoot10"
reg query %RegKey% /v %RegVal% >nul 2>&1 || (echo Cannot find Windows Kit >&2 & exit /b 1)
for /f "tokens=2,*" %%i in ('reg query %RegKey% /v %RegVal% ^| findstr %RegVal%') do (
    set KitsRoot=%%j
)
for /f "tokens=*" %%i in ('reg query %RegKey% /f * /k ^| findstr "\10."') do (
    set KitsInst=%%~nxi
)

set PATH=%KitsRoot%bin\%KitsInst%\x64;%WIX%\bin;%PATH%
rem set CPATH=C:\Program Files (x86)\WinFsp\inc\fuse
set CGO_ENABLED=0

for /f %%d in ('powershell -NoProfile -NonInteractive -ExecutionPolicy Unrestricted "$d=[System.DateTime]::Now; $d.ToString('yy')+$d.DayOfYear.ToString('000')"') do (
    set MyBuildNumber=%%d
)

mingw32-make -C %~dp0 MyBuildNumber=%MyBuildNumber% %*

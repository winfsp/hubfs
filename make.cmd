@echo off

setlocal

set CPATH=C:\Program Files (x86)\WinFsp\inc\fuse

for /f %%d in ('powershell -NoProfile -NonInteractive -ExecutionPolicy Unrestricted "$d=[System.DateTime]::Now; $d.ToString('yy')+$d.DayOfYear.ToString('000')"') do (
    set MyBuildNumber=%%d
)

mingw32-make MyBuildNumber=%MyBuildNumber% %*

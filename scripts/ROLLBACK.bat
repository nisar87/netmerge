@echo off
rem Puts the previous NetMerge version back (use if the newest one misbehaves). Keep this file next to NetMerge.exe.
set "D=%~dp0"
if not exist "%D%NetMerge-previous.exe" (echo No previous version saved. & pause & exit /b)
powershell -NoProfile -Command "Start-Process powershell -Verb RunAs -Wait -ArgumentList '-NoProfile -Command Stop-Process -Name NetMerge -Force -ErrorAction SilentlyContinue; Start-Sleep 2; Copy-Item -Force ''%D%NetMerge-previous.exe'' ''%D%NetMerge.exe''; Start-Process ''%D%NetMerge.exe'''"
echo Done.
pause

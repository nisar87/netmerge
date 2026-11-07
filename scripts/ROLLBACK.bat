@echo off
rem Puts the previous NetMerge version back (use if the newest one misbehaves).
if not exist "E:\NetMerge\NetMerge-previous.exe" (echo No previous version saved. & pause & exit /b)
powershell -NoProfile -Command "Start-Process powershell -Verb RunAs -Wait -ArgumentList '-NoProfile -Command Stop-Process -Name NetMerge,mihomo -Force -ErrorAction SilentlyContinue; Start-Sleep 2; Copy-Item -Force E:\NetMerge\NetMerge-previous.exe E:\NetMerge\NetMerge.exe; Start-Process E:\NetMerge\NetMerge.exe'"
echo Done.
pause

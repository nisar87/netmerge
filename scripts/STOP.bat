@echo off
REM Emergency switch: stops NetMerge and its engine, restoring your normal internet.
powershell -NoProfile -ExecutionPolicy Bypass -Command "Start-Process powershell -Verb RunAs -ArgumentList '-NoProfile -Command Stop-Process -Name NetMerge,mihomo -Force -ErrorAction SilentlyContinue'"
echo NetMerge stopped - normal internet restored.
timeout /t 2 >nul

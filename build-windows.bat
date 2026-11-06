@echo off
setlocal
where go >nul 2>nul || (echo Install Go from https://go.dev/dl/ first. & exit /b 1)
set GOOS=windows
set GOARCH=amd64
go build -trimpath -ldflags="-s -w -H=windowsgui" -o VioletTweaks.exe .
if errorlevel 1 exit /b 1
echo Built VioletTweaks.exe

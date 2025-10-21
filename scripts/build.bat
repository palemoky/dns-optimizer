@echo off

pushd %~dp0\..

echo Building dns-optimizer...

REM 创建一个用于存放构建结果的目录
if exist builds (
    rmdir /s /q builds
)
mkdir builds

echo --> Building for Windows (amd64)...
set GOOS=windows
set GOARCH=amd64
go build -ldflags="-s -w" -o ./builds/dns-optimizer-windows-amd64.exe .

echo --> Building for Linux (amd64)...
set GOOS=linux
set GOARCH=amd64
go build -ldflags="-s -w" -o ./builds/dns-optimizer-linux-amd64 .

echo.
echo All builds completed successfully! Find them in the 'builds' directory.

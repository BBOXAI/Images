@echo off
chcp 65001
echo Building Windows amd64...
set GOOS=windows
set GOARCH=amd64
go build -o webpimg-windows-amd64.exe main.go

echo Building Linux amd64...
set GOOS=linux
set GOARCH=amd64
go build -o webpimg-linux-amd64 main.go

echo Build completed!
echo Windows version: webpimg-windows-amd64.exe
echo Linux version: webpimg-linux-amd64
pause
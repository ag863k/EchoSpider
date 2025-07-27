@echo off
setlocal enabledelayedexpansion

echo ================================
echo    EchoSpider Build Script
echo ================================
echo.

REM Check if Go is installed
where go >nul 2>nul
if %errorlevel% neq 0 (
    echo Error: Go is not installed or not in PATH
    echo Please install Go from https://golang.org/dl/
    echo After installation, restart your terminal and try again.
    pause
    exit /b 1
)

echo Go installation found:
go version
echo.

echo Downloading dependencies...
go mod tidy
if %errorlevel% neq 0 (
    echo Failed to download dependencies
    pause
    exit /b 1
)

echo.
echo Running tests...
go test -v ./...
if %errorlevel% neq 0 (
    echo Tests failed
    pause
    exit /b 1
)

echo.
echo Building EchoSpider...
go build -ldflags "-s -w" -o echospider.exe .
if %errorlevel% neq 0 (
    echo Build failed
    pause
    exit /b 1
)

echo.
echo ================================
echo Build completed successfully!
echo ================================
echo.
echo Executable: echospider.exe
echo.
echo Usage examples:
echo   echospider.exe https://httpbin.org
echo   echospider.exe https://httpbin.org --depth 3 --workers 20
echo   echospider.exe https://httpbin.org --verbose --respect-robots
echo   echospider.exe --help
echo.
echo Quick test:
echo   echospider.exe https://httpbin.org --depth 1 --workers 5
echo.
pause

@echo off
if "%1"=="" (
    echo "usage: build.cmd <version>"
    goto end
)

set VERSION=%1
SET CGO_ENABLED=0

go build -ldflags "-X main.Version=%VERSION% -s -w" -o nettica-client.exe
if %ERRORLEVEL%==0 (
    echo "build success"
) else (
    echo "build failed"
    goto end
)

signtool.exe sign /fd sha256 /tr http://ts.ssl.com /td sha256 /n "Nettica Corporation" "nettica-client.exe"
xcopy nettica-client.exe ..\nettica-agent\extra /d /y

:end


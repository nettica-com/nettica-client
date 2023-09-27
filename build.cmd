if not exist "C:\Program Files (x86)\Windows Kits\10\bin\10.0.19041.0\x64\signtool.exe" (
    echo "signtool.exe not found"
    exit 1
)

if "%1"=="" (
    echo "usage: build.cmd <version>"
    exit 1
)

set VERSION=%1
SET CGO_ENABLED=0

if go build -ldflags "-X main.Version=%VERSION% -s -w" -o nettica-client.exe ; then
    echo "build success"
else
    echo "build failed"
    exit 1
fi

"C:\Program Files (x86)\Windows Kits\10\bin\10.0.19041.0\x64\signtool.exe"  sign /fd sha256 /tr http://ts.ssl.com /td sha256 /a "nettica-client.exe"

SET CGO_ENABLED=0
go build
"C:\Program Files (x86)\Windows Kits\10\bin\10.0.19041.0\x64\signtool.exe"  sign /fd sha256 /tr http://ts.ssl.com /td sha256 /a "nettica-client.exe"
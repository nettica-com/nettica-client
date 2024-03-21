# nettica-client

Nettica control plane for WireGuard

<img src="https://nettica.com/nettica.png" alt="A WireGuard control plane">

## Requirements

* Golang 1.21.3+
* Docker (optional)
* Windows SDK (optional - for code signing - signtool.exe)

## Build

```
go get
env CGO_ENABLED=0 go build
```

### Build for Debian Linux distributions

```
./build.sh 1.0.0 armhf
./build.sh 1.0.0 arm64
./build.sh 1.0.0 amd64
```

### Usage Linux

```
systemctl enable nettica
systemctl start nettica
systemctl stop nettica
```

### Build for Windows
There is no stand-alone installer for Windows.  The binary is installed with the Nettica Agent for Windows.  Production usage requires the code to the signed.
```
build.cmd 1.0.0
```

### Usage for Windows

```
nettica-client.exe install
nettica-client.exe start
nettica-client.exe stop
nettica-client.exe remove

net start nettica
net stop nettica
```


### Build for Fedora (RPM) Linux distributions

```
./build.sh 1.0.0 amd64
cd rpmbuild/SPECS
nano nettica.spec

# Update the version and platform as necessary

rpmbuild -bb nettica.spec
```

### Build Docker image

Nettica uses an Alpine base and is about 40MB

```
sudo docker build . --no-cache
sudo docker images
sudo docker tag xxx nettica-client:latest
```

## Notes
* Building for Linux requires additional packages be installed to build debian or RPM packages. Installing them are self-explanatory from the errors.
* Building for Linux can cross-compile for 32-bit ARM (armhf/arm7l), 64-bit ARM (arm64), and Intel/AMD X86-64 (amd64)
* Packaging for ARM64 RPM requires an aarch64 host

## Need Help

mailto:support@nettica.com

## License
* Released under MIT License

WireGuard® is a registered trademark of Jason A. Donenfeld.

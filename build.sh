#!/bin/bash
set -e

# Description: Build script for nettica-client

ARCH=$(uname -m)

if [[ ! -v VERSION ]]; then
  VERSION=development
fi


# Check arguments
if [ $# -eq 0 ]; then
    echo "Usage: build.sh version [arch] (eg. build.sh 2.0.0 amd64|arm64|armhf))"
    echo "       not specifying the arch will build all three"
    exit 1
fi

# Set version to arg 1
if [ $# -ge 1 ]; then
  VERSION=$1
fi

echo $2
if [ $# -eq 2 ]; then
  ARCH=$2
  ARCHS=($2)
fi

if [ $ARCH == 'x86_64' ]; then
  ARCH=amd64
fi

if [ $ARCH == 'aarch64' ]; then
  ARCH=arm64
fi

if [ $ARCH == 'arm/v7' ]; then
  ARCH=armhf
fi

export GOOS=linux
export CGO_ENABLED=0

if [ -z $ARCHS ]; then
  ARCHS=("armhf" "arm64" "amd64")
fi

for ARCH in ${ARCHS[@]}; do
    export GOARCH=$ARCH


    if [ $GOARCH == "armhf" ]; then
        export GOARCH=arm
        export GOARM=7
    fi


    echo "Building version $VERSION for $ARCH/$GOARCH"


    if go build -ldflags "-X main.Version=$VERSION" ; then
       echo "Build success"
    else
        echo "Build failed"
        exit 1
    fi

    cp -r nettica-client_2.X.X_${ARCH}/ nettica-client_${VERSION}_${ARCH}/
    sed -i "s/2.X.X/$VERSION/g" nettica-client_${VERSION}_${ARCH}/DEBIAN/control
    mkdir -p nettica-client_${VERSION}_${ARCH}/usr/bin/
    cp nettica-client nettica-client_${VERSION}_${ARCH}/usr/bin/nettica-client
    dpkg-deb -Zxz --build --root-owner-group nettica-client_${VERSION}_${ARCH}/
    rm -rf nettica-client_${VERSION}_${ARCH}/

    echo "Build complete"

done

exit 0


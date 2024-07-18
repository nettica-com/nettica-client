#!/bin/bash
set -e

# Description: Build script for nettica-client RPMs

# Check arguments
if [ $# -eq 0 ]; then
    echo "Usage: buildrpm.sh version arch (eg. build.sh 3.0.0 aarch64 | x86_64)"
    echo "       if arch unspecified then both are built"
    exit 1
fi

# Set version to arg 1
if [ $# -ge 1 ]; then
  VERSION=$1
fi

if [ $# -eq 1 ]; then
  ARCHS=(aarch64 x86_64)
fi

if [ $# -eq 2 ]; then
  ARCHS=($2)
fi

for ARCH in ${ARCHS[@]}; do

    if [ $ARCH == 'aarch64' ]; then
        if env GOOS=linux CGO_ENABLED=0 GOARCH=arm64 go build -ldflags "-X main.Version=$VERSION" ; then
            echo "Build success"
        else
            echo "Build failed"
            exit 1
        fi
    fi

    if [ $ARCH == 'x86_64' ]; then
        if env GOOS=linux CGO_ENABLED=0 GOARCH=amd64 go build -ldflags "-X main.Version=$VERSION" ; then
            echo "Build success"
        else
            echo "Build failed"
            exit 1
        fi
    fi


    cp -r rpmbuild/ rpmbuild-${VERSION}/
    sed -i "s/2.X.X/$VERSION/g" rpmbuild-${VERSION}/SPECS/nettica.spec
    rpmbuild --verbose --target ${ARCH} -bb rpmbuild-${VERSION}/SPECS/nettica.spec
    rm -rf rpmbuild-${VERSION}/
    mv ~/rpmbuild/RPMS/$ARCH/nettica-$VERSION-1.$ARCH.rpm .

    echo RPM built

done

exit 0

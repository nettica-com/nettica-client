# Description: Build script for nettica-client

# Check arguments
if [ $# -eq 0 ]; then
    echo "Usage: build.sh [version] [arch] (eg. build.sh 2.0.0 amd64|arm64))"
    exit 1
fi

# Set version to arg 1
export VERSION=$1
export ARCH=$2

echo "Building version $VERSION for $ARCH"

export GOOS=linux
export GOARCH=$ARCH
export CGO_ENABLED=0

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

exit 0


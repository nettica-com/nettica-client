# Check arguments
if [ $# -eq 0 ]; then
    echo "Usage: build_docker.sh [version]"
    exit 1
fi

docker buildx build --build-arg VERSION=$1 --build-arg DOCKER=true --platform linux/amd64,linux/arm64,linux/arm/v7 --provenance=true --sbom=true -t nettica/nettica-client:latest . --push --progress=plain

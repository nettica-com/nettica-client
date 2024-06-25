@echo off
if "%~1"=="" (
  echo Usage: docker_buildx.cmd VERSION [latest]
) else (
  if "%~2"=="latest" (
    docker buildx build --build-arg="VERSION=%1" --platform linux/amd64,linux/arm64,linux/arm/v7 --provenance=true --sbom=true -t nettica/nettica-client:%1 -t nettica/nettica-client:latest . --push
  ) else (
    docker buildx build --build-arg="VERSION=%1" --platform linux/amd64,linux/arm64,linux/arm/v7 --provenance=true --sbom=true -t nettica/nettica-client:%1 . --push
  )
)

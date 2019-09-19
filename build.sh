#!/bin/sh

set -x
set -e

if [ -z "$IMAGE_TAG" ]; then
    IMAGE_TAG="quay.io/rh-jmc-team/container-jfr-operator"
fi

echo "Building operator image $IMAGE_TAG to local registry..."

operator-sdk build "$IMAGE_TAG"

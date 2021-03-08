#!/bin/sh

if [ -z "$GRAFANA_IMAGE" ]; then
    GRAFANA_IMAGE="quay.io/rh-jmc-team/container-jfr-grafana-dashboard"
fi

if [ -z "$GRAFANA_TAG" ]; then
    GRAFANA_TAG="1.0.0-BETA3"
fi

if [ -z "$BUILDER" ]; then
    BUILDER="podman"
fi

$BUILDER build -t $GRAFANA_IMAGE:$GRAFANA_TAG -f "$(dirname $0)"/Containerfile
$BUILDER tag $GRAFANA_IMAGE:$GRAFANA_TAG $GRAFANA_IMAGE:latest

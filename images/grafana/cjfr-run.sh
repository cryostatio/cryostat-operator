#!/bin/bash -e

# Base image entrypoint
RUN_GRAFANA="/run.sh"

# Datasource config
DATASOURCE_YAML="/etc/grafana/provisioning/datasources/datasource.yaml"

# Substitute value of JFR_DATASOURCE_URL environment variable into datasource.yaml
sed -i "s|JFR_DATASOURCE_URL|${JFR_DATASOURCE_URL}|g" "${DATASOURCE_YAML}"

# Run base entrypoint with any additional command line args
exec "${RUN_GRAFANA}" "$@"

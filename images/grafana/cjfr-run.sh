#!/bin/bash -e

# Base image entrypoint
RUN_GRAFANA="/run.sh"

# Substitute value of JFR_DATASOURCE_URL environment variable into datasource.yaml
# and move it to the expected provisioning directory
sed -e "s|JFR_DATASOURCE_URL|${JFR_DATASOURCE_URL}|g" /tmp/datasource.yaml.in > /etc/grafana/provisioning/datasources/datasource.yaml
rm -f /tmp/datasource.yaml.in

# Run base entrypoint with any additional command line args
exec "${RUN_GRAFANA}" "$@"

#!/usr/local/env bash

set -xe

CRYOSTAT_URL="$1"

if [ -z $CRYOSTAT_URL ]; then
    echo "Cryostat URL is expected as an argument. Found none"; exit 2
fi

# Get datasource UID

DATASOURCE_UID=$(curl -kLs ${CRYOSTAT_URL}/grafana/api/datasources/name/jfr-datasource | jq .uid)

echo $DATASOURCE_UID


BODY="$(curl -ksL https://localhost:8080/grafana/api/dashboards/uid/main | jq .dashboard.panels[0].targets[0].url_options.data | sed -e 's/${__timeTo:date}/2024-08-28T20:23:35.127Z/' -e 's/${__timeFrom:date}/2024-05-01T20:23:35.127Z/')"


curl -kL --data $BODY ${CRYOSTAT_URL}/grafana/api/datasources/proxy/uid/$DATASOURCE_UID

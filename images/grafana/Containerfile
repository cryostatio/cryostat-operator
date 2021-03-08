FROM docker.io/grafana/grafana:7.2.1

EXPOSE 3000

RUN grafana-cli plugins install grafana-simple-json-datasource

COPY --chown=grafana:root \
	dashboards.yaml \
	dashboard.json \
	/etc/grafana/provisioning/dashboards

COPY --chown=grafana:root \
    datasource.yaml \
    /etc/grafana/provisioning/datasources

# Listen address of jfr-datasource
ENV JFR_DATASOURCE_URL "http://0.0.0.0:8080"

USER grafana
ENTRYPOINT [ "/run.sh" ]

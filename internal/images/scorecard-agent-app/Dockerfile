# Copyright The Cryostat Authors.
# SPDX-License-Identifier: Apache-2.0

# Use the same Agent artifact as the Operator's injection init container.
ARG AGENT_INIT_IMG=quay.io/cryostat/cryostat-agent-init:latest
FROM ${AGENT_INIT_IMG} AS agent

# Java bytecode is portable; compile on the host even for multi-architecture images.
FROM --platform=${BUILDPLATFORM} registry.access.redhat.com/ubi9/openjdk-21:latest AS builder
COPY internal/images/scorecard-agent-app/SampleApplication.java /tmp/SampleApplication.java
RUN javac -d /tmp/classes /tmp/SampleApplication.java

# This target deliberately contains no Agent: the Operator must inject it.
FROM registry.access.redhat.com/ubi9/openjdk-21-runtime:latest AS application
COPY --from=builder /tmp/classes /opt/scorecard/
EXPOSE 8080
USER 1001
ENTRYPOINT ["java", "-cp", "/opt/scorecard", "SampleApplication"]

# The bundled variant is configured explicitly by the Scorecard test.
FROM application AS bundled-agent
COPY --from=agent /cryostat/agent/cryostat-agent-shaded.jar /opt/cryostat/cryostat-agent-shaded.jar

FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends \
      ca-certificates curl gnupg && \
    install -m 0755 -d /etc/apt/keyrings && \
    curl -fsSL https://download.docker.com/linux/debian/gpg \
      -o /etc/apt/keyrings/docker.asc && \
    chmod a+r /etc/apt/keyrings/docker.asc && \
    echo "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.asc] https://download.docker.com/linux/debian bookworm stable" \
      > /etc/apt/sources.list.d/docker.list && \
    apt-get update && apt-get install -y --no-install-recommends \
      docker-ce-cli docker-compose-plugin git && \
    apt-get purge -y gnupg && apt-get autoremove -y && \
    rm -rf /var/lib/apt/lists/*

COPY simpledeploy /usr/local/bin/simpledeploy
RUN mkdir -p /etc/simpledeploy /var/lib/simpledeploy

EXPOSE 80 443 8443
ENV SIMPLEDEPLOY_HEALTH_PORT=8443
HEALTHCHECK --interval=30s --timeout=5s --start-period=15s --retries=3 \
  CMD curl -fsS http://localhost:${SIMPLEDEPLOY_HEALTH_PORT}/api/health || exit 1

ENTRYPOINT ["/usr/local/bin/simpledeploy"]
CMD ["serve", "--config", "/etc/simpledeploy/config.yaml"]

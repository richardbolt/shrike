FROM alpine:3.6

ENV PORT=${PORT:-8080} API_PORT=${API_PORT:-8475}

# Root Certificates needed for making https/ssl requests
RUN apk update && \
  apk add ca-certificates && \
  apk add --no-cache curl && \
  update-ca-certificates && \
  rm -rf /var/cache/apk/*

# Create a working directory and copy the server into it
RUN mkdir -p /usr/src/app
WORKDIR /usr/src/app
COPY bin/server /usr/src/app/

# HEALTHCHECK first runs after --interval and then every --interval afterwards.
HEALTHCHECK --interval=30s --timeout=3s \
  CMD curl --fail http://localhost:${PORT}/ping || exit 1

EXPOSE ${PORT} ${API_PORT}

CMD ["./server"]

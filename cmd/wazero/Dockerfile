# This is an example Dockerfile used to show a way to integrate wasm.
FROM golang:1.20-alpine AS build

RUN apk add --no-cache git

WORKDIR /build

# wazero doesn't publish a Docker image or binary, yet, so build on on demand.
RUN git clone --depth 1 https://github.com/tetratelabs/wazero.git \
  && (cd wazero; go build -o wazero -ldflags "-w -s" ./cmd/wazero)

# wazero has no dependencies, so it can run on scratch
FROM scratch
COPY --from=build /build/wazero/wazero /bin/wazero

ENTRYPOINT ["/bin/wazero", "run", "-cachedir=/cache", "-hostlogging=filesystem", "-mount=.:/"]

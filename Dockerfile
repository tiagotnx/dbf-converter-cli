# Dockerfile — static binary for air-gapped or container-native pipelines.
# Build:  docker build -t dbf-converter .
# Usage:  docker run --rm -v $(pwd):/data dbf-converter -i /data/in.dbf -o /data/out.csv

ARG GO_VERSION=1.25

FROM golang:${GO_VERSION}-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG VERSION=dev
ARG COMMIT=unknown
ARG DATE=unknown
RUN CGO_ENABLED=0 go build \
    -trimpath \
    -ldflags "-s -w -X main.version=${VERSION} -X main.commit=${COMMIT} -X main.date=${DATE}" \
    -o /out/dbf-converter .

FROM scratch
COPY --from=build /out/dbf-converter /dbf-converter
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
WORKDIR /data
ENTRYPOINT ["/dbf-converter"]

FROM --platform=$BUILDPLATFORM golang:1.26.3-bookworm AS build

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG VERSION=dev
ARG TARGETARCH
ENV CGO_ENABLED=0 GOOS=linux

RUN GOARCH=$TARGETARCH go build -trimpath \
    -ldflags "-s -w -X main.version=$VERSION" \
    -o /out/querysheriff-collector ./cmd/collector

# static ships tzdata, which the collector needs to load the server's log_timezone.
FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=build /out/querysheriff-collector /querysheriff-collector

# No USER: as a sidecar the collector runs as the postgres uid so it can read the jsonlogs.
ENTRYPOINT ["/querysheriff-collector"]

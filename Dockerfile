FROM golang:1.26.3-bookworm AS build

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG VERSION=dev

RUN CGO_ENABLED=0 go build -trimpath \
    -ldflags "-s -w -X main.version=$VERSION" \
    -o /out/querysheriff-collector ./cmd/collector

# Includes tzdata needed for PostgreSQL log_timezone.
FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=build /out/querysheriff-collector /querysheriff-collector

# Runs as the postgres uid when configured as a sidecar.
ENTRYPOINT ["/querysheriff-collector"]

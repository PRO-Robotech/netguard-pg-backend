FROM golang:1.24-alpine AS builder
ARG TARGETOS
ARG TARGETARCH

WORKDIR /src

# install updates and git for go get
RUN apk update --no-cache && \
    apk upgrade --no-cache && \
    apk add git

# Install goose v3.23.1 (latest version compatible with Go 1.22)
RUN mkdir -p /app/bin && \
    GOBIN=/app/bin GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH:-amd64} go install github.com/pressly/goose/v3/cmd/goose@v3.23.1

# Copy migrations from root migrations directory (complete schema)
COPY migrations/ /app/

# second release stage
FROM gcr.io/distroless/static:nonroot
WORKDIR /app/
COPY --from=builder /app/ /app/
USER 65532:65532
CMD ["/app/bin/goose"]

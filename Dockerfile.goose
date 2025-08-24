ARG GO_VER="1.22"
ARG ALPINE_VER="3.20"

# first build stage - install goose directly
FROM golang:${GO_VER}-alpine${ALPINE_VER} AS builder

WORKDIR /src

# install updates and git for go get
RUN apk update --no-cache && \
    apk upgrade --no-cache && \
    apk add git

# Install goose v3.23.1 (latest version compatible with Go 1.22)
RUN mkdir -p /app/bin && \
    GOBIN=/app/bin go install github.com/pressly/goose/v3/cmd/goose@v3.23.1

# Copy migrations from root migrations directory (complete schema)
COPY migrations/ /app/

# second release stage
FROM alpine:${ALPINE_VER}

# create user other than root and install updates
RUN addgroup -g 101 app && \
    adduser -H -u 101 -G app -s /bin/sh -D app && \
    apk update --no-cache && \
    apk upgrade --no-cache

# place all necessary executables and other files into /app directory
WORKDIR /app/
COPY --from=builder --chown=app:app /app/ /app/

# run container as new non-root user
USER app

CMD ["/app/bin/goose"]
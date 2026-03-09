ARG BUILDPLATFORM=linux/amd64

FROM --platform=$BUILDPLATFORM node:22-alpine3.23 AS frontend-build

WORKDIR /build

COPY frontend/package.json frontend/yarn.lock ./

RUN yarn install --frozen-lockfile

COPY frontend/ ./

RUN yarn build

FROM golang:1.26-alpine3.23 AS app-build

WORKDIR /build

RUN apk --no-cache add upx

COPY go.mod go.sum ./

RUN go mod download

COPY . /build

COPY --from=frontend-build /build/build/ /build/static/dist/

RUN CGO_ENABLED=0 go build -ldflags="-w -s" -o transmitter ./cmd/transmitter && \
    upx -9 --lzma transmitter && \
    chmod +x transmitter

FROM alpine:3.23.3

WORKDIR /app

RUN apk --no-cache add ca-certificates tzdata && \
    addgroup -g 10001 transmitter && \
    adduser -h /app -D -u 10001 -G transmitter transmitter && \
    chmod 700 /app && \
    chown -R transmitter: /app

COPY --from=app-build /build/transmitter /app/transmitter

RUN chown -R transmitter: /app && chmod +x /app/transmitter

USER transmitter

HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD wget -q -O- http://localhost:8080/api/health || exit 1

CMD ["/app/transmitter"]

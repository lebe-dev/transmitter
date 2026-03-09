# Stage 1: Build frontend
FROM node:22-alpine AS frontend
WORKDIR /app
COPY frontend/package.json frontend/yarn.lock ./
RUN yarn install --frozen-lockfile
COPY frontend/ ./
RUN yarn build

# Stage 2: Build Go binary with embedded frontend
FROM golang:1.26-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=frontend /app/build ./static/dist
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /transmitter ./cmd/transmitter

# Stage 3: Minimal runtime image
FROM alpine:3.23.2
RUN apk add --no-cache ca-certificates tzdata
COPY --from=builder /transmitter /usr/local/bin/transmitter
RUN addgroup -g 10001 -S app && adduser -u 10001 -S -G app app
USER app
ENTRYPOINT ["/usr/local/bin/transmitter"]

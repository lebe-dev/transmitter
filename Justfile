# --- Variables ---

version := `grep 'Version = ' cmd/transmitter/main.go | head -1 | cut -d '"' -f 2`
imageName := 'tinyops/transmitter'

# --- Utility ---
cleanup:
    rm -f transmitter
    rm -rf static/dist frontend/build

# --- Dependencies ---
bump-backend-deps:
    go get -u ./...
    go mod tidy

bump-frontend-deps:
    cd frontend && yarn upgrade

bump-deps: bump-backend-deps && bump-frontend-deps

# --- Build ---
build-frontend:
    cd frontend && yarn install --frozen-lockfile && yarn build
    mkdir -p static/dist
    cp -r frontend/build/* static/dist/

build: build-frontend && format
    go build -ldflags="-s -w" -o transmitter ./cmd/transmitter

# --- Lints ---
lint-backend: format
    go vet ./...

lint-frontend:
    cd frontend && yarn check

lint: format
    just lint-backend
    just lint-frontend

# --- Tests ---
test-backend:
    go test ./...

test name='':
    go test ./... -run '{{ name }}'

# --- Format ---
format:
    go fmt ./...

# --- Development ---
run-backend:
    go run ./cmd/transmitter

run-frontend:
    cd frontend && yarn dev

# --- Docker ---
docker-build:
    docker buildx build --platform linux/arm/v7 -t {{ imageName }}:{{ version }} .

docker-push:
    docker push {{ imageName }}:{{ version }}
    docker push {{ imageName }}:latest

release-image: docker-build && docker-push

docker-build-local:
    docker build -t {{ imageName }}:latest .

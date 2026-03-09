version := `cat cmd/transmitter/main.go | grep Version | head -1 | cut -d " " -f 4 | tr -d "\""`
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
    cd frontend && yarn install && yarn upgrade

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

lint-frontend: format
    cd frontend && yarn check

lint: format
    just lint-backend
    just lint-frontend

# --- Tests ---
test-backend:
    go test ./...

test name='':
    go test ./... -run '{{ name }}'

# --- Coverage ---
coverage:
    go test ./... -coverprofile=coverage.out
    go tool cover -func=coverage.out
    go tool cover -html=coverage.out -o coverage.html
    @echo "Coverage report generated at coverage.html"

# --- Format ---
format:
    go fmt ./...

# --- Dev Environment ---
start-env:
    docker compose -f docker-compose-dev.yml up -d

stop-env:
    docker compose -f docker-compose-dev.yml down

# --- Development ---
run-backend:
    go run ./cmd/transmitter

run-frontend:
    cd frontend && yarn dev -- --port=4200

# --- Image ---
build-image: test && lint
    docker buildx build --platform linux/arm/v7 -t {{ imageName }}:{{ version }} .

build-image-local:
    docker build -t {{ imageName }}:latest .

push-image:
    docker push {{ imageName }}:{{ version }}

release-image: build-image && push-image

release: release-image

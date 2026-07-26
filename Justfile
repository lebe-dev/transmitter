# Load variables from .env (gitignored) into recipe environments — e.g. SONAR_TOKEN.
set dotenv-load

version := `cat VERSION`
buildNumber := `git rev-parse --short HEAD`
tag := version + "-" + buildNumber
imageName := 'tinyops/transmitter'

# --- Utility ---
cleanup:
    rm -f transmitter
    rm -rf static/dist frontend/build

stop:
    lsof -ti :4200 | xargs kill -9
    lsof -ti :18080 | xargs kill -9

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
    go build -ldflags="-s -w -X main.Version={{ version }} -X main.BuildNumber={{ buildNumber }}" -o transmitter ./cmd/transmitter

# --- Lints ---
lint-backend: format
    go vet ./...
    golangci-lint run ./...

lint-frontend: format
    cd frontend && yarn run check

lint: format
    just lint-backend
    just lint-frontend

# --- SonarQube (static analysis) ---
sonarHostUrl := env_var_or_default("SONAR_HOST_URL", "http://host.docker.internal:9000")

sonar-scan:
    #!/usr/bin/env bash
    set -euo pipefail
    if [ -z "${SONAR_TOKEN:-}" ]; then
        echo "error: SONAR_TOKEN is not set." >&2
        echo "  Generate a token at {{ sonarHostUrl }} -> My Account -> Security," >&2
        echo "  then add it to .env:  SONAR_TOKEN=sqp_xxx" >&2
        exit 1
    fi
    docker run --rm \
        --add-host=host.docker.internal:host-gateway \
        -e SONAR_HOST_URL="{{ sonarHostUrl }}" \
        -e SONAR_TOKEN="$SONAR_TOKEN" \
        -v "$PWD:/usr/src" \
        sonarsource/sonar-scanner-cli:latest \
        -Dsonar.projectVersion="{{ version }}"

# --- Tests ---
test-backend:
    go test ./...

test-frontend:
    cd frontend && yarn test

# Run all tests (backend + frontend), or a focused backend test: just test name=Foo
test name='':
    #!/usr/bin/env bash
    set -euo pipefail
    if [ -z "{{ name }}" ]; then
        just test-backend
        just test-frontend
    else
        go test ./... -run '{{ name }}'
    fi

# --- Coverage ---
coverage:
    go test ./... -coverprofile=coverage.out
    go tool cover -func=coverage.out
    go tool cover -html=coverage.out -o coverage.html
    @echo "Backend coverage report generated at coverage.html"
    cd frontend && yarn test:coverage

# --- Format ---
format:
    gofmt -s -w .
    goimports -w .

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
    docker buildx build --platform linux/arm/v7,linux/amd64 --driver docker-container -t {{ imageName }}:{{ tag }} .
    # docker buildx build --platform linux/arm/v8 -t {{ imageName }}:{{ tag }} .
    # docker buildx build --platform linux/amd64 -t {{ imageName }}:{{ tag }} .

build-image-local:
    docker build -t {{ imageName }}:latest .

push-image:
    docker push {{ imageName }}:{{ tag }}

release-image:
    docker buildx inspect multibuilder > /dev/null 2>&1 || docker buildx create --name multibuilder --driver docker-container
    docker buildx use multibuilder
    docker buildx inspect --bootstrap
    docker buildx build --platform linux/amd64,linux/arm/v7,linux/arm64/v8 -t {{ imageName }}:{{ tag }} --push .

release: release-image

deploy:
    ssh -t rpi "cd /opt/transmitter && sudo sed -i -E 's#(image: {{ imageName }}:).*#\1{{ tag }}#' docker-compose.yml && sudo docker compose pull && sudo docker compose down && sudo docker compose up -d"

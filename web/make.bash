#!/usr/bin/env bash
set -e
cd "$(dirname "$0")"

# Build the Docker image
echo "Building web assets with Docker..."
docker build -q -t pat-web-builder .

# Create named volumes for caching
docker volume create pat-web-node-modules >/dev/null 2>&1 || true
docker volume create pat-web-nvm-cache >/dev/null 2>&1 || true

# Run the container with volume mounts for caching and bind mount for source
if [[ "$1" == "dev" ]]; then
    echo "Starting dev server on port 8081..."
    docker run --rm -it \
        -v "$(pwd):/app" \
        -v pat-web-node-modules:/app/node_modules \
        -v pat-web-nvm-cache:/home/node/.nvm \
        -w /app \
        -p 8081:8081 \
        pat-web-builder \
        "nvm install && nvm use && npm install && npx webpack-dev-server --mode=development --port=8081 --host 0.0.0.0"
else
    echo "Running npm build..."
    docker run --rm \
        -v "$(pwd):/app" \
        -v pat-web-node-modules:/app/node_modules \
        -v pat-web-nvm-cache:/home/node/.nvm \
        -w /app \
        pat-web-builder \
        "nvm install && nvm use && npm ci && npm run production"
fi

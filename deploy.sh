#!/bin/bash

set -e

echo "🚀 Building and deploying Waystone Web application..."

# Check if .env exists; if not, create it from .env.example
if [ ! -f .env ]; then
    if [ -f .env.example ]; then
        echo "📋 Creating .env file from .env.example..."
        cp .env.example .env
        echo "⚠️  Please edit .env and set your GOOGLE_CLIENT_ID and GOOGLE_CLIENT_SECRET"
        echo "   You can continue deployment without these for development mode."
    else
        echo "⚠️  Warning: Neither .env nor .env.example found"
        echo "   Environment variables will use system defaults"
    fi
fi

# Load .env file if it exists
if [ -f .env ]; then
    echo "📥 Loading environment variables from .env..."
    set -a
    source .env
    set +a
fi

# Display loaded configuration (without secrets)
if [ ! -z "$GOOGLE_CLIENT_ID" ]; then
    echo "✓ GOOGLE_CLIENT_ID is set"
else
    echo "⚠️  GOOGLE_CLIENT_ID not set (OAuth will not work)"
fi

if [ ! -z "$GOOGLE_CLIENT_SECRET" ]; then
    echo "✓ GOOGLE_CLIENT_SECRET is set"
else
    echo "⚠️  GOOGLE_CLIENT_SECRET not set (OAuth will not work)"
fi

echo "✓ PORT=${PORT:-8080}"
echo "✓ OAUTH_CALLBACK_URL=${OAUTH_CALLBACK_URL:-http://localhost:8080/auth/callback}"

# Check if Docker is running
if ! docker info > /dev/null 2>&1; then
    echo "❌ Docker is not running. Please start Docker and try again."
    exit 1
fi

# Build the Docker image
echo ""
echo "📦 Building Docker image..."
docker-compose build

# Stop any existing container
echo "🛑 Stopping any existing containers..."
docker-compose down 2>/dev/null || true

# Start the container
echo "▶️  Starting container..."
docker-compose up -d

# Wait for container to be ready
echo "⏳ Waiting for application to be ready..."
sleep 2

# Check if container is running
if docker-compose ps | grep -q "Up"; then
    echo "✅ Deployment successful!"
    echo "🌐 Application is running at http://localhost:8080"
    echo ""
    echo "Available commands:"
    echo "  docker-compose logs -f          # View logs"
    echo "  docker-compose stop             # Stop the application"
    echo "  docker-compose down             # Stop and remove containers"
    echo "  docker-compose down -v          # Stop and remove containers + data volume"
else
    echo "❌ Failed to start container"
    docker-compose logs
    exit 1
fi

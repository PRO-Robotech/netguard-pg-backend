#!/bin/bash

# Exit on any error
set -e

echo "=== Testing Docker setup for netguard-pg-backend ==="

# Build the Docker image
echo "Building Docker image..."
docker build -t netguard-pg-backend .

# Run the container in detached mode
echo "Starting container..."
CONTAINER_ID=$(docker run -d -p 8080:8080 -p 9090:9090 netguard-pg-backend)

# Wait for the service to start
echo "Waiting for service to start..."
sleep 5

# Check if the service is accessible
echo "Checking if service is accessible..."
if curl -s http://localhost:8080/swagger/ | grep -q "Swagger UI"; then
    echo "✅ Swagger UI is accessible"
else
    echo "❌ Failed to access Swagger UI"
    docker logs $CONTAINER_ID
    docker stop $CONTAINER_ID
    exit 1
fi

# Check if the API is accessible
echo "Checking if API is accessible..."
if curl -s http://localhost:8080/v1/sync/status | grep -q "updated_at"; then
    echo "✅ API is accessible"
else
    echo "❌ Failed to access API"
    docker logs $CONTAINER_ID
    docker stop $CONTAINER_ID
    exit 1
fi

# Stop the container
echo "Stopping container..."
docker stop $CONTAINER_ID

echo "=== Docker setup test completed successfully ==="
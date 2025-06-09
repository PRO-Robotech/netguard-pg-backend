#!/bin/bash

# Create the swagger-ui directory if it doesn't exist
mkdir -p swagger-ui

# Copy the Swagger JSON file to the swagger-ui directory
cp protos/pkg/api/netguard/api.swagger.json swagger-ui/api.swagger.json

echo "Swagger JSON file copied to swagger-ui directory"
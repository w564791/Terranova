#!/bin/bash

# Script to generate Swagger documentation for IAC Platform
# This script will regenerate the Swagger docs after annotations are added

set -e

echo "🔄 Generating Swagger documentation..."

cd backend

# Install swag if not already installed
if ! command -v swag &> /dev/null; then
    echo "📦 Installing swag..."
    go install github.com/swaggo/swag/cmd/swag@latest
fi

# Generate Swagger docs
echo "📝 Running swag init..."
swag init -g main.go --output docs --parseDependency --parseInternal

echo " Swagger documentation generated successfully!"
echo "📄 Documentation files:"
echo "   - backend/docs/docs.go"
echo "   - backend/docs/swagger.json"
echo "   - backend/docs/swagger.yaml"
echo ""
echo "🌐 Access Swagger UI at: http://localhost:8080/swagger/index.html"

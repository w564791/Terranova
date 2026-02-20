#!/bin/bash

# Agent WebSocket Server Startup Script
# This script starts a standalone WebSocket server for Agent C&C connections
# to avoid Gin framework interference with WebSocket connections

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo -e "${GREEN}Starting Agent WebSocket Server...${NC}"

# Check if .env file exists
if [ ! -f "../backend/.env" ]; then
    echo -e "${RED}Error: .env file not found in backend directory${NC}"
    exit 1
fi

# Load environment variables
export $(cat ../backend/.env | grep -v '^#' | xargs)

# Set default values if not in .env
export AGENT_WS_PORT=${AGENT_WS_PORT:-8091}
export DB_HOST=${DB_HOST:-localhost}
export DB_PORT=${DB_PORT:-5432}
export DB_USER=${DB_USER:-postgres}
export DB_NAME=${DB_NAME:-iac_platform}
export DB_SSLMODE=${DB_SSLMODE:-disable}

echo -e "${YELLOW}Configuration:${NC}"
echo "  WebSocket Port: $AGENT_WS_PORT"
echo "  Database: $DB_HOST:$DB_PORT/$DB_NAME"
echo ""

# Navigate to backend directory
cd ../backend

# Create temporary main.go for WebSocket server
cat > /tmp/agent_ws_main.go << 'EOF'
package main

import (
	"crypto/sha256"
	"encoding/base64"
	"log"
	"net/http"
	"os"
	"strings"

	"iac-platform/internal/database"
	"iac-platform/internal/handlers"
	"iac-platform/internal/models"

	"gorm.io/gorm"
)

func main() {
	// Initialize database
	cfg := database.Config{
		Host:     os.Getenv("DB_HOST"),
		Port:     os.Getenv("DB_PORT"),
		User:     os.Getenv("DB_USER"),
		Password: os.Getenv("DB_PASSWORD"),
		DBName:   os.Getenv("DB_NAME"),
		SSLMode:  os.Getenv("DB_SSLMODE"),
	}

	db, err := database.Initialize(cfg)
	if err != nil {
		log.Fatal("Failed to initialize database:", err)
	}

	// Get WebSocket port
	wsPort := os.Getenv("AGENT_WS_PORT")
	if wsPort == "" {
		wsPort = "8091"
	}

	// Create raw WebSocket handler
	rawHandler := handlers.NewRawAgentCCHandler(db)

	// Create HTTP server with authentication
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/agents/control", func(w http.ResponseWriter, r *http.Request) {
		// Check authorization header
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		// Extract token
		token := strings.TrimPrefix(authHeader, "Bearer ")
		if token == authHeader {
			http.Error(w, "Invalid authorization format", http.StatusUnauthorized)
			return
		}

		// Authenticate token
		if !authenticatePoolToken(db, token) {
			http.Error(w, "Invalid token", http.StatusUnauthorized)
			return
		}

		// Pass to raw handler
		rawHandler.ServeHTTP(w, r)
	})

	log.Printf("[AgentWS] Starting WebSocket server on port %s", wsPort)
	log.Printf("[AgentWS] WebSocket endpoint: ws://localhost:%s/api/v1/agents/control", wsPort)
	if err := http.ListenAndServe(":"+wsPort, mux); err != nil {
		log.Fatalf("[AgentWS] Failed to start: %v", err)
	}
}

func authenticatePoolToken(db *gorm.DB, token string) bool {
	if !strings.HasPrefix(token, "apt_") {
		return false
	}

	hash := sha256.Sum256([]byte(token))
	tokenHash := base64.StdEncoding.EncodeToString(hash[:])

	var poolToken models.PoolToken
	err := db.Where("token_hash = ? AND is_active = true", tokenHash).
		First(&poolToken).Error

	if err != nil {
		return false
	}

	if poolToken.ExpiresAt != nil && poolToken.ExpiresAt.Before(db.NowFunc()) {
		return false
	}

	db.Model(&poolToken).Update("last_used_at", db.NowFunc())
	return true
}
EOF

echo -e "${GREEN}Starting WebSocket server on port $AGENT_WS_PORT...${NC}"
go run /tmp/agent_ws_main.go

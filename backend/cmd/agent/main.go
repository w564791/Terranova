package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"iac-platform/agent/control"
	"iac-platform/services"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	log.Printf("IAC Agent v3.2 starting...")

	// 1. Read environment variables
	apiEndpoint := os.Getenv("IAC_API_ENDPOINT")
	agentToken := os.Getenv("IAC_AGENT_TOKEN")
	agentName := os.Getenv("IAC_AGENT_NAME")
	protocol := os.Getenv("IAC_AGENT_PROTOCOL")

	// Validate required variables
	if apiEndpoint == "" || agentToken == "" || agentName == "" {
		log.Fatal("Required environment variables not set: IAC_API_ENDPOINT, IAC_AGENT_TOKEN, IAC_AGENT_NAME")
	}

	// Default protocol to http if not specified
	if protocol == "" {
		protocol = "http"
	}

	// Validate protocol
	if protocol != "http" && protocol != "https" {
		log.Fatalf("Invalid IAC_AGENT_PROTOCOL: %s (must be 'http' or 'https')", protocol)
	}

	// Get API server port (default: 8080)
	serverPort := "8080"
	if port := os.Getenv("SERVER_PORT"); port != "" {
		serverPort = port
	}

	log.Printf("Configuration:")
	log.Printf("  - API Endpoint: %s", apiEndpoint)
	log.Printf("  - Server Port: %s", serverPort)
	log.Printf("  - Protocol: %s", protocol)
	log.Printf("  - Agent Name: %s", agentName)

	// 2. Create API client with full URL
	fullAPIURL := fmt.Sprintf("%s://%s:%s", protocol, apiEndpoint, serverPort)
	apiClient := services.NewAgentAPIClient(fullAPIURL, agentToken)
	log.Printf("API client created with base URL: %s", fullAPIURL)

	// 3. Register agent with retry logic
	log.Printf("Registering agent (with exponential backoff: 2s, 4s, 8s, 16s, then 60s)...")
	var agentID, poolID string
	var err error

	backoff := 2 * time.Second
	maxBackoff := 60 * time.Second
	attempt := 0

	for {
		attempt++
		log.Printf("Registration attempt #%d", attempt)

		agentID, poolID, err = apiClient.Register(agentName)
		if err == nil {
			log.Printf("Agent registered successfully:")
			log.Printf("  - Agent ID: %s", agentID)
			log.Printf("  - Pool ID: %s", poolID)
			break
		}

		log.Printf("Registration attempt #%d failed: %v", attempt, err)
		log.Printf("Retrying in %v...", backoff)

		time.Sleep(backoff)

		// Exponential backoff: 2s -> 4s -> 8s -> 16s -> 60s (capped)
		backoff *= 2
		if backoff > maxBackoff {
			backoff = maxBackoff
		}
	}

	// 4. Fetch and generate HCP credentials file
	log.Printf("Fetching HCP credentials for pool %s...", poolID)
	if err := fetchAndGenerateHCPCredentials(apiClient); err != nil {
		log.Printf("Warning: Failed to generate HCP credentials: %v", err)
		log.Printf("Agent will continue without HCP credentials")
	}

	// 5. Create data accessor (Remote mode)
	dataAccessor := services.NewRemoteDataAccessor(apiClient)
	log.Printf("Remote data accessor created")

	// 6. Create stream manager
	streamManager := services.NewOutputStreamManager()
	log.Printf("Stream manager created")

	// 6. Create executor (using RemoteDataAccessor)
	executor := services.NewTerraformExecutorWithAccessor(dataAccessor, streamManager)
	log.Printf("Terraform executor created (Agent mode)")

	// 7. Create C&C manager with protocol
	ccManager := control.NewCCManager(apiClient, executor, streamManager, protocol)
	ccManager.AgentID = agentID
	ccManager.PoolID = poolID
	log.Printf("C&C manager created")

	// 8. Connect to C&C channel with retry logic
	log.Printf("Connecting to C&C channel (with exponential backoff: 2s, 4s, 8s, 16s, then 60s)...")
	if err := ccManager.Connect(); err != nil {
		log.Printf("Failed to establish initial connection: %v", err)
		log.Printf("Note: Connection will continue retrying in the background")
	} else {
		log.Printf("Connected to C&C channel successfully")
	}

	// 9. Start heartbeat loop
	log.Printf("Starting heartbeat loop...")
	go ccManager.HeartbeatLoop()

	// 10. Start HCP credentials refresh loop
	log.Printf("Starting HCP credentials refresh loop (every 5 minutes)...")
	go startCredentialsRefreshLoop(apiClient)

	// 11. Agent is now ready
	log.Printf("========================================")
	log.Printf("IAC Agent is ready and waiting for tasks")
	log.Printf("========================================")

	// 11. Wait for shutdown signal
	ccManager.WaitForShutdown()

	log.Printf("Agent stopped")
}

// fetchAndGenerateHCPCredentials fetches HCP secrets from the server and generates credentials.tfrc.json
func fetchAndGenerateHCPCredentials(apiClient *services.AgentAPIClient) error {
	log.Printf("[HCP Credentials] Fetching pool secrets from server...")

	// Call the new API endpoint to get pool secrets
	respBody, err := apiClient.GetPoolSecrets()
	if err != nil {
		return fmt.Errorf("failed to fetch pool secrets: %w", err)
	}

	// Extract credentials from response
	credentials, ok := respBody["credentials"].(map[string]interface{})
	if !ok {
		return fmt.Errorf("invalid response format: credentials field missing or wrong type")
	}

	// If no credentials, skip file generation
	if len(credentials) == 0 {
		log.Printf("[HCP Credentials] No HCP secrets found for this pool, skipping credentials file generation")
		return nil
	}

	log.Printf("[HCP Credentials] Found %d HCP credential(s)", len(credentials))

	// Get home directory
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}

	// Create ~/.terraform.d directory
	terraformDir := filepath.Join(homeDir, ".terraform.d")
	if err := os.MkdirAll(terraformDir, 0700); err != nil {
		return fmt.Errorf("failed to create .terraform.d directory: %w", err)
	}

	// Prepare credentials structure for Terraform
	credentialsFile := map[string]interface{}{
		"credentials": credentials,
	}

	// Write credentials.tfrc.json
	credentialsPath := filepath.Join(terraformDir, "credentials.tfrc.json")
	credentialsJSON, err := json.MarshalIndent(credentialsFile, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal credentials: %w", err)
	}

	// Write with restricted permissions (only owner can read/write)
	if err := os.WriteFile(credentialsPath, credentialsJSON, 0600); err != nil {
		return fmt.Errorf("failed to write credentials file: %w", err)
	}

	log.Printf("[HCP Credentials] Successfully generated credentials file at %s", credentialsPath)
	log.Printf("[HCP Credentials] File contains %d credential(s)", len(credentials))

	return nil
}

// startCredentialsRefreshLoop starts a background loop to periodically refresh HCP credentials
func startCredentialsRefreshLoop(apiClient *services.AgentAPIClient) {
	// Refresh interval: 5 minutes
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	log.Printf("[HCP Credentials Refresh] Background refresh loop started (interval: 5 minutes)")

	for range ticker.C {
		log.Printf("[HCP Credentials Refresh] Starting periodic refresh...")

		if err := fetchAndGenerateHCPCredentials(apiClient); err != nil {
			log.Printf("[HCP Credentials Refresh] Warning: Failed to refresh credentials: %v", err)
		} else {
			log.Printf("[HCP Credentials Refresh] Credentials refreshed successfully")
		}
	}
}

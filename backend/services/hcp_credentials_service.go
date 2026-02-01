package services

import (
	"encoding/json"
	"fmt"
	"iac-platform/internal/crypto"
	"iac-platform/internal/models"
	"log"
	"os"
	"path/filepath"

	"gorm.io/gorm"
)

// HCPCredentialsService handles HCP credentials file generation for agents
type HCPCredentialsService struct {
	db *gorm.DB
}

// NewHCPCredentialsService creates a new HCP credentials service
func NewHCPCredentialsService(db *gorm.DB) *HCPCredentialsService {
	return &HCPCredentialsService{
		db: db,
	}
}

// HCPCredentials represents the structure of credentials.tfrc.json
type HCPCredentials struct {
	Credentials map[string]HCPToken `json:"credentials"`
}

// HCPToken represents a single HCP token entry
type HCPToken struct {
	Token string `json:"token"`
}

// GenerateCredentialsFile generates ~/.terraform.d/credentials.tfrc.json for the agent
// Returns true if file was generated, false if no HCP secrets exist
func (s *HCPCredentialsService) GenerateCredentialsFile(poolID string) (bool, error) {
	log.Printf("[HCP Credentials] Starting credentials file generation for pool %s", poolID)

	// 1. Query HCP secrets for this agent pool
	var secrets []models.Secret
	err := s.db.Where("resource_type = ? AND resource_id = ? AND secret_type = ? AND is_active = ?",
		models.ResourceTypeAgentPool, poolID, models.SecretTypeHCP, true).
		Find(&secrets).Error

	if err != nil {
		return false, fmt.Errorf("failed to query HCP secrets: %w", err)
	}

	// 2. If no HCP secrets, don't generate file
	if len(secrets) == 0 {
		log.Printf("[HCP Credentials] No HCP secrets found for pool %s, skipping file generation", poolID)
		return false, nil
	}

	log.Printf("[HCP Credentials] Found %d HCP secrets for pool %s", len(secrets), poolID)

	// 3. Decrypt secrets and build credentials structure
	credentials := HCPCredentials{
		Credentials: make(map[string]HCPToken),
	}

	for _, secret := range secrets {
		// Parse metadata to get the key
		var metadata models.SecretMetadata
		if err := json.Unmarshal(secret.Metadata, &metadata); err != nil {
			log.Printf("[HCP Credentials] Warning: failed to parse metadata for secret %s: %v", secret.SecretID, err)
			continue
		}

		// Decrypt the value
		decryptedValue, err := crypto.DecryptValue(secret.ValueHash)
		if err != nil {
			log.Printf("[HCP Credentials] Warning: failed to decrypt secret %s: %v", secret.SecretID, err)
			continue
		}

		// Add to credentials map
		// The key should be the HCP hostname (e.g., "app.terraform.io")
		credentials.Credentials[metadata.Key] = HCPToken{
			Token: decryptedValue,
		}

		log.Printf("[HCP Credentials] Added credential for key: %s", metadata.Key)
	}

	// 4. If no valid credentials after decryption, don't generate file
	if len(credentials.Credentials) == 0 {
		log.Printf("[HCP Credentials] No valid credentials after decryption, skipping file generation")
		return false, nil
	}

	// 5. Determine home directory
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return false, fmt.Errorf("failed to get home directory: %w", err)
	}

	// 6. Create ~/.terraform.d directory if it doesn't exist
	terraformDir := filepath.Join(homeDir, ".terraform.d")
	if err := os.MkdirAll(terraformDir, 0700); err != nil {
		return false, fmt.Errorf("failed to create .terraform.d directory: %w", err)
	}

	// 7. Write credentials.tfrc.json
	credentialsPath := filepath.Join(terraformDir, "credentials.tfrc.json")
	credentialsJSON, err := json.MarshalIndent(credentials, "", "  ")
	if err != nil {
		return false, fmt.Errorf("failed to marshal credentials: %w", err)
	}

	// Write with restricted permissions (only owner can read/write)
	if err := os.WriteFile(credentialsPath, credentialsJSON, 0600); err != nil {
		return false, fmt.Errorf("failed to write credentials file: %w", err)
	}

	log.Printf("[HCP Credentials] Successfully generated credentials file at %s with %d entries",
		credentialsPath, len(credentials.Credentials))

	return true, nil
}

// RemoveCredentialsFile removes the credentials file if it exists
func (s *HCPCredentialsService) RemoveCredentialsFile() error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}

	credentialsPath := filepath.Join(homeDir, ".terraform.d", "credentials.tfrc.json")

	// Check if file exists
	if _, err := os.Stat(credentialsPath); os.IsNotExist(err) {
		log.Printf("[HCP Credentials] Credentials file does not exist, nothing to remove")
		return nil
	}

	// Remove the file
	if err := os.Remove(credentialsPath); err != nil {
		return fmt.Errorf("failed to remove credentials file: %w", err)
	}

	log.Printf("[HCP Credentials] Successfully removed credentials file at %s", credentialsPath)
	return nil
}

// RefreshCredentialsFile regenerates the credentials file
// This should be called when secrets are added/updated/deleted
func (s *HCPCredentialsService) RefreshCredentialsFile(poolID string) error {
	log.Printf("[HCP Credentials] Refreshing credentials file for pool %s", poolID)

	// Remove existing file first
	if err := s.RemoveCredentialsFile(); err != nil {
		log.Printf("[HCP Credentials] Warning: failed to remove existing credentials file: %v", err)
	}

	// Generate new file
	generated, err := s.GenerateCredentialsFile(poolID)
	if err != nil {
		return fmt.Errorf("failed to generate credentials file: %w", err)
	}

	if !generated {
		log.Printf("[HCP Credentials] No HCP secrets found, credentials file not generated")
	}

	return nil
}

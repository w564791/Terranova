package services

import (
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"os"
	"sync"

	"iac-platform/internal/models"

	"gorm.io/gorm"
)

// PlatformConfig holds platform configuration
type PlatformConfig struct {
	BaseURL  string // 完整的基础 URL，如 http://localhost:8080
	Protocol string // 协议：http 或 https
	Host     string // 主机地址
	APIPort  string // API 端口
	CCPort   string // Agent CC 端口
}

// PlatformConfigService provides platform configuration
type PlatformConfigService struct {
	db     *gorm.DB
	cache  *PlatformConfig
	mu     sync.RWMutex
	loaded bool
}

// NewPlatformConfigService creates a new platform config service
func NewPlatformConfigService(db *gorm.DB) *PlatformConfigService {
	return &PlatformConfigService{
		db: db,
	}
}

// GetConfig returns the platform configuration
// It always reads from database to ensure latest values are used
// IMPORTANT: If base_url is set, it takes precedence and host/protocol/api_port are parsed from it
// This ensures consistency even if the database has mismatched values
func (s *PlatformConfigService) GetConfig() (*PlatformConfig, error) {
	config := &PlatformConfig{}

	// Try to load from database
	if s.db != nil {
		// Load platform_base_url first (it takes precedence)
		var baseURLConfig models.SystemConfig
		if err := s.db.Where("key = ?", "platform_base_url").First(&baseURLConfig).Error; err == nil {
			config.BaseURL = s.parseJSONString(baseURLConfig.Value)
		}

		// Load platform_protocol
		var protocolConfig models.SystemConfig
		if err := s.db.Where("key = ?", "platform_protocol").First(&protocolConfig).Error; err == nil {
			config.Protocol = s.parseJSONString(protocolConfig.Value)
		}

		// Load platform_host
		var hostConfig models.SystemConfig
		if err := s.db.Where("key = ?", "platform_host").First(&hostConfig).Error; err == nil {
			config.Host = s.parseJSONString(hostConfig.Value)
		}

		// Load platform_api_port
		var apiPortConfig models.SystemConfig
		if err := s.db.Where("key = ?", "platform_api_port").First(&apiPortConfig).Error; err == nil {
			config.APIPort = s.parseJSONString(apiPortConfig.Value)
		}

		// Load platform_cc_port
		var ccPortConfig models.SystemConfig
		if err := s.db.Where("key = ?", "platform_cc_port").First(&ccPortConfig).Error; err == nil {
			config.CCPort = s.parseJSONString(ccPortConfig.Value)
		}
	}

	// Fall back to environment variables if not set in database
	if config.BaseURL == "" {
		config.BaseURL = os.Getenv("BASE_URL")
	}
	if config.Protocol == "" {
		config.Protocol = os.Getenv("PLATFORM_PROTOCOL")
	}
	if config.Host == "" {
		config.Host = os.Getenv("PLATFORM_HOST")
	}
	if config.APIPort == "" {
		config.APIPort = os.Getenv("PLATFORM_API_PORT")
	}
	if config.CCPort == "" {
		config.CCPort = os.Getenv("PLATFORM_CC_PORT")
	}

	// IMPORTANT: If base_url is set, parse host/protocol/api_port from it
	// This ensures consistency and fixes any mismatched values in the database
	if config.BaseURL != "" {
		parsedURL, err := url.Parse(config.BaseURL)
		if err == nil {
			// Override protocol from base_url
			if parsedURL.Scheme != "" {
				config.Protocol = parsedURL.Scheme
			}
			// Override host from base_url
			if parsedURL.Hostname() != "" {
				config.Host = parsedURL.Hostname()
			}
			// Override api_port from base_url
			if parsedURL.Port() != "" {
				config.APIPort = parsedURL.Port()
			} else if parsedURL.Scheme == "https" {
				config.APIPort = "443"
			} else if parsedURL.Scheme == "http" {
				config.APIPort = "80"
			}
			log.Printf("[PlatformConfig] Parsed from base_url: protocol=%s, host=%s, api_port=%s",
				config.Protocol, config.Host, config.APIPort)
		} else {
			log.Printf("[PlatformConfig] Warning: failed to parse base_url '%s': %v", config.BaseURL, err)
		}
	}

	// Set defaults for any remaining empty values
	if config.Protocol == "" {
		config.Protocol = "http"
	}
	if config.Host == "" {
		config.Host = "localhost"
	}
	if config.APIPort == "" {
		config.APIPort = "8080"
	}
	if config.CCPort == "" {
		config.CCPort = "8081"
	}

	// Build BaseURL if not set
	if config.BaseURL == "" {
		config.BaseURL = fmt.Sprintf("%s://%s:%s", config.Protocol, config.Host, config.APIPort)
	}

	return config, nil
}

// GetBaseURL returns the platform base URL
func (s *PlatformConfigService) GetBaseURL() string {
	config, err := s.GetConfig()
	if err != nil || config == nil {
		return "http://localhost:8080"
	}
	return config.BaseURL
}

// GetCCURL returns the Agent CC URL
func (s *PlatformConfigService) GetCCURL() string {
	config, err := s.GetConfig()
	if err != nil || config == nil {
		return "http://localhost:8081"
	}
	return fmt.Sprintf("%s://%s:%s", config.Protocol, config.Host, config.CCPort)
}

// RefreshConfig forces a reload of the configuration
func (s *PlatformConfigService) RefreshConfig() error {
	s.mu.Lock()
	s.loaded = false
	s.cache = nil
	s.mu.Unlock()

	_, err := s.GetConfig()
	return err
}

// parseJSONString extracts a string value from JSON
// The value in system_configs is stored as JSONB, so strings are quoted
func (s *PlatformConfigService) parseJSONString(value interface{}) string {
	if value == nil {
		return ""
	}

	// If it's already a string, try to unmarshal it as JSON string
	if str, ok := value.(string); ok {
		var result string
		if err := json.Unmarshal([]byte(str), &result); err == nil {
			return result
		}
		return str
	}

	// If it's a byte slice
	if bytes, ok := value.([]byte); ok {
		var result string
		if err := json.Unmarshal(bytes, &result); err == nil {
			return result
		}
		return string(bytes)
	}

	return fmt.Sprintf("%v", value)
}

package auth

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"
)

// Config holds authentication configuration
type Config struct {
	Enabled    bool     `mapstructure:"enabled"`
	APIKeys    []string `mapstructure:"api_keys"`
	BearerAuth bool     `mapstructure:"bearer_auth"`
	BasicAuth  bool     `mapstructure:"basic_auth"`
}

// AuthMiddleware provides API key authentication for LogSieve
type AuthMiddleware struct {
	config Config
	logger zerolog.Logger
	apiKeys map[string]bool
}

// NewAuthMiddleware creates a new authentication middleware
func NewAuthMiddleware(config Config, logger zerolog.Logger) *AuthMiddleware {
	apiKeys := make(map[string]bool)
	for _, key := range config.APIKeys {
		apiKeys[key] = true
	}

	return &AuthMiddleware{
		config:  config,
		logger:  logger.With().Str("component", "auth").Logger(),
		apiKeys: apiKeys,
	}
}

// Middleware returns a Gin middleware function for authentication
func (am *AuthMiddleware) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Skip authentication if disabled
		if !am.config.Enabled {
			c.Next()
			return
		}

		// Try different authentication methods
		authenticated := false

		// 1. API Key in header (X-API-Key)
		if apiKey := c.GetHeader("X-API-Key"); apiKey != "" {
			if am.validateAPIKey(apiKey) {
				authenticated = true
				am.logger.Debug().Str("method", "api_key").Msg("Authenticated request")
			}
		}

		// 2. Bearer token
		if !authenticated && am.config.BearerAuth {
			authHeader := c.GetHeader("Authorization")
			if strings.HasPrefix(authHeader, "Bearer ") {
				token := strings.TrimPrefix(authHeader, "Bearer ")
				if am.validateAPIKey(token) {
					authenticated = true
					am.logger.Debug().Str("method", "bearer").Msg("Authenticated request")
				}
			}
		}

		// 3. Query parameter (for legacy support, not recommended for production)
		if !authenticated {
			if apiKey := c.Query("api_key"); apiKey != "" {
				if am.validateAPIKey(apiKey) {
					authenticated = true
					am.logger.Debug().Str("method", "query_param").Msg("Authenticated request")
				}
			}
		}

		if !authenticated {
			am.logger.Warn().
				Str("client_ip", c.ClientIP()).
				Str("path", c.Request.URL.Path).
				Msg("Unauthorized access attempt")

			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "Unauthorized",
				"message": "Valid API key required. Provide via X-API-Key header, Bearer token, or api_key query parameter",
			})
			c.Abort()
			return
		}

		c.Next()
	}
}

// validateAPIKey checks if the provided API key is valid
func (am *AuthMiddleware) validateAPIKey(key string) bool {
	if key == "" {
		return false
	}

	return am.apiKeys[key]
}

// AddAPIKey adds a new API key to the allowed list
func (am *AuthMiddleware) AddAPIKey(key string) {
	am.apiKeys[key] = true
	am.logger.Info().Msg("Added new API key")
}

// RemoveAPIKey removes an API key from the allowed list
func (am *AuthMiddleware) RemoveAPIKey(key string) {
	delete(am.apiKeys, key)
	am.logger.Info().Msg("Removed API key")
}

// GetKeyCount returns the number of configured API keys
func (am *AuthMiddleware) GetKeyCount() int {
	return len(am.apiKeys)
}

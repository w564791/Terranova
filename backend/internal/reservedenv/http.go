package reservedenv

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// Refuse writes 422 reserved_env_var when any of keys is reserved.
// details.key / details.reserved_prefix carry the first hit; details.reserved
// lists all hits. Returns true when the request was refused.
func Refuse(c *gin.Context, keys ...string) bool {
	hits := FindHits(keys)
	if len(hits) == 0 {
		return false
	}
	first := hits[0]
	c.JSON(http.StatusUnprocessableEntity, gin.H{
		"error": "environment variable key is reserved by the platform",
		"code":  ErrorCode,
		"details": gin.H{
			"key":             first.Key,
			"reserved_prefix": first.ReservedPrefix,
			"reserved":        hits,
		},
	})
	return true
}

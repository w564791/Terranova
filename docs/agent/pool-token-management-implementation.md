# Agent Pool Token Management Implementation

## Overview
This document describes the implementation of the Agent Pool Token Management system, which supports both static tokens for traditional agent pools and temporary tokens for K8s-based agent pools.

## Implementation Date
October 29, 2025

## Architecture

### Token Types

#### 1. Static Tokens
- **Purpose**: Long-lived tokens for static agent pools
- **Lifecycle**: Created manually, revoked manually
- **Use Case**: Traditional agent deployments where agents run continuously
- **Features**:
  - One token can register multiple agents
  - Optional expiration time
  - Manual revocation required
  - Token format: `apt_{pool_id}_{64-char-hex}`

#### 2. K8s Temporary Tokens
- **Purpose**: Short-lived tokens for K8s Job-based agents
- **Lifecycle**: Created automatically with K8s Job, auto-revoked after completion
- **Use Case**: Dynamic agent provisioning via Kubernetes Jobs
- **Features**:
  - One token per K8s Job/Pod
  - Automatic expiration
  - Automatic cleanup
  - Token format: `apt_{pool_id}_{64-char-hex}`

## Database Schema

### Table: `pool_tokens`

```sql
CREATE TABLE pool_tokens (
    token_hash VARCHAR(64) PRIMARY KEY,           -- SHA-256 hash of token
    token_name VARCHAR(100) NOT NULL,             -- Human-readable name
    token_type VARCHAR(20) NOT NULL,              -- 'static' or 'k8s_temporary'
    pool_id VARCHAR(50) NOT NULL,                 -- Reference to agent_pools
    is_active BOOLEAN DEFAULT true NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP NOT NULL,
    created_by VARCHAR(50),
    revoked_at TIMESTAMP,
    revoked_by VARCHAR(50),
    last_used_at TIMESTAMP,
    expires_at TIMESTAMP,
    k8s_job_name VARCHAR(255),                    -- K8s Job name (for k8s_temporary)
    k8s_pod_name VARCHAR(255),                    -- K8s Pod name (for k8s_temporary)
    k8s_namespace VARCHAR(100) DEFAULT 'terraform',
    
    FOREIGN KEY (pool_id) REFERENCES agent_pools(pool_id) ON DELETE CASCADE
);
```

### Indexes
- `idx_pool_tokens_pool_id` - Query tokens by pool
- `idx_pool_tokens_type` - Filter by token type
- `idx_pool_tokens_active` - Filter active tokens
- `idx_pool_tokens_expires_at` - Cleanup expired tokens
- `idx_pool_tokens_k8s_job` - Query by K8s job name

## Backend Implementation

### Files Created/Modified

1. **`backend/internal/models/pool_token.go`**
   - PoolToken model
   - Request/Response models
   - K8sJobTemplateConfig model

2. **`backend/internal/application/service/pool_token_service.go`**
   - GenerateStaticToken()
   - GenerateK8sTemporaryToken()
   - ListPoolTokens()
   - RevokeToken()
   - ValidateToken()
   - CleanupExpiredTokens()
   - UpdateK8sConfig()
   - GetK8sConfig()

3. **`backend/internal/handlers/agent_pool_handler.go`**
   - CreatePoolToken()
   - ListPoolTokens()
   - RevokePoolToken()
   - UpdateK8sConfig()
   - GetK8sConfig()

4. **`backend/internal/router/router_agent.go`**
   - Added token management routes
   - Added K8s config routes

### API Endpoints

#### Token Management

```
POST   /api/v1/agent-pools/:pool_id/tokens
GET    /api/v1/agent-pools/:pool_id/tokens
DELETE /api/v1/agent-pools/:pool_id/tokens/:token_name
```

#### K8s Configuration

```
PUT    /api/v1/agent-pools/:pool_id/k8s-config
GET    /api/v1/agent-pools/:pool_id/k8s-config
```

### Security Features

1. **Token Storage**
   - Only SHA-256 hash stored in database
   - Plaintext token returned only once at creation
   - No way to retrieve plaintext token after creation

2. **Token Validation**
   - Hash-based lookup
   - Active status check
   - Expiration check
   - Last used timestamp tracking

3. **Authorization**
   - IAM permission checks: `AGENT_POOLS` resource
   - Admin bypass available
   - Organization-level permissions

## Token Format

### Static Token
```
apt_pool-abc123def456_1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef
```

### Components
- Prefix: `apt_` (Agent Pool Token)
- Pool ID: `pool-abc123def456`
- Random: 64 hex characters (32 bytes)

## K8s Integration

### Job Template Configuration

The K8s configuration is stored in the `agent_pools.k8s_config` JSONB field:

```json
{
  "image": "terraform-agent:latest",
  "image_pull_policy": "Always",
  "command": ["/bin/sh"],
  "args": ["-c", "terraform-agent run"],
  "env": {
    "TF_LOG": "INFO"
  },
  "resources": {
    "requests": {
      "cpu": "500m",
      "memory": "512Mi"
    },
    "limits": {
      "cpu": "1000m",
      "memory": "1Gi"
    }
  },
  "restart_policy": "Never",
  "backoff_limit": 3,
  "ttl_seconds_after_finished": 3600
}
```

### K8s Job Creation Flow

1. User triggers job creation via API
2. System generates temporary token (expires in 1 hour by default)
3. System creates K8s Job with:
   - Token injected as environment variable
   - Job name and pod name recorded
   - Namespace fixed to "terraform"
4. Agent in pod uses token to register
5. After job completion:
   - Token auto-expires
   - Cleanup service removes expired tokens

## Token Cleanup Mechanism

### Automatic Cleanup

A background service should periodically call:

```go
poolTokenService.CleanupExpiredTokens(ctx)
```

Recommended schedule: Every 5 minutes

### Manual Cleanup

Tokens can be manually revoked via:
- API: `DELETE /api/v1/agent-pools/:pool_id/tokens/:token_name`
- Service: `poolTokenService.RevokeToken(ctx, poolID, tokenName, revokedBy)`

## Usage Examples

### Creating a Static Token

```bash
curl -X POST http://localhost:8080/api/v1/agent-pools/pool-abc123/tokens \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "token_name": "production-agent-token",
    "expires_at": "2026-12-31T23:59:59Z"
  }'
```

Response:
```json
{
  "token": "apt_pool-abc123_1234567890abcdef...",
  "token_name": "production-agent-token",
  "token_type": "static",
  "pool_id": "pool-abc123",
  "created_at": "2025-10-29T17:00:00Z",
  "created_by": "user-123",
  "expires_at": "2026-12-31T23:59:59Z"
}
```

### Listing Pool Tokens

```bash
curl -X GET http://localhost:8080/api/v1/agent-pools/pool-abc123/tokens \
  -H "Authorization: Bearer $TOKEN"
```

Response:
```json
{
  "tokens": [
    {
      "token_name": "production-agent-token",
      "token_type": "static",
      "pool_id": "pool-abc123",
      "is_active": true,
      "created_at": "2025-10-29T17:00:00Z",
      "created_by": "user-123",
      "last_used_at": "2025-10-29T17:30:00Z",
      "expires_at": "2026-12-31T23:59:59Z"
    }
  ],
  "total": 1
}
```

### Revoking a Token

```bash
curl -X DELETE http://localhost:8080/api/v1/agent-pools/pool-abc123/tokens/production-agent-token \
  -H "Authorization: Bearer $TOKEN"
```

### Updating K8s Configuration

```bash
curl -X PUT http://localhost:8080/api/v1/agent-pools/pool-k8s123/k8s-config \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "k8s_config": {
      "image": "terraform-agent:v1.2.3",
      "image_pull_policy": "Always",
      "resources": {
        "requests": {
          "cpu": "500m",
          "memory": "512Mi"
        }
      }
    }
  }'
```

## Migration Steps

1. **Run Database Migration**
   ```bash
   psql -U postgres -d iac_platform -f scripts/create_pool_tokens_table.sql
   ```

2. **Restart Backend Service**
   ```bash
   cd backend
   go build
   ./iac-platform
   ```

3. **Verify APIs**
   - Test token creation
   - Test token listing
   - Test token revocation
   - Test K8s config management

## Frontend Integration (TODO)

### AgentPoolDetail Page Updates Needed

1. **Token Management Section**
   - Display list of tokens
   - Create new token button
   - Revoke token button
   - Show token details (name, type, status, created, expires)
   - Copy token to clipboard (only shown once at creation)

2. **K8s Configuration Section** (for K8s pools)
   - Form to edit K8s Job template
   - JSON editor or structured form
   - Save configuration button
   - Validation for required fields

3. **UI Components**
   - Token creation dialog
   - Token display with copy button
   - Confirmation dialog for revocation
   - K8s config editor

## Testing Checklist

- [ ] Database migration successful
- [ ] Create static token
- [ ] List tokens
- [ ] Revoke token
- [ ] Token validation
- [ ] Expired token cleanup
- [ ] K8s config update
- [ ] K8s config retrieval
- [ ] Permission checks
- [ ] Error handling
- [ ] Frontend integration

## Security Considerations

1. **Token Storage**: Never log or display full tokens except at creation
2. **Token Transmission**: Always use HTTPS in production
3. **Token Rotation**: Implement regular token rotation policy
4. **Access Control**: Enforce IAM permissions strictly
5. **Audit Logging**: Log all token operations

## Future Enhancements

1. **Token Rotation**: Automatic token rotation for static tokens
2. **Token Scopes**: Fine-grained permissions per token
3. **Rate Limiting**: Limit token usage per time period
4. **Token Analytics**: Usage statistics and monitoring
5. **K8s Job Monitoring**: Real-time job status tracking
6. **Multi-namespace Support**: Support for multiple K8s namespaces

## Related Documentation

- [Agent Architecture](../10-agent-architecture.md)
- [Pool Authorization Migration](../iam/pool-authorization-migration-complete.md)
- [K8s Implementation](../workspace/02-agent-k8s-implementation.md)

## Status

**Backend Implementation**:  Complete
**Database Schema**:  Complete
**API Routes**:  Complete
**Frontend UI**: ⏳ Pending
**Testing**: ⏳ Pending
**Documentation**:  Complete

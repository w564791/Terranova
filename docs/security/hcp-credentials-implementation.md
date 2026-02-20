# HCP Credentials File Generation for Agents

## Overview

This document describes the implementation of automatic HCP (HashiCorp Cloud Platform) credentials file generation for IAC Platform agents. When HCP secrets are added to an agent pool, the system automatically generates a `~/.terraform.d/credentials.tfrc.json` file on the agent machine, enabling Terraform to authenticate with HCP services.

## Architecture

### Components

1. **HCPCredentialsService** (`backend/services/hcp_credentials_service.go`)
   - Core service responsible for generating and managing credentials files
   - Queries HCP secrets from the database
   - Decrypts secret values
   - Writes credentials to `~/.terraform.d/credentials.tfrc.json`

2. **Agent Registration Integration** (`backend/internal/handlers/agent_handler.go`)
   - Automatically generates credentials file when agent registers
   - Returns `hcp_credentials_generated` flag in registration response

3. **Secret Management Integration** (`backend/internal/handlers/secret_handler.go`)
   - Triggers credentials refresh when HCP secrets are created
   - Triggers credentials refresh when HCP secrets are updated
   - Triggers credentials refresh when HCP secrets are deleted
   - Removes credentials file if no HCP secrets remain

## File Format

The generated `credentials.tfrc.json` file follows the Terraform CLI configuration format:

```json
{
  "credentials": {
    "app.terraform.io": {
      "token": "your-hcp-token-here"
    },
    "custom.terraform.io": {
      "token": "another-token-here"
    }
  }
}
```

### Key Structure

- **Key**: The HCP hostname (e.g., `app.terraform.io`, custom Terraform Enterprise hostname)
- **Value**: An object containing the authentication token

## Workflow

### 1. Agent Registration and Initial Setup

```
1. Agent calls /api/v1/agents/register with pool token
2. Server creates agent record in database
3. Agent calls /api/v1/agents/pool/secrets to fetch HCP secrets
4. Agent generates ~/.terraform.d/credentials.tfrc.json locally
5. Agent starts background refresh loop (every 5 minutes)
6. Agent is ready to execute tasks
```

**For Local Mode:**
- Server generates credentials file during registration
- File is written to server's filesystem

**For Remote/K8s Mode:**
- Agent fetches secrets via API after registration
- Agent generates credentials file on its own filesystem
- Background loop keeps credentials up-to-date

### 2. Secret Creation

```
1. User creates HCP secret via /api/v1/agent_pool/{poolId}/secrets
2. Secret is encrypted and stored in database
3. If resource_type is agent_pool and secret_type is hcp:
   - Trigger async credentials refresh
   - HCPCredentialsService.RefreshCredentialsFile(poolID)
4. Credentials file is regenerated with new secret
```

### 3. Secret Update

```
1. User updates HCP secret via /api/v1/agent_pool/{poolId}/secrets/{secretId}
2. Secret value is re-encrypted and updated in database
3. If resource_type is agent_pool and secret_type is hcp:
   - Trigger async credentials refresh
4. Credentials file is regenerated with updated secret
```

### 4. Secret Deletion

```
1. User deletes HCP secret via /api/v1/agent_pool/{poolId}/secrets/{secretId}
2. Secret is removed from database
3. If resource_type is agent_pool:
   - Trigger async credentials refresh
   - If no HCP secrets remain, credentials file is removed
4. Credentials file is either regenerated or removed
```

## Security Considerations

### File Permissions

- Credentials file is created with `0600` permissions (read/write for owner only)
- Directory `~/.terraform.d` is created with `0700` permissions

### Encryption

- Secret values are encrypted in the database using AES-256-GCM
- Values are only decrypted when generating the credentials file
- Decrypted values are never logged or exposed in API responses

### Access Control

- Only agents with valid pool tokens can trigger credentials generation
- Secrets are scoped to specific agent pools
- Users must have appropriate permissions to manage secrets

## Supported Agent Modes

### 1. Local Agent Mode

- Credentials file is written to the local filesystem
- Path: `~/.terraform.d/credentials.tfrc.json`
- Used when agent runs on the same machine as the IAC Platform server

### 2. Remote Agent Mode

- Credentials file is written to the remote agent's filesystem
- Agent must have write access to `~/.terraform.d` directory
- Credentials are generated during agent registration

### 3. Kubernetes Agent Mode

- Credentials file is written to the pod's filesystem
- File persists for the lifetime of the pod
- New pods receive fresh credentials during initialization
- Can be combined with Kubernetes secrets for additional security

## API Endpoints

### Agent Registration
```
POST /api/v1/agents/register
Authorization: Bearer {pool_token}

Response:
{
  "agent_id": "agent-pool-xxx-123456789",
  "pool_id": "pool-xxx",
  "status": "online",
  "registered_at": "2025-01-04T17:00:00Z",
  "hcp_credentials_generated": true
}
```

### Get Pool Secrets (Agent-side)
```
GET /api/v1/agents/pool/secrets
Authorization: Bearer {pool_token}

Response:
{
  "credentials": {
    "app.terraform.io": {
      "token": "your-hcp-token-here"
    },
    "custom.terraform.io": {
      "token": "another-token-here"
    }
  }
}
```

**Usage**: Remote and K8s agents call this endpoint to retrieve HCP secrets and generate their local `credentials.tfrc.json` file.

### Create Secret
```
POST /api/v1/agent_pool/{poolId}/secrets
Content-Type: application/json

{
  "key": "app.terraform.io",
  "value": "your-hcp-token",
  "secret_type": "hcp",
  "description": "HCP Terraform Cloud token"
}
```

## Error Handling

### Credentials Generation Failures

- If credentials generation fails during agent registration, the agent is still registered
- Error is logged but does not block registration
- Agent can still function without HCP credentials (for non-HCP workspaces)

### Refresh Failures

- Credentials refresh is performed asynchronously
- Failures are logged but do not affect the API response
- Users should monitor logs for refresh failures

## Credentials Refresh Mechanism

### Automatic Refresh

The system provides multiple mechanisms to keep credentials up-to-date:

1. **Server-side Refresh (Local Mode)**
   - Triggered when secrets are created/updated/deleted
   - Asynchronous refresh via goroutine
   - Immediate update on secret changes

2. **C&C Push Notification (Remote/K8s Mode)** ⭐ **Real-time**
   - Server sends `refresh_credentials` command via C&C WebSocket
   - Agent receives command and immediately fetches latest secrets
   - Updates credentials file within seconds
   - No polling delay - instant propagation

3. **Agent-side Polling (Backup Mechanism)**
   - Background loop runs every 5 minutes
   - Calls `/api/v1/agents/pool/secrets` to fetch latest secrets
   - Serves as fallback if C&C notification fails
   - Ensures eventual consistency

### Refresh Flow

```
User modifies secret
    ↓
Server saves to database
    ↓
┌─────────────────────┬──────────────────────┐
│   Local Mode        │  Remote/K8s Mode     │
├─────────────────────┼──────────────────────┤
│ Direct file refresh │ C&C push (instant)   │
│                     │ + Polling (5min)     │
└─────────────────────┴──────────────────────┘
    ↓
Credentials updated
```

### C&C Message Format

**Server → Agent:**
```json
{
  "type": "refresh_credentials",
  "payload": {
    "pool_id": "pool-xxx",
    "timestamp": 1704384000
  }
}
```

**Agent → Server (Acknowledgment):**
```json
{
  "type": "credentials_refreshed",
  "payload": {
    "agent_id": "agent-pool-xxx-123",
    "pool_id": "pool-xxx",
    "timestamp": 1704384001,
    "count": 2
  }
}
```

### Refresh Latency

| Mechanism | Latency | Reliability |
|-----------|---------|-------------|
| C&C Push | < 1 second | High (if agent connected) |
| Polling | Up to 5 minutes | Very High (always works) |
| Combined | < 1 second (with 5min fallback) | Excellent |

## Monitoring and Logging

### Log Messages

```
[HCP Credentials] Starting credentials file generation for pool {poolId}
[HCP Credentials] Found {count} HCP secrets for pool {poolId}
[HCP Credentials] Added credential for key: {hostname}
[HCP Credentials] Successfully generated credentials file at {path} with {count} entries
[HCP Credentials] No HCP secrets found for pool {poolId}, skipping file generation
[HCP Credentials] Failed to refresh credentials after secret creation: {error}
[HCP Credentials Refresh] Background refresh loop started (interval: 5 minutes)
[HCP Credentials Refresh] Starting periodic refresh...
[HCP Credentials Refresh] Credentials refreshed successfully
```

### Metrics to Monitor

- Number of credentials files generated
- Number of HCP secrets per pool
- Credentials refresh success/failure rate
- File generation latency

## Troubleshooting

### Credentials File Not Generated

1. Check if HCP secrets exist for the agent pool:
   ```sql
   SELECT * FROM secrets 
   WHERE resource_type = 'agent_pool' 
   AND resource_id = 'pool-xxx' 
   AND secret_type = 'hcp' 
   AND is_active = true;
   ```

2. Check agent registration logs for errors

3. Verify agent has write permissions to `~/.terraform.d`

### Terraform Not Using Credentials

1. Verify credentials file exists: `ls -la ~/.terraform.d/credentials.tfrc.json`

2. Check file permissions: Should be `0600`

3. Verify file format is valid JSON

4. Check Terraform is looking in the correct location

### Credentials Not Refreshing

1. Check secret handler logs for refresh trigger

2. Verify HCPCredentialsService is initialized in SecretHandler

3. Check for async goroutine errors in logs

## Future Enhancements

### Planned Features

1. **Credentials Rotation**
   - Automatic rotation based on expiry dates
   - Notification before expiration

2. **Multiple Secret Types**
   - Support for AWS credentials
   - Support for Azure credentials
   - Support for GCP credentials

3. **Credentials Validation**
   - Test credentials before writing file
   - Validate token format

4. **Audit Logging**
   - Track when credentials are accessed
   - Log credential usage by workspace

## Testing

### Manual Testing Steps

1. Create an agent pool
2. Add HCP secret to the pool:
   ```bash
   curl -X POST http://localhost:8080/api/v1/agent_pool/pool-xxx/secrets \
     -H "Authorization: Bearer {token}" \
     -H "Content-Type: application/json" \
     -d '{
       "key": "app.terraform.io",
       "value": "your-token",
       "secret_type": "hcp"
     }'
   ```
3. Register an agent with the pool token
4. Verify credentials file exists: `cat ~/.terraform.d/credentials.tfrc.json`
5. Run a Terraform workspace that uses HCP
6. Verify Terraform can authenticate

### Automated Testing

- Unit tests for HCPCredentialsService
- Integration tests for agent registration flow
- End-to-end tests with actual Terraform execution

## References

- [Terraform CLI Configuration](https://www.terraform.io/docs/cli/config/config-file.html)
- [HCP Terraform Authentication](https://www.terraform.io/docs/cloud/registry/using.html#authentication)
- [IAC Platform Secrets Design](./universal-secrets-storage-design.md)

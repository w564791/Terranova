# Provider Global Templates Design

## Background

The current provider settings are workspace-scoped only, stored in `workspace.provider_config` as a JSONB field. This design has two problems:

1. **No reusability**: Each workspace independently configures its own providers. Common configurations (e.g., shared AWS credentials, standard K8s cluster endpoints) must be duplicated across workspaces.
2. **Mandatory provider**: Task creation fails with HTTP 400 when `provider_config` is null/empty (`workspace_task_controller.go:135-141`). This is unreasonable because many Terraform modules already include provider configuration, or providers can authenticate via environment variables.

## Goals

- Create a global provider template system (similar to IaC Engine Versions pattern)
- Allow workspaces to reference global templates with optional field-level overrides
- Remove the mandatory provider requirement for task execution
- Support arbitrary provider types (not just AWS/Azure/Google)
- Maintain backward compatibility with existing workspace provider configurations

## Data Model

### New Table: `provider_templates`

```go
type ProviderTemplate struct {
    ID          uint      `gorm:"primaryKey"`
    Name        string    `gorm:"not null;uniqueIndex"` // "K8s Production", "TencentCloud GZ"
    Type        string    `gorm:"not null"`              // "aws", "kubernetes", "tencentcloud", "ode"
    Source      string    `gorm:"not null"`              // "hashicorp/aws", "hashicorp/kubernetes", "IBM/ode"
    Config      JSONB     `gorm:"type:jsonb;not null"`   // provider block content (auth, endpoints, etc.)
    Version     string    `gorm:"type:varchar(50)"`      // "6.0", "2.35.0" (optional)
    Constraint  string    `gorm:"type:varchar(10)"`      // "~>", ">=", "=" (optional)
    IsDefault   bool      `gorm:"default:false"`         // one default per type
    Enabled     bool      `gorm:"default:true"`
    Description string
    CreatedBy   *uint
    CreatedAt   time.Time
    UpdatedAt   time.Time
}
```

**Config field stores only the `provider "<type>" {}` block content**, not required_providers. Example for Kubernetes:

```json
{
  "host": "https://k8s-api.example.com:6443",
  "cluster_ca_certificate": "...",
  "client_certificate": "...",
  "client_key": "..."
}
```

Example for TencentCloud:

```json
{
  "secret_id": "AKIDxxxxxxxxxxxx",
  "secret_key": "xxxxxxxxxxxxxxxx",
  "region": "ap-guangzhou"
}
```

Example for ODE (IBM):

```json
{
  "ode_host": "https://your-ode-hostname:port",
  "ode_username": "your-ode-user",
  "ode_password": "your-ode-password",
  "ode_tls": {
    "ca_file": "file(\"/path/to/ca_file\")",
    "server_name": "your-ode-server-name"
  }
}
```

### Workspace Model Changes

```go
type Workspace struct {
    // NEW: referenced global template IDs
    ProviderTemplateIDs JSONB  `json:"provider_template_ids" gorm:"type:jsonb"`
    // NEW: workspace-level overrides per provider type
    ProviderOverrides   JSONB  `json:"provider_overrides" gorm:"type:jsonb"`
    // KEPT: final resolved provider config (cached at save time)
    ProviderConfig      JSONB  `json:"provider_config" gorm:"type:jsonb"`
    // KEPT: hash for terraform init -upgrade optimization
    ProviderConfigHash  string `json:"provider_config_hash" gorm:"type:varchar(64)"`
}
```

**ProviderOverrides format** (only overridden fields are merged):

```json
{
  "aws": {
    "region": "ap-southeast-1"
  },
  "kubernetes": {
    "host": "https://staging-k8s.example.com:6443"
  }
}
```

## API Design

### Global Provider Template APIs

Routes under `/api/v1/global/settings/` (following IaC Engine pattern):

| Method | Path | Permission | Description |
|--------|------|------------|-------------|
| GET | `/provider-templates` | PROVIDER_TEMPLATES:ORGANIZATION:READ | List all templates |
| POST | `/provider-templates` | PROVIDER_TEMPLATES:ORGANIZATION:WRITE | Create template |
| GET | `/provider-templates/:id` | PROVIDER_TEMPLATES:ORGANIZATION:READ | Get template detail |
| PUT | `/provider-templates/:id` | PROVIDER_TEMPLATES:ORGANIZATION:WRITE | Update template |
| DELETE | `/provider-templates/:id` | PROVIDER_TEMPLATES:ORGANIZATION:ADMIN | Delete (check usage) |
| POST | `/provider-templates/:id/set-default` | PROVIDER_TEMPLATES:ORGANIZATION:ADMIN | Set as default for type |

### Workspace API Changes

Existing `PATCH /workspaces/:id` extended with new fields:

```json
{
  "provider_template_ids": [1, 2],
  "provider_overrides": {
    "aws": { "region": "ap-southeast-1" }
  }
}
```

### New: Resolved Provider Config API

```
GET /api/v1/workspaces/:id/resolved-provider-config
```

Returns the merged final `provider.tf.json` content for preview purposes.

## Merge Logic

### Configuration Resolution Priority (low to high)

1. Global template default configuration
2. Workspace `provider_overrides` (shallow merge per provider type)

### provider.tf.json Generation Flow

```
Workspace has provider_template_ids?
├── Yes → Load templates + merge overrides → Generate provider.tf.json
├── No → Workspace has legacy provider_config?
│   ├── Yes → Use legacy config to generate provider.tf.json (backward compat)
│   └── No → Skip provider.tf.json generation (let Terraform handle it)
```

### Merge Algorithm

```go
func resolveProviderConfig(templates []ProviderTemplate, overrides map[string]interface{}) map[string]interface{} {
    result := map[string]interface{}{
        "provider": map[string]interface{}{},
    }
    requiredProviders := map[string]interface{}{}

    for _, tmpl := range templates {
        // Build provider block
        config := deepCopy(tmpl.Config)
        if override, ok := overrides[tmpl.Type]; ok {
            // Shallow merge: override fields replace template fields
            for k, v := range override.(map[string]interface{}) {
                config[k] = v
            }
        }
        result["provider"].(map[string]interface{})[tmpl.Type] = []interface{}{config}

        // Build required_providers
        if tmpl.Source != "" {
            rp := map[string]interface{}{"source": tmpl.Source}
            if tmpl.Version != "" && tmpl.Constraint != "" {
                rp["version"] = tmpl.Constraint + " " + tmpl.Version
            }
            requiredProviders[tmpl.Type] = rp
        }
    }

    if len(requiredProviders) > 0 {
        result["terraform"] = []interface{}{
            map[string]interface{}{"required_providers": []interface{}{requiredProviders}},
        }
    }

    return result
}
```

## Backend Changes

### Files to Modify

1. **`backend/internal/models/provider_template.go`** (NEW)
   - ProviderTemplate model definition
   - Database migration

2. **`backend/internal/models/workspace.go`**
   - Add `ProviderTemplateIDs` and `ProviderOverrides` fields

3. **`backend/services/provider_template_service.go`** (NEW)
   - CRUD operations for provider templates
   - Set default (atomic transaction, same as TerraformVersionService)
   - Check usage before deletion
   - Resolve config (merge templates + overrides)

4. **`backend/controllers/provider_template_controller.go`** (NEW)
   - HTTP handlers for template CRUD

5. **`backend/internal/router/router_global.go`**
   - Register new routes under `/global/settings/provider-templates`

6. **`backend/controllers/workspace_task_controller.go`**
   - Remove lines 135-141 (mandatory provider check)

7. **`backend/services/terraform_executor.go`**
   - Skip `provider.tf.json` generation when no provider config exists
   - Handle nil `SnapshotProviderConfig` as valid
   - Add template resolution logic before generating provider.tf.json

8. **`backend/services/provider_service.go`**
   - Remove hardcoded type validation (aws/azure/google/alicloud)
   - Add generic validation (config not empty, source format correct)
   - Support arbitrary provider types

9. **`backend/controllers/workspace_controller.go`**
   - Handle new fields in workspace update
   - Resolve and cache provider_config when template_ids or overrides change

### Snapshot Changes

When creating a task snapshot, the resolved `provider_config` (after merge) is captured in `SnapshotProviderConfig`. This ensures reproducibility even if global templates change later.

## Frontend Changes

### Admin Page: Provider Templates Management

Add a new section to the Admin page (reference: IaC Engine Versions section in `Admin.tsx`):

- Table: Name, Type, Source, Version, Default badge, Enabled toggle
- Create/Edit dialog: Provider type (free text input), name, source, version constraint, config (key-value editor or JSON editor)
- Set Default button (per type)
- Delete with usage check

### Workspace Settings: Provider Tab Redesign

Replace the current `ProviderSettings.tsx` with a simplified interface:

1. **Template Selection**: Multi-select dropdown showing available global templates, grouped by type
2. **Override Section**: For each selected template, show a collapsible panel with the template's fields, allowing users to modify values
3. **Preview**: Show the resolved `provider.tf.json` that will be generated
4. **Custom Mode**: Toggle to fall back to the legacy full-config editor for backward compatibility
5. **None Mode**: Option to explicitly configure no provider

### Files to Modify

1. **`frontend/src/pages/Admin.tsx`** — Add Provider Templates management section
2. **`frontend/src/pages/ProviderSettings.tsx`** — Redesign to template selection + override mode
3. **`frontend/src/pages/WorkspaceSettings.tsx`** — Update tab integration if needed
4. **`frontend/src/services/admin.ts`** — Add provider template API methods

## Data Migration

- **No forced migration**: Existing `workspace.provider_config` data continues to work as-is
- **Fallback logic**: New code checks `provider_template_ids` first, then falls back to `provider_config`
- **Admin opt-in**: Admins can manually create global templates from commonly used configurations
- **Gradual adoption**: Workspaces can switch to template references at their own pace

## Security Considerations

- Global templates may contain sensitive credentials (access keys, passwords)
- Apply the same `FilterSensitiveInfo` logic to template API responses
- Extend sensitive field detection beyond AWS-specific fields to support arbitrary providers
- Consider configurable sensitive field patterns per provider type

## Testing Strategy

- Unit tests for merge logic (template + overrides → final config)
- Unit tests for provider_template_service CRUD
- Integration tests for task execution with: templates only, templates + overrides, legacy config, no config
- Frontend tests for template selection and override UI

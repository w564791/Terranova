# Module Demo Management Feature

## Overview

This document describes the Module Demo Management feature, which allows users to create, manage, and version demonstration configurations for Terraform modules.

## Database Schema

### Tables

#### module_demos
Main table for storing demo configurations.

| Column | Type | Description |
|--------|------|-------------|
| id | SERIAL | Primary key |
| module_id | INTEGER | Foreign key to modules table |
| name | VARCHAR(200) | Demo name |
| description | TEXT | Demo description |
| current_version_id | INTEGER | Foreign key to current version |
| is_active | BOOLEAN | Soft delete flag |
| usage_notes | TEXT | Usage instructions |
| created_by | INTEGER | User who created the demo |
| created_at | TIMESTAMP | Creation timestamp |
| updated_at | TIMESTAMP | Last update timestamp |

#### module_demo_versions
Version history for demo configurations.

| Column | Type | Description |
|--------|------|-------------|
| id | SERIAL | Primary key |
| demo_id | INTEGER | Foreign key to module_demos |
| version | INTEGER | Version number (auto-increment) |
| is_latest | BOOLEAN | Flag for latest version |
| config_data | JSONB | Configuration data (form values) |
| change_summary | TEXT | Summary of changes |
| change_type | VARCHAR(20) | Type: create, update, rollback |
| diff_from_previous | TEXT | JSON diff from previous version |
| created_by | INTEGER | User who created this version |
| created_at | TIMESTAMP | Creation timestamp |

### Indexes

- `idx_module_demos_module`: Index on module_id
- `idx_module_demos_active`: Index on is_active
- `idx_module_demo_versions_demo`: Index on demo_id
- `idx_module_demo_versions_latest`: Index on is_latest
- `idx_module_demo_versions_version`: Composite index on (demo_id, version)

## Backend API

### Endpoints

#### Demo CRUD Operations

**GET /api/v1/modules/:moduleId/demos**
- Get all demos for a module
- Returns: Array of demo objects with current version

**POST /api/v1/modules/:moduleId/demos**
- Create a new demo
- Body: `{ name, description, usage_notes, config_data }`
- Automatically creates version 1

**GET /api/v1/demos/:id**
- Get demo details
- Returns: Demo object with current version and module info

**PUT /api/v1/demos/:id**
- Update demo (creates new version)
- Body: `{ name, description, usage_notes, config_data, change_summary }`
- Automatically increments version

**DELETE /api/v1/demos/:id**
- Soft delete demo (sets is_active = false)

#### Version Management

**GET /api/v1/demos/:id/versions**
- Get all versions for a demo
- Returns: Array of versions ordered by version DESC

**GET /api/v1/demo-versions/:versionId**
- Get specific version details
- Returns: Version object with creator info

**GET /api/v1/demos/:id/compare?version1=X&version2=Y**
- Compare two versions
- Returns: `{ version1, version2, diff, has_changes }`

**POST /api/v1/demos/:id/rollback**
- Rollback to a specific version
- Body: `{ version_id }`
- Creates a new version with rollback type

## Frontend Implementation

### Module Detail Page Enhancement

Add a new "Demos" tab to the module detail page (`/modules/:id`).

### Components

#### 1. DemoList Component
- Display all demos for the module
- Show demo name, description, version, last updated
- Actions: View, Edit, Delete
- "Create Demo" button

#### 2. DemoForm Component
- Form fields:
  - Name (required)
  - Description
  - Usage Notes
  - Config Data (dynamic form based on module schema)
  - Change Summary (for updates)
- Validation
- Submit/Cancel buttons

#### 3. DemoVersionHistory Component
- List all versions
- Show version number, change type, summary, timestamp, creator
- Actions: View, Compare, Rollback
- Visual timeline

#### 4. VersionCompare Component
- Side-by-side comparison
- Highlight differences
- JSON diff visualization
- "Rollback to this version" button

### User Workflows

#### Create Demo
1. Click "Create Demo" button
2. Fill in demo form with configuration
3. Submit → Creates demo with version 1
4. Redirect to demo detail view

#### Edit Demo
1. Click "Edit" on a demo
2. Form pre-filled with current version data
3. Modify configuration
4. Add change summary
5. Submit → Creates new version
6. Updates current_version_id

#### View Version History
1. Click "History" on a demo
2. See timeline of all versions
3. Click version to view details
4. Compare any two versions
5. Rollback to previous version if needed

#### Compare Versions
1. Select two versions from history
2. View side-by-side comparison
3. See highlighted differences
4. Option to rollback to older version

#### Rollback
1. From version history or compare view
2. Click "Rollback to this version"
3. Confirm action
4. Creates new version with rollback type
5. Updates current version

## Version Control Features

### Automatic Versioning
- Every update creates a new version
- Version numbers auto-increment
- Only one version marked as `is_latest`
- Previous versions preserved

### Change Tracking
- `change_type`: create, update, rollback
- `change_summary`: User-provided description
- `diff_from_previous`: Automatic JSON diff

### Rollback Safety
- Rollback creates new version (doesn't delete history)
- Preserves complete audit trail
- Can rollback multiple times

## UI/UX Design

### Demo List View
```
┌─────────────────────────────────────────────────────┐
│ Demos                                  [Create Demo]│
├─────────────────────────────────────────────────────┤
│ ┌─────────────────────────────────────────────────┐ │
│ │ Production Config                    v3 • 2d ago │ │
│ │ Standard production configuration               │ │
│ │ [View] [Edit] [History] [Delete]                │ │
│ └─────────────────────────────────────────────────┘ │
│ ┌─────────────────────────────────────────────────┐ │
│ │ Development Config                   v2 • 5d ago │ │
│ │ Development environment setup                   │ │
│ │ [View] [Edit] [History] [Delete]                │ │
│ └─────────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────────┘
```

### Version History View
```
┌─────────────────────────────────────────────────────┐
│ Version History: Production Config                  │
├─────────────────────────────────────────────────────┤
│ ● v3 (Current) • 2 days ago • John Doe              │
│   Updated bucket encryption settings                │
│   [View] [Compare]                                  │
│                                                     │
│ ○ v2 • 1 week ago • Jane Smith                      │
│   Added lifecycle rules                             │
│   [View] [Compare] [Rollback]                       │
│                                                     │
│ ○ v1 • 2 weeks ago • John Doe                       │
│   Initial version                                   │
│   [View] [Compare] [Rollback]                       │
└─────────────────────────────────────────────────────┘
```

### Version Compare View
```
┌─────────────────────────────────────────────────────┐
│ Compare Versions                                    │
├──────────────────────┬──────────────────────────────┤
│ Version 2            │ Version 3 (Current)          │
│ 1 week ago           │ 2 days ago                   │
├──────────────────────┼──────────────────────────────┤
│ {                    │ {                            │
│   "bucket_name": "…" │   "bucket_name": "…"         │
│   "encryption": {    │   "encryption": {            │
│-    "enabled": false │+    "enabled": true          │
│   }                  │+    "kms_key": "arn:…"       │
│ }                    │   }                          │
│                      │ }                            │
├──────────────────────┴──────────────────────────────┤
│                    [Rollback to v2]                 │
└─────────────────────────────────────────────────────┘
```

## Implementation Notes

### Service Layer
- `ModuleDemoService`: Business logic for demo management
- Transaction support for version creation
- Automatic diff calculation
- Version number management

### Controller Layer
- `ModuleDemoController`: HTTP request handling
- Input validation
- User authentication integration
- Error handling

### Frontend Services
- `moduleDemos.ts`: API client for demo operations
- Type definitions for Demo and DemoVersion
- Error handling and loading states

### State Management
- Local component state for forms
- React Query for data fetching and caching
- Optimistic updates for better UX

## Security Considerations

- User authentication required for all operations
- User ID tracked for audit trail
- Soft delete preserves data
- Version history immutable
- API key encryption (if config contains sensitive data)

## Testing Strategy

### Backend Tests
- Unit tests for service methods
- Integration tests for API endpoints
- Transaction rollback tests
- Version comparison tests

### Frontend Tests
- Component rendering tests
- Form validation tests
- API integration tests
- User workflow tests

## Future Enhancements

1. **Demo Templates**: Pre-defined demo configurations
2. **Demo Sharing**: Share demos between modules
3. **Demo Export/Import**: JSON export/import functionality
4. **Demo Tags**: Categorize demos with tags
5. **Demo Comments**: Add comments to versions
6. **Demo Approval**: Approval workflow for production demos
7. **Demo Scheduling**: Schedule demo updates
8. **Demo Notifications**: Notify on demo changes

## Migration Guide

### Database Migration
```bash
psql postgresql://postgres:postgres123@localhost:5432/iac_platform \
  -f scripts/create_module_demos.sql
```

### Backend Deployment
1. Deploy new models, services, controllers
2. Update router with new routes
3. Restart backend service

### Frontend Deployment
1. Deploy new components and services
2. Update module detail page
3. Clear browser cache

## API Examples

### Create Demo
```bash
curl -X POST http://localhost:8080/api/v1/modules/30/demos \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Production Config",
    "description": "Standard production configuration",
    "usage_notes": "Use this for production deployments",
    "config_data": {
      "bucket_name": "my-prod-bucket",
      "encryption": {
        "enabled": true,
        "kms_key": "arn:aws:kms:..."
      }
    }
  }'
```

### Update Demo
```bash
curl -X PUT http://localhost:8080/api/v1/demos/1 \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Production Config",
    "config_data": {
      "bucket_name": "my-prod-bucket",
      "encryption": {
        "enabled": true,
        "kms_key": "arn:aws:kms:...",
        "algorithm": "AES256"
      }
    },
    "change_summary": "Added encryption algorithm"
  }'
```

### Get Version History
```bash
curl http://localhost:8080/api/v1/demos/1/versions \
  -H "Authorization: Bearer $TOKEN"
```

### Compare Versions
```bash
curl "http://localhost:8080/api/v1/demos/1/compare?version1=1&version2=2" \
  -H "Authorization: Bearer $TOKEN"
```

### Rollback
```bash
curl -X POST http://localhost:8080/api/v1/demos/1/rollback \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"version_id": 2}'
```

## Conclusion

The Module Demo Management feature provides a comprehensive solution for managing demonstration configurations with full version control. It follows the same patterns as the workspace_resources implementation and integrates seamlessly with the existing module management system.

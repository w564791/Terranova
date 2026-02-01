# Demo Preview Feature Implementation Plan

## Current Status

###  Completed
- Backend API fully implemented (100%)
- DemoList component with View button
- DemoForm component for create/edit
- Basic integration in ModuleDetail page

### ⏳ In Progress
- Demo Preview feature (similar to ViewResource)

## Implementation Plan

### 1. Create DemoPreview Component

**File**: `frontend/src/components/DemoPreview.tsx`

**Features**:
- Display demo configuration using FormPreview
- Show demo metadata (name, description, version, etc.)
- Support form view and JSON view toggle
- Close button to return to list

**Key Dependencies**:
- Import `FormPreview` from `../components/DynamicForm`
- Load module schema to render form
- Use demo's `config_data` as form values

### 2. Integration Steps

**In ModuleDetail.tsx**:
```typescript
import DemoPreview from '../components/DemoPreview';

// Add state
const [viewingDemo, setViewingDemo] = useState<ModuleDemo | undefined>();

// Add modal
{viewingDemo && (
  <DemoPreview
    demo={viewingDemo}
    moduleId={module.id}
    onClose={() => setViewingDemo(undefined)}
  />
)}
```

### 3. DemoPreview Component Structure

```typescript
interface DemoPreviewProps {
  demo: ModuleDemo;
  moduleId: number;
  onClose: () => void;
}

const DemoPreview: React.FC<DemoPreviewProps> = ({ demo, moduleId, onClose }) => {
  const [schema, setSchema] = useState<FormSchema | null>(null);
  const [dataViewMode, setDataViewMode] = useState<'form' | 'json'>('form');
  
  useEffect(() => {
    loadModuleSchema();
  }, [moduleId]);
  
  const loadModuleSchema = async () => {
    // Load schema from /modules/:id/schemas
    // Process schema similar to ViewResource
  };
  
  return (
    <div className={styles.overlay} onClick={onClose}>
      <div className={styles.modal} onClick={(e) => e.stopPropagation()}>
        <div className={styles.header}>
          <h2>{demo.name}</h2>
          <button onClick={onClose}>×</button>
        </div>
        
        <div className={styles.content}>
          {/* Demo metadata */}
          <div className={styles.metadata}>
            <div>Version: v{demo.current_version?.version}</div>
            <div>Description: {demo.description}</div>
            {demo.usage_notes && <div>Usage: {demo.usage_notes}</div>}
          </div>
          
          {/* View toggle */}
          <div className={styles.viewToggle}>
            <button onClick={() => setDataViewMode('form')}>Form View</button>
            <button onClick={() => setDataViewMode('json')}>JSON View</button>
          </div>
          
          {/* Form preview */}
          {schema && (
            <FormPreview
              schema={schema}
              values={demo.current_version?.config_data || {}}
              onClose={() => {}}
              inline={true}
              viewMode={dataViewMode}
              onViewModeChange={setDataViewMode}
            />
          )}
        </div>
      </div>
    </div>
  );
};
```

### 4. Styling

**File**: `frontend/src/components/DemoPreview.module.css`

Use similar styles as DemoForm.module.css:
- Overlay with modal
- Header with close button
- Content area with metadata and form preview
- View toggle buttons
- Responsive design

## Reference Implementation

See `frontend/src/pages/ViewResource.tsx` for complete reference:
- Lines 1-50: Component structure and state
- Lines 100-200: Schema loading logic
- Lines 400-500: FormPreview usage
- Lines 500-600: View mode toggle

## Quick Implementation

Due to context limitations, here's a minimal working version:

```typescript
// DemoPreview.tsx
import React, { useState, useEffect } from 'react';
import { type ModuleDemo } from '../services/moduleDemos';
import { FormPreview } from '../components/DynamicForm';
import { processApiSchema } from '../utils/schemaTypeMapper';
import api from '../services/api';
import styles from './DemoPreview.module.css';

interface DemoPreviewProps {
  demo: ModuleDemo;
  moduleId: number;
  onClose: () => void;
}

const DemoPreview: React.FC<DemoPreviewProps> = ({ demo, moduleId, onClose }) => {
  const [schema, setSchema] = useState<any>(null);
  const [dataViewMode, setDataViewMode] = useState<'form' | 'json'>('form');
  
  useEffect(() => {
    loadSchema();
  }, [moduleId]);
  
  const loadSchema = async () => {
    try {
      const response = await api.get(`/modules/${moduleId}/schemas`);
      const schemas = response.data || response;
      if (schemas.length > 0) {
        const activeSchema = schemas.find((s: any) => s.status === 'active') || schemas[0];
        const processed = processApiSchema(activeSchema);
        setSchema(processed.schema_data);
      }
    } catch (error) {
      console.error('Failed to load schema:', error);
    }
  };
  
  return (
    <div className={styles.overlay} onClick={onClose}>
      <div className={styles.modal} onClick={(e) => e.stopPropagation()}>
        <div className={styles.header}>
          <h2>{demo.name} - v{demo.current_version?.version}</h2>
          <button className={styles.closeButton} onClick={onClose}>×</button>
        </div>
        
        <div className={styles.content}>
          {demo.description && (
            <p className={styles.description}>{demo.description}</p>
          )}
          
          {schema && (
            <FormPreview
              schema={schema}
              values={demo.current_version?.config_data || {}}
              onClose={() => {}}
              inline={true}
              viewMode={dataViewMode}
              onViewModeChange={setDataViewMode}
            />
          )}
        </div>
      </div>
    </div>
  );
};

export default DemoPreview;
```

## Next Steps

1. Create `DemoPreview.tsx` component
2. Create `DemoPreview.module.css` (reuse DemoForm styles)
3. Import and use in ModuleDetail.tsx
4. Test the preview functionality

## Estimated Time

- Component creation: 30 minutes
- Testing and refinement: 15 minutes
- Total: 45 minutes

---

**Note**: The preview feature requires the module to have an active schema defined. If no schema exists, show a message prompting the user to create one first.

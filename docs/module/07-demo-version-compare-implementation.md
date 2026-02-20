# Demo Version Compare Feature - Implementation Guide

## Overview

Add version comparison feature to DemoDetail page, similar to ViewResource's compare mode.

## Current Status

 DemoDetail page created
 Version selection dropdown implemented
 Basic view mode working
⏳ Compare mode needs to be added

## Implementation Steps

### 1. Add Compare Mode State

In `DemoDetail.tsx`, add:

```typescript
type ViewMode = 'view' | 'compare';
const [viewMode, setViewMode] = useState<ViewMode>('view');
const [compareFromVersion, setCompareFromVersion] = useState<number | null>(null);
const [compareToVersion, setCompareToVersion] = useState<number | null>(null);
const [diffFields, setDiffFields] = useState<DiffField[]>([]);

interface DiffField {
  field: string;
  type: 'added' | 'removed' | 'modified' | 'unchanged';
  oldValue?: any;
  newValue?: any;
  expanded?: boolean;
}
```

### 2. Add Compare Button

After version selector, add:

```typescript
{selectedVersion && selectedVersion !== demo.current_version?.version && (
  <button
    onClick={handleStartCompare}
    style={{
      padding: '10px 16px',
      background: '#007bff',
      color: 'white',
      border: 'none',
      borderRadius: '6px',
      cursor: 'pointer',
      fontSize: '14px'
    }}
  >
    对比版本
  </button>
)}
```

### 3. Implement Compare Functions

```typescript
const handleStartCompare = async () => {
  if (!selectedVersion || !demo?.current_version?.version) return;
  
  setViewMode('compare');
  setCompareFromVersion(selectedVersion);
  setCompareToVersion(demo.current_version.version);
  
  await handleCompareVersions(selectedVersion, demo.current_version.version);
};

const handleCompareVersions = async (fromVer: number, toVer: number) => {
  try {
    const result = await moduleDemoService.compareVersions(
      parseInt(demoId!),
      versions.find(v => v.version === fromVer)?.id || 0,
      versions.find(v => v.version === toVer)?.id || 0
    );
    
    // Parse diff and create diffFields
    const diff = JSON.parse(result.diff);
    const fields = calculateDiff(diff.old, diff.new);
    setDiffFields(fields);
  } catch (error: any) {
    showToast(extractErrorMessage(error), 'error');
  }
};

const calculateDiff = (oldConfig: any, newConfig: any): DiffField[] => {
  const fields: DiffField[] = [];
  const allKeys = new Set([...Object.keys(oldConfig), ...Object.keys(newConfig)]);
  
  allKeys.forEach(key => {
    const oldValue = oldConfig[key];
    const newValue = newConfig[key];
    
    if (!(key in oldConfig)) {
      fields.push({ field: key, type: 'added', newValue, expanded: false });
    } else if (!(key in newConfig)) {
      fields.push({ field: key, type: 'removed', oldValue, expanded: false });
    } else if (JSON.stringify(oldValue) !== JSON.stringify(newValue)) {
      fields.push({ field: key, type: 'modified', oldValue, newValue, expanded: false });
    } else {
      fields.push({ field: key, type: 'unchanged', oldValue, newValue, expanded: false });
    }
  });
  
  return fields;
};
```

### 4. Add Compare View UI

Replace the preview content section with:

```typescript
{viewMode === 'view' ? (
  // Current view mode content
  <div className={styles.previewContent}>
    {/* Existing preview code */}
  </div>
) : (
  // Compare mode
  <div>
    <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: '20px' }}>
      <h2>版本对比</h2>
      <button onClick={() => setViewMode('view')}>返回查看</button>
    </div>
    
    {/* Version selectors */}
    <div style={{ display: 'flex', gap: '16px', marginBottom: '20px' }}>
      <div style={{ flex: 1 }}>
        <label>From (旧版本):</label>
        <select
          value={compareFromVersion || ''}
          onChange={(e) => {
            const from = parseInt(e.target.value);
            setCompareFromVersion(from);
            if (compareToVersion) {
              handleCompareVersions(from, compareToVersion);
            }
          }}
        >
          {versions.map((v) => (
            <option key={v.id} value={v.version}>
              v{v.version}
            </option>
          ))}
        </select>
      </div>
      
      <div style={{ flex: 1 }}>
        <label>To (新版本):</label>
        <select
          value={compareToVersion || ''}
          onChange={(e) => {
            const to = parseInt(e.target.value);
            setCompareToVersion(to);
            if (compareFromVersion) {
              handleCompareVersions(compareFromVersion, to);
            }
          }}
        >
          {versions.map((v) => (
            <option key={v.id} value={v.version}>
              v{v.version} {v.is_latest ? '(当前)' : ''}
            </option>
          ))}
        </select>
      </div>
    </div>
    
    {/* Diff display */}
    <div>
      {diffFields.map((field, index) => (
        <div key={field.field} style={{ marginBottom: '16px', padding: '16px', background: 'white', borderRadius: '6px', border: '1px solid #e0e0e0' }}>
          <div style={{ display: 'flex', alignItems: 'center', gap: '12px' }}>
            <div style={{ 
              width: '4px', 
              height: '20px', 
              background: field.type === 'added' ? '#28a745' :
                         field.type === 'removed' ? '#dc3545' :
                         field.type === 'modified' ? '#ffc107' : '#6c757d'
            }} />
            <strong>{field.field}</strong>
            <span style={{ 
              padding: '2px 8px',
              borderRadius: '4px',
              fontSize: '11px',
              background: field.type === 'added' ? '#d4edda' :
                         field.type === 'removed' ? '#f8d7da' :
                         field.type === 'modified' ? '#fff3cd' : '#e9ecef',
              color: field.type === 'added' ? '#155724' :
                     field.type === 'removed' ? '#721c24' :
                     field.type === 'modified' ? '#856404' : '#495057'
            }}>
              {field.type}
            </span>
          </div>
          
          {field.type !== 'unchanged' && (
            <div style={{ marginTop: '12px' }}>
              {field.type === 'removed' && (
                <pre style={{ background: '#f8d7da', padding: '12px', borderRadius: '4px' }}>
                  {JSON.stringify(field.oldValue, null, 2)}
                </pre>
              )}
              {field.type === 'added' && (
                <pre style={{ background: '#d4edda', padding: '12px', borderRadius: '4px' }}>
                  {JSON.stringify(field.newValue, null, 2)}
                </pre>
              )}
              {field.type === 'modified' && (
                <div style={{ display: 'flex', gap: '12px' }}>
                  <div style={{ flex: 1 }}>
                    <div>旧版本:</div>
                    <pre style={{ background: '#f8d7da', padding: '12px', borderRadius: '4px' }}>
                      {JSON.stringify(field.oldValue, null, 2)}
                    </pre>
                  </div>
                  <div style={{ flex: 1 }}>
                    <div>新版本:</div>
                    <pre style={{ background: '#d4edda', padding: '12px', borderRadius: '4px' }}>
                      {JSON.stringify(field.newValue, null, 2)}
                    </pre>
                  </div>
                </div>
              )}
            </div>
          )}
        </div>
      ))}
    </div>
  </div>
)}
```

### 5. Add URL Parameter Support

```typescript
useEffect(() => {
  const urlMode = searchParams.get('mode');
  const urlVersion = searchParams.get('version');
  
  if (urlMode === 'compare' && urlVersion) {
    const versionNum = parseInt(urlVersion);
    setViewMode('compare');
    setCompareFromVersion(versionNum);
    setCompareToVersion(demo?.current_version?.version || null);
    if (demo?.current_version?.version) {
      handleCompareVersions(versionNum, demo.current_version.version);
    }
  }
}, [demo, searchParams]);
```

## Reference

See `frontend/src/pages/ViewResource.tsx`:
- Lines 50-80: ViewMode and compare state
- Lines 200-250: handleStartCompare function
- Lines 300-400: handleCompareVersions function
- Lines 500-700: Compare view UI

## Estimated Time

- Implementation: 1-2 hours
- Testing: 30 minutes
- Total: 1.5-2.5 hours

---

**Status**: Implementation guide complete
**Next**: Implement compare mode in DemoDetail.tsx

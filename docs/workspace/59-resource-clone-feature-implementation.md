# Resource Clone Feature Implementation

## Overview
Implemented "Clone and Edit Resource" functionality that allows users to create a copy of an existing resource with pre-filled configuration data.

## Implementation Date
2025-10-14

## Components Created

### 1. SplitButton Component
**File**: `frontend/src/components/SplitButton.tsx`

A reusable split button component that provides:
- Main action button
- Dropdown toggle button
- Menu with additional actions
- Click-outside detection to close menu
- Disabled state support

**Props**:
```typescript
interface SplitButtonProps {
  mainLabel: string;
  mainOnClick: () => void;
  menuItems: MenuItem[];
  disabled?: boolean;
  className?: string;
}

interface MenuItem {
  label: string;
  onClick: () => void;
  icon?: string;
}
```

**Styling**: `frontend/src/components/SplitButton.module.css`
- Primary color scheme matching project design
- Hover and active states
- Dropdown menu with shadow and border
- Responsive menu items

### 2. ViewResource Page Updates
**File**: `frontend/src/pages/ViewResource.tsx`

**Changes**:
1. Added import for `SplitButton` component
2. Added `handleCloneAndEdit()` function that navigates to edit page with `?mode=clone` parameter
3. Replaced single "编辑资源" button with `SplitButton`:
   - Main action: Edit resource (existing behavior)
   - Dropdown menu: Clone and edit resource (new feature)

**Code**:
```typescript
const handleCloneAndEdit = () => {
  navigate(`/workspaces/${id}/resources/${resourceId}/edit?mode=clone`);
};

<SplitButton
  mainLabel="编辑资源"
  mainOnClick={handleEdit}
  menuItems={[
    {
      label: '克隆并编辑资源',
      onClick: handleCloneAndEdit,
      icon: '📋'
    }
  ]}
/>
```

### 3. EditResource Page Updates
**File**: `frontend/src/pages/EditResource.tsx`

**Changes**:
1. Added `useSearchParams` import
2. Added state variables:
   - `isCloneMode`: boolean flag for clone mode
   - `moduleSource`: stores the module source for reuse
3. Added effect to detect clone mode from URL parameter
4. Updated `loadResource()` to store module source in state
5. Updated `handleSubmit()` to handle both edit and clone modes:
   - **Edit mode**: Updates existing resource (PUT request)
   - **Clone mode**: Creates new resource (POST request) with `_clone` suffix
6. Updated UI to show clone mode indicator:
   - Title changes to "克隆资源"
   - Resource name shows with `_clone` suffix
   - Blue badge showing "克隆模式"
   - Button text changes to "创建资源"

**Clone Mode Logic**:
```typescript
if (isCloneMode) {
  // Create new resource with cloned data
  const newTFCode = {
    module: {
      [`${resource?.resource_type}_${resource?.resource_name}_clone`]: [
        {
          source: moduleSource,
          ...formData
        }
      ]
    }
  };
  
  await api.post(`/workspaces/${id}/resources`, {
    resource_type: resource?.resource_type,
    resource_name: `${resource?.resource_name}_clone`,
    tf_code: newTFCode,
    variables: resource?.current_version?.variables || {},
    change_summary: changeSummary.trim()
  });
}
```

## User Flow

1. **View Resource Page**:
   - User views an existing resource
   - Sees split button with "编辑资源" as main action
   - Clicks dropdown arrow to reveal menu
   - Selects "克隆并编辑资源" option

2. **Clone Mode Edit Page**:
   - Page loads with all configuration pre-filled from source resource
   - Title shows "克隆资源"
   - Resource name displays with `_clone` suffix
   - Blue "克隆模式" badge visible
   - User can modify configuration as needed
   - User enters change summary
   - Clicks "创建资源" button

3. **Result**:
   - New resource created with cloned configuration
   - User redirected to workspace resources tab
   - Success toast: "资源克隆成功"

## Features

### Split Button
-  Clean, professional UI matching AWS console style
-  Dropdown menu with hover effects
-  Click-outside detection
-  Icon support for menu items
-  Disabled state support
-  Reusable component for future use

### Clone Functionality
-  Pre-fills all configuration from source resource
-  Preserves module source
-  Preserves variables
-  Automatic `_clone` suffix for new resource name
-  Clear visual indicator of clone mode
-  Creates new resource (doesn't modify original)
-  Requires change summary for audit trail

### User Experience
-  Intuitive split button design
-  Clear mode indication
-  Consistent with existing UI patterns
-  Helpful success/error messages
-  Smooth navigation flow

## Technical Details

### URL Parameters
- Clone mode triggered by: `?mode=clone`
- Example: `/workspaces/1/resources/123/edit?mode=clone`

### API Calls
- **Edit Mode**: `PUT /workspaces/:id/resources/:resourceId`
- **Clone Mode**: `POST /workspaces/:id/resources`

### Resource Naming
- Original: `s3_bucket_example`
- Cloned: `s3_bucket_example_clone`

### Module Key Naming
- Original: `s3_bucket_example`
- Cloned: `s3_bucket_example_clone`

## Testing Checklist

- [ ] Split button renders correctly
- [ ] Dropdown menu opens/closes properly
- [ ] Click outside closes menu
- [ ] Clone option navigates to edit page with mode=clone
- [ ] Edit page detects clone mode from URL
- [ ] Clone mode shows correct UI indicators
- [ ] Configuration is pre-filled correctly
- [ ] Module source is preserved
- [ ] Variables are preserved
- [ ] Resource name shows _clone suffix
- [ ] Submit creates new resource (not update)
- [ ] Success message shows "资源克隆成功"
- [ ] User redirected to resources tab
- [ ] New resource appears in list
- [ ] Original resource unchanged

## Future Enhancements

1. **Custom Clone Name**: Allow user to edit the cloned resource name before creation
2. **Clone from Version**: Allow cloning from specific historical versions
3. **Bulk Clone**: Clone multiple resources at once
4. **Clone to Different Workspace**: Clone resource to another workspace
5. **Clone Template**: Save clone as template for reuse

## Related Files

- `frontend/src/components/SplitButton.tsx`
- `frontend/src/components/SplitButton.module.css`
- `frontend/src/pages/ViewResource.tsx`
- `frontend/src/pages/EditResource.tsx`

## Git Commits

Implementation completed in single session on 2025-10-14.

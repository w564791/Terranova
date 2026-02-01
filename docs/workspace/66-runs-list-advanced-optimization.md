# Workspace Runs List - Advanced Optimization Implementation

## Overview
This document outlines the implementation plan for three advanced optimizations to the Workspace Runs list:
1. Professional date range picker with dual calendar view
2. Backend-powered filter counts for accurate statistics
3. Backend search functionality for comprehensive data querying

## Current State Analysis

### Frontend (WorkspaceDetail.tsx)
- Simple HTML5 date inputs for custom date range
- Filter counts calculated from current page data only
- Search performed on frontend (current page only)
- Pagination working with backend API
- URL state synchronization implemented

### Backend (workspace_task_controller.go)
- GetTasks API supports basic pagination
- Filters by task_type and status (query params)
- Returns: tasks, total, page, page_size, pages
- No search parameter support
- No filter counts in response

## Optimization 1: Professional Date Range Picker

### Design Requirements
Based on user screenshot, implement a date picker with:
- **Dual calendar view**: Display two months side-by-side
- **Absolute/Relative toggle**: Switch between absolute dates and relative time ranges
- **Time selection**: Hour:Minute:Second picker (00:00:00 format)
- **Timezone selector**: UTC and other timezone options
- **Clear visual hierarchy**: Start date → End date flow
- **Action buttons**: Clear, Cancel, Apply

### Implementation Plan

#### Option 1: Use react-datepicker library
```bash
npm install react-datepicker @types/react-datepicker
```

**Pros:**
- Mature, well-maintained library
- Built-in dual calendar support
- Time picker included
- Customizable styling
- Good TypeScript support

**Cons:**
- Additional dependency (~100KB)
- May need custom styling to match design

#### Option 2: Build custom component
**Pros:**
- Full control over design
- No external dependencies
- Exact match to requirements

**Cons:**
- More development time
- Need to handle edge cases
- Maintenance burden

**Recommendation**: Use react-datepicker for faster implementation and reliability.

### Component Structure
```typescript
// frontend/src/components/DateRangePicker.tsx
interface DateRangePickerProps {
  startDate: string;
  endDate: string;
  onApply: (startDate: string, endDate: string) => void;
  onClear: () => void;
  onCancel: () => void;
}

// Features:
// - Dual month calendar view
// - Absolute/Relative mode toggle
// - Time picker (HH:MM:SS)
// - Timezone selector
// - Preset ranges (Last 24h, 7d, 30d)
```

### Integration Points
1. Replace current custom date picker in WorkspaceDetail.tsx
2. Update state management for date/time values
3. Format dates to ISO 8601 for API calls
4. Handle timezone conversions

## Optimization 2: Backend Filter Counts

### Current Problem
```typescript
// Frontend calculates counts from current page only
const filterCounts = {
  all: total,  // ✓ Correct (from backend)
  errored: runs.filter(r => r.status === 'failed').length,  // ✗ Wrong (current page only)
  running: runs.filter(r => r.status === 'running').length,  // ✗ Wrong
  // ... etc
}
```

### Backend API Enhancement

#### Modified Response Format
```go
// GET /api/v1/workspaces/:id/tasks?page=1&page_size=10&status=xxx&start_date=xxx&end_date=xxx&search=xxx

{
  "tasks": [...],
  "total": 88,
  "page": 1,
  "page_size": 10,
  "pages": 9,
  "filter_counts": {
    "all": 88,
    "needs_attention": 5,    // pending + requires_approval
    "errored": 15,            // failed
    "running": 2,             // running
    "on_hold": 0,             // on_hold
    "success": 60,            // success + applied
    "cancelled": 6            // cancelled
  }
}
```

#### Implementation Steps

1. **Add filter counts calculation in GetTasks**
```go
func (c *WorkspaceTaskController) GetTasks(ctx *gin.Context) {
    // ... existing code ...
    
    // Calculate filter counts (respecting time range and search)
    baseQuery := c.db.Model(&models.WorkspaceTask{}).Where("workspace_id = ?", workspaceID)
    
    // Apply time range filter if provided
    if startDate := ctx.Query("start_date"); startDate != "" {
        baseQuery = baseQuery.Where("created_at >= ?", startDate)
    }
    if endDate := ctx.Query("end_date"); endDate != "" {
        baseQuery = baseQuery.Where("created_at <= ?", endDate)
    }
    
    // Apply search filter if provided
    if search := ctx.Query("search"); search != "" {
        baseQuery = baseQuery.Where(
            "description LIKE ? OR CAST(id AS TEXT) LIKE ?",
            "%"+search+"%", "%"+search+"%",
        )
    }
    
    filterCounts := map[string]int64{
        "all": 0,
        "needs_attention": 0,
        "errored": 0,
        "running": 0,
        "on_hold": 0,
        "success": 0,
        "cancelled": 0,
    }
    
    // Count all
    baseQuery.Count(&filterCounts["all"])
    
    // Count by status
    baseQuery.Where("status = ?", "failed").Count(&filterCounts["errored"])
    baseQuery.Where("status = ?", "running").Count(&filterCounts["running"])
    baseQuery.Where("status = ?", "on_hold").Count(&filterCounts["on_hold"])
    baseQuery.Where("status = ?", "cancelled").Count(&filterCounts["cancelled"])
    
    // Count success (success + applied)
    baseQuery.Where("status IN ?", []string{"success", "applied"}).Count(&filterCounts["success"])
    
    // Count needs attention (pending + requires_approval)
    baseQuery.Where("status IN ?", []string{"pending", "requires_approval"}).Count(&filterCounts["needs_attention"])
    
    ctx.JSON(http.StatusOK, gin.H{
        "tasks":         tasks,
        "total":         total,
        "page":          page,
        "page_size":     pageSize,
        "pages":         (int(total) + pageSize - 1) / pageSize,
        "filter_counts": filterCounts,
    })
}
```

2. **Update frontend to use backend counts**
```typescript
// Remove useMemo calculation
// Use counts from API response directly
const [filterCounts, setFilterCounts] = useState({
  all: 0,
  needsAttention: 0,
  errored: 0,
  running: 0,
  onHold: 0,
  success: 0,
  cancelled: 0,
});

// In fetchRuns:
if (data && data.filter_counts) {
  setFilterCounts({
    all: data.filter_counts.all,
    needsAttention: data.filter_counts.needs_attention,
    errored: data.filter_counts.errored,
    running: data.filter_counts.running,
    onHold: data.filter_counts.on_hold,
    success: data.filter_counts.success,
    cancelled: data.filter_counts.cancelled,
  });
}
```

## Optimization 3: Backend Search

### Current Problem
- Frontend searches only current page data
- Users cannot find runs on other pages
- Search by description, ID, task type limited to visible data

### Backend Implementation

#### Add Search Parameter Support
```go
func (c *WorkspaceTaskController) GetTasks(ctx *gin.Context) {
    // ... existing code ...
    
    query := c.db.Model(&models.WorkspaceTask{}).Where("workspace_id = ?", workspaceID)
    
    // Search parameter
    if search := ctx.Query("search"); search != "" {
        // Search in description, ID, and task_type
        query = query.Where(
            "description LIKE ? OR CAST(id AS TEXT) LIKE ? OR task_type LIKE ?",
            "%"+search+"%", "%"+search+"%", "%"+search+"%",
        )
    }
    
    // Time range filters
    if startDate := ctx.Query("start_date"); startDate != "" {
        query = query.Where("created_at >= ?", startDate)
    }
    if endDate := ctx.Query("end_date"); endDate != "" {
        query = query.Where("created_at <= ?", endDate)
    }
    
    // Status filter
    if status := ctx.Query("status"); status != "" {
        // Map frontend filter to backend status
        switch status {
        case "needs_attention":
            query = query.Where("status IN ?", []string{"pending", "requires_approval"})
        case "errored":
            query = query.Where("status = ?", "failed")
        case "success":
            query = query.Where("status IN ?", []string{"success", "applied"})
        case "cancelled":
            query = query.Where("status = ?", "cancelled")
        default:
            query = query.Where("status = ?", status)
        }
    }
    
    // ... rest of pagination logic ...
}
```

### Frontend Updates

#### Update fetchRuns to pass search parameter
```typescript
const fetchRuns = async () => {
  try {
    setLoading(true);
    
    // Build query parameters
    const params = new URLSearchParams({
      page: page.toString(),
      page_size: pageSize.toString(),
    });
    
    // Add search if present
    if (searchQuery) {
      params.append('search', searchQuery);
    }
    
    // Add time range if custom
    if (timeFilter === 'custom') {
      if (customStartDate) params.append('start_date', customStartDate);
      if (customEndDate) params.append('end_date', customEndDate);
    } else if (timeFilter !== 'all') {
      // Calculate relative time range
      const endDate = new Date();
      const startDate = new Date();
      if (timeFilter === '24h') startDate.setHours(startDate.getHours() - 24);
      else if (timeFilter === '7d') startDate.setDate(startDate.getDate() - 7);
      else if (timeFilter === '30d') startDate.setDate(startDate.getDate() - 30);
      
      params.append('start_date', startDate.toISOString());
      params.append('end_date', endDate.toISOString());
    }
    
    // Add status filter
    if (filter !== 'all') {
      params.append('status', filter);
    }
    
    const data = await api.get(`/workspaces/${workspaceId}/tasks?${params.toString()}`);
    
    // Update state with response
    setRuns(data.tasks || []);
    setTotal(data.total || 0);
    setFilterCounts(data.filter_counts || {});
  } catch (error) {
    // ... error handling ...
  } finally {
    setLoading(false);
  }
};
```

#### Remove frontend filtering
```typescript
// DELETE: No longer needed
// const filteredRuns = React.useMemo(() => {
//   let filtered = runs;
//   // ... frontend filtering logic ...
//   return filtered;
// }, [runs, filter, timeFilter, searchQuery]);

// Use runs directly from API
```

## Implementation Checklist

### Phase 1: Backend Search & Filter Counts
- [ ] Update GetTasks to accept search parameter
- [ ] Implement search logic (description, ID, task_type)
- [ ] Add time range filter support (start_date, end_date)
- [ ] Implement filter counts calculation
- [ ] Update response format to include filter_counts
- [ ] Test API with various filter combinations
- [ ] Handle edge cases (empty search, invalid dates)

### Phase 2: Frontend Backend Integration
- [ ] Update fetchRuns to pass search parameter
- [ ] Update fetchRuns to pass time range parameters
- [ ] Update fetchRuns to pass status filter
- [ ] Remove frontend filtering logic
- [ ] Update state to use backend filter counts
- [ ] Test search functionality
- [ ] Test filter counts accuracy
- [ ] Verify pagination works with filters

### Phase 3: Date Range Picker Component
- [ ] Install react-datepicker (or decide on custom implementation)
- [ ] Create DateRangePicker component
- [ ] Implement dual calendar view
- [ ] Add Absolute/Relative mode toggle
- [ ] Implement time picker (HH:MM:SS)
- [ ] Add timezone selector
- [ ] Implement preset ranges (24h, 7d, 30d)
- [ ] Add Clear/Cancel/Apply buttons
- [ ] Style to match design requirements
- [ ] Integrate into WorkspaceDetail.tsx
- [ ] Replace existing date inputs
- [ ] Test date/time selection
- [ ] Test timezone handling
- [ ] Verify ISO 8601 format for API

### Phase 4: Testing & Polish
- [ ] Test all filter combinations
- [ ] Test search with pagination
- [ ] Test date range with search
- [ ] Verify filter counts update correctly
- [ ] Test edge cases (no results, large datasets)
- [ ] Performance testing (large date ranges)
- [ ] Cross-browser testing
- [ ] Mobile responsiveness
- [ ] Update documentation
- [ ] Create user guide for new features

## Technical Considerations

### Performance
- Filter counts calculation may be expensive for large datasets
- Consider caching filter counts for frequently accessed workspaces
- Use database indexes on created_at, status, description columns
- Limit search query complexity

### Database Indexes
```sql
-- Recommended indexes for performance
CREATE INDEX idx_workspace_tasks_workspace_created ON workspace_tasks(workspace_id, created_at DESC);
CREATE INDEX idx_workspace_tasks_status ON workspace_tasks(status);
CREATE INDEX idx_workspace_tasks_description ON workspace_tasks(description);
```

### Error Handling
- Invalid date formats
- Timezone conversion errors
- Search query too broad (performance)
- Empty result sets
- API timeout for complex queries

### User Experience
- Show loading state during search
- Debounce search input (300ms)
- Clear search button
- Search result count display
- "No results" message with suggestions
- Preserve filter state in URL

## API Specification

### Updated GetTasks Endpoint

```
GET /api/v1/workspaces/:id/tasks

Query Parameters:
- page: int (default: 1)
- page_size: int (default: 10, max: 100)
- search: string (optional) - Search in description, ID, task_type
- start_date: ISO 8601 datetime (optional) - Filter tasks created after this date
- end_date: ISO 8601 datetime (optional) - Filter tasks created before this date
- status: string (optional) - Filter by status (all, needs_attention, errored, running, on_hold, success, cancelled)
- task_type: string (optional) - Filter by task type (plan, apply, plan_and_apply)

Response:
{
  "tasks": [
    {
      "id": 123,
      "workspace_id": 10,
      "task_type": "plan",
      "status": "success",
      "description": "Update S3 bucket configuration",
      "created_at": "2025-10-13T12:00:00Z",
      "created_by": 5,
      "changes_add": 2,
      "changes_change": 1,
      "changes_destroy": 0,
      "stage": "completed",
      "started_at": "2025-10-13T12:00:05Z",
      "completed_at": "2025-10-13T12:01:30Z"
    }
  ],
  "total": 88,
  "page": 1,
  "page_size": 10,
  "pages": 9,
  "filter_counts": {
    "all": 88,
    "needs_attention": 5,
    "errored": 15,
    "running": 2,
    "on_hold": 0,
    "success": 60,
    "cancelled": 6
  }
}
```

## Migration Notes

### Breaking Changes
- None (backward compatible)
- New fields added to response (filter_counts)
- New query parameters (optional)

### Rollback Plan
- Backend changes are additive
- Frontend can fall back to client-side filtering if API doesn't return filter_counts
- Date picker can be feature-flagged

## Success Metrics

### Functionality
- ✓ Search works across all pages
- ✓ Filter counts show accurate totals
- ✓ Date range picker provides professional UX
- ✓ All filters work together correctly
- ✓ Performance acceptable (<500ms for typical queries)

### User Experience
- ✓ Intuitive date selection
- ✓ Clear visual feedback
- ✓ Fast search response
- ✓ Accurate result counts
- ✓ Smooth pagination

## Next Steps

1. Review and approve implementation plan
2. Start with Phase 1 (Backend changes)
3. Test backend thoroughly before frontend changes
4. Implement Phase 2 (Frontend integration)
5. Implement Phase 3 (Date picker)
6. Complete Phase 4 (Testing & polish)
7. Deploy and monitor

## References

- react-datepicker: https://reactdatepicker.com/
- ISO 8601 date format: https://en.wikipedia.org/wiki/ISO_8601
- GORM query documentation: https://gorm.io/docs/query.html
- Previous optimization: docs/workspace/runs-list-final-optimization.md

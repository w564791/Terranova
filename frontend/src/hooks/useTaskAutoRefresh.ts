import { useEffect, useRef, useCallback } from 'react';
import api from '../services/api';

interface Task {
  id: number;
  status: string;
  [key: string]: any;
}

interface UseTaskAutoRefreshOptions {
  workspaceId: string;
  enabled: boolean;
  onUpdate: (incompleteTasks: Task[]) => void;
  interval?: number;
}

// Incomplete statuses that need refreshing
const INCOMPLETE_STATUSES = ['pending', 'running', 'apply_pending', 'requires_approval', 'plan_completed'];

/**
 * Custom hook for auto-refreshing incomplete tasks
 * Only fetches tasks with incomplete statuses to minimize server load
 */
export const useTaskAutoRefresh = ({
  workspaceId,
  enabled,
  onUpdate,
  interval = 5000, // 5 seconds default
}: UseTaskAutoRefreshOptions) => {
  const intervalRef = useRef<number | null>(null);
  const isRefreshingRef = useRef(false);

  const fetchIncompleteTasks = useCallback(async () => {
    if (isRefreshingRef.current || !enabled) return;

    try {
      isRefreshingRef.current = true;

      // Fetch all incomplete tasks in parallel
      const statusQueries = INCOMPLETE_STATUSES.map(status =>
        api.get(`/workspaces/${workspaceId}/tasks?status=${status}&page=1&page_size=10000`)
      );

      const results = await Promise.all(statusQueries);
      
      // Combine all incomplete tasks
      const allIncompleteTasks: Task[] = [];
      results.forEach((response: any) => {
        const data = response.data || response;
        if (data.tasks && Array.isArray(data.tasks)) {
          allIncompleteTasks.push(...data.tasks);
        }
      });

      // Remove duplicates (in case a task appears in multiple status queries)
      const uniqueTasks = Array.from(
        new Map(allIncompleteTasks.map(task => [task.id, task])).values()
      );

      console.log(`[TaskAutoRefresh] Found ${uniqueTasks.length} incomplete tasks`);
      
      // Call the update callback
      onUpdate(uniqueTasks);

      // If no incomplete tasks, we can stop polling
      if (uniqueTasks.length === 0) {
        console.log('[TaskAutoRefresh] No incomplete tasks, stopping auto-refresh');
        if (intervalRef.current) {
          clearInterval(intervalRef.current);
          intervalRef.current = null;
        }
      }
    } catch (error) {
      console.error('[TaskAutoRefresh] Failed to fetch incomplete tasks:', error);
    } finally {
      isRefreshingRef.current = false;
    }
  }, [workspaceId, enabled, onUpdate]);

  useEffect(() => {
    if (!enabled) {
      // Clear interval when disabled
      if (intervalRef.current) {
        clearInterval(intervalRef.current);
        intervalRef.current = null;
      }
      return;
    }

    // Start polling
    console.log('[TaskAutoRefresh] Starting auto-refresh');
    fetchIncompleteTasks(); // Fetch immediately

    intervalRef.current = setInterval(fetchIncompleteTasks, interval);

    // Cleanup on unmount or when dependencies change
    return () => {
      if (intervalRef.current) {
        clearInterval(intervalRef.current);
        intervalRef.current = null;
      }
    };
  }, [enabled, fetchIncompleteTasks, interval]);

  return {
    refresh: fetchIncompleteTasks,
  };
};

/**
 * Shared task status labels / categories.
 * Used by Workspaces list, Workspace Runs tab, Overview latest run, and TaskDetail
 * so all surfaces show the same human-readable status for a given task.
 */

export type TaskStatusCategory =
  | 'success'
  | 'attention'
  | 'error'
  | 'running'
  | 'pending'
  | 'neutral';

export interface TaskStatusInput {
  status: string;
  task_type?: string;
  /** Optional stage for running tasks (TaskDetail shows finer labels). */
  stage?: string;
  apply_confirmed_by?: string | null;
  apply_confirmed_at?: string | null;
}

/**
 * Human-readable status label. Prefer TaskDetail semantics as the source of truth.
 * When `detailedRunning` is true (TaskDetail), expand running into stage-specific text.
 */
export function getTaskStatusLabel(
  input: TaskStatusInput,
  options?: { detailedRunning?: boolean }
): string {
  const status = input.status || '';
  const taskType = input.task_type || '';

  switch (status) {
    case 'pending':
      return 'Pending';
    case 'waiting':
      return 'Waiting';
    case 'running': {
      if (!options?.detailedRunning) {
        return 'Running';
      }
      if (input.apply_confirmed_by || input.apply_confirmed_at) {
        return 'Applying...';
      }
      const stage = input.stage || '';
      if (stage === 'init') return 'Initializing...';
      if (stage === 'fetching') return 'Fetching...';
      if (stage === 'plan' || stage === 'planning') return 'Planning...';
      if (
        stage === 'apply' ||
        stage === 'applying' ||
        stage === 'pre_apply' ||
        stage === 'restoring_plan'
      ) {
        return 'Applying...';
      }
      if (stage === 'pending') return 'Pending...';
      return 'Running...';
    }
    case 'plan_completed':
    case 'apply_pending':
    case 'requires_approval':
      return 'Awaiting Confirmation';
    case 'decision_required':
      return 'Decision Required';
    case 'planned_and_finished':
      return 'Planned and Finished';
    case 'success':
    case 'applied': {
      if (taskType === 'plan' || (taskType === 'plan_and_apply' && status === 'success')) {
        return 'Planned';
      }
      if (taskType === 'apply' || (taskType === 'plan_and_apply' && status === 'applied')) {
        return 'Applied';
      }
      if (status === 'applied') return 'Applied';
      // success without known task type: treat as Applied for apply-like defaults
      return taskType === 'plan' ? 'Planned' : 'Applied';
    }
    case 'failed':
      return 'Failed';
    case 'cancelled':
      return 'Cancelled';
    default:
      return status || 'Unknown';
  }
}

/** Category used for color / indicator styling. */
export function getTaskStatusCategory(status: string): TaskStatusCategory {
  if (status === 'success' || status === 'applied' || status === 'planned_and_finished') {
    return 'success';
  }
  if (
    status === 'decision_required' ||
    status === 'requires_approval' ||
    status === 'apply_pending' ||
    status === 'plan_completed'
  ) {
    return 'attention';
  }
  if (status === 'failed') {
    return 'error';
  }
  if (status === 'running') {
    return 'running';
  }
  if (status === 'pending' || status === 'waiting') {
    return 'pending';
  }
  return 'neutral';
}

/** Whether status is a non-terminal (still progressing / needs action) state. */
export function isNonFinalTaskStatus(status: string): boolean {
  return [
    'pending',
    'waiting',
    'running',
    'plan_completed',
    'apply_pending',
    'decision_required',
    'requires_approval',
  ].includes(status);
}

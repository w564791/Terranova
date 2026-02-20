-- Create task_comments table
CREATE TABLE IF NOT EXISTS task_comments (
  id SERIAL PRIMARY KEY,
  task_id INTEGER NOT NULL REFERENCES workspace_tasks(id) ON DELETE CASCADE,
  user_id INTEGER,
  username VARCHAR(255) NOT NULL,
  comment TEXT NOT NULL,
  action_type VARCHAR(50), -- 'comment', 'confirm_apply', 'cancel', 'cancel_previous'
  created_at TIMESTAMP DEFAULT NOW()
);

-- Create indexes
CREATE INDEX IF NOT EXISTS idx_task_comments_task_id ON task_comments(task_id);
CREATE INDEX IF NOT EXISTS idx_task_comments_created_at ON task_comments(created_at DESC);

-- Add comment to describe the table
COMMENT ON TABLE task_comments IS 'Task comments and action logs';
COMMENT ON COLUMN task_comments.action_type IS 'Type of action: comment, confirm_apply, cancel, cancel_previous';

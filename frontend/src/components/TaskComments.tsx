import React, { useEffect, useState } from 'react';
import api from '../services/api';
import styles from './TaskComments.module.css';

interface Comment {
  id: number;
  task_id: number;
  user_id?: number;
  username: string;
  comment: string;
  action_type?: string;
  created_at: string;
}

interface TaskCommentsProps {
  workspaceId: number | string;
  taskId: number;
}

const TaskComments: React.FC<TaskCommentsProps> = ({ workspaceId, taskId }) => {
  const [comments, setComments] = useState<Comment[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    fetchComments();
  }, [workspaceId, taskId]);

  const fetchComments = async () => {
    try {
      setLoading(true);
      const data: any = await api.get(`/workspaces/${workspaceId}/tasks/${taskId}/comments`);
      setComments(data.comments || []);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load comments');
    } finally {
      setLoading(false);
    }
  };

  const formatDate = (dateString: string) => {
    const date = new Date(dateString);
    const now = new Date();
    const diffMs = now.getTime() - date.getTime();
    const diffMins = Math.floor(diffMs / 60000);
    const diffHours = Math.floor(diffMs / 3600000);
    const diffDays = Math.floor(diffMs / 86400000);

    if (diffMins < 1) return 'just now';
    if (diffMins < 60) return `${diffMins} minute${diffMins > 1 ? 's' : ''} ago`;
    if (diffHours < 24) return `${diffHours} hour${diffHours > 1 ? 's' : ''} ago`;
    if (diffDays < 7) return `${diffDays} day${diffDays > 1 ? 's' : ''} ago`;
    
    return date.toLocaleDateString('en-US', {
      month: 'short',
      day: 'numeric',
      year: date.getFullYear() !== now.getFullYear() ? 'numeric' : undefined,
    });
  };

  const getActionTypeLabel = (actionType?: string) => {
    switch (actionType) {
      case 'confirm_apply':
        return 'confirmed apply';
      case 'cancel':
        return 'cancelled task';
      case 'cancel_previous':
        return 'cancelled previous tasks';
      default:
        return 'commented';
    }
  };

  if (loading) {
    return (
      <div className={styles.commentsContainer}>
        <div className={styles.commentsHeader}>
          <h3>Comments</h3>
        </div>
        <div className={styles.loading}>Loading comments...</div>
      </div>
    );
  }

  if (error) {
    return (
      <div className={styles.commentsContainer}>
        <div className={styles.commentsHeader}>
          <h3>Comments</h3>
        </div>
        <div className={styles.error}>{error}</div>
      </div>
    );
  }

  return (
    <div className={styles.commentsContainer}>
      <div className={styles.commentsHeader}>
        <h3>Comments</h3>
        <span className={styles.commentCount}>{comments.length}</span>
      </div>

      {comments.length === 0 ? (
        <div className={styles.noComments}>
          <p>No comments yet</p>
          <span>Be the first to add a comment</span>
        </div>
      ) : (
        <div className={styles.commentsList}>
          {comments.map((comment) => (
            <div key={comment.id} className={styles.commentItem}>
              <div className={styles.commentAvatar}>
                {comment.username.charAt(0).toUpperCase()}
              </div>
              <div className={styles.commentContent}>
                <div className={styles.commentHeader}>
                  <span className={styles.commentUsername}>{comment.username}</span>
                  {comment.action_type && (
                    <span className={styles.commentAction}>
                      {getActionTypeLabel(comment.action_type)}
                    </span>
                  )}
                  <span className={styles.commentTime}>{formatDate(comment.created_at)}</span>
                </div>
                <div className={styles.commentText}>{comment.comment}</div>
              </div>
            </div>
          ))}
        </div>
      )}
    </div>
  );
};

export default TaskComments;

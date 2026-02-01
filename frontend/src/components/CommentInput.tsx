import React, { useState } from 'react';
import styles from './CommentInput.module.css';

interface CommentInputProps {
  onSubmit: (comment: string) => void;
  onCancel: () => void;
  placeholder?: string;
  submitLabel?: string;
  isSubmitting?: boolean;
  maxLength?: number;
}

const CommentInput: React.FC<CommentInputProps> = ({
  onSubmit,
  onCancel,
  placeholder = 'Add a comment...',
  submitLabel = 'Submit',
  isSubmitting = false,
  maxLength = 500,
}) => {
  const [comment, setComment] = useState('');
  const [error, setError] = useState('');

  const handleSubmit = () => {
    if (!comment.trim()) {
      setError('Comment cannot be empty');
      return;
    }

    if (comment.length > maxLength) {
      setError(`Comment must be less than ${maxLength} characters`);
      return;
    }

    onSubmit(comment.trim());
  };

  const handleChange = (e: React.ChangeEvent<HTMLTextAreaElement>) => {
    setComment(e.target.value);
    if (error) setError('');
  };

  const handleKeyDown = (e: React.KeyboardEvent<HTMLTextAreaElement>) => {
    if (e.key === 'Enter' && (e.metaKey || e.ctrlKey)) {
      e.preventDefault();
      handleSubmit();
    }
  };

  return (
    <div className={styles.commentInputContainer}>
      <div className={styles.inputWrapper}>
        <textarea
          className={`${styles.textarea} ${error ? styles.textareaError : ''}`}
          value={comment}
          onChange={handleChange}
          onKeyDown={handleKeyDown}
          placeholder={placeholder}
          rows={4}
          disabled={isSubmitting}
          autoFocus
        />
        <div className={styles.inputFooter}>
          <span className={styles.charCount}>
            {comment.length} / {maxLength}
          </span>
          {error && <span className={styles.error}>{error}</span>}
        </div>
      </div>

      <div className={styles.actions}>
        <button
          type="button"
          className={styles.cancelButton}
          onClick={onCancel}
          disabled={isSubmitting}
        >
          Cancel
        </button>
        <button
          type="button"
          className={styles.submitButton}
          onClick={handleSubmit}
          disabled={isSubmitting || !comment.trim()}
        >
          {isSubmitting ? 'Submitting...' : submitLabel}
        </button>
      </div>

      <div className={styles.hint}>
        Press <kbd>⌘</kbd> + <kbd>Enter</kbd> to submit
      </div>
    </div>
  );
};

export default CommentInput;

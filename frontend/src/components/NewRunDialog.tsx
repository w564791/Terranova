import React, { useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { useToast } from '../contexts/ToastContext';
import { extractErrorMessage } from '../utils/errorHandler';
import api from '../services/api';
import styles from './NewRunDialog.module.css';

interface NewRunDialogProps {
  isOpen: boolean;
  workspaceId: string;
  onClose: () => void;
  onSuccess?: () => void;
}

type RunType = 'plan' | 'plan_and_apply' | 'add_resources';

const NewRunDialog: React.FC<NewRunDialogProps> = ({
  isOpen,
  workspaceId,
  onClose,
  onSuccess
}) => {
  const navigate = useNavigate();
  const { showToast } = useToast();
  const [runType, setRunType] = useState<RunType>('plan');
  const [description, setDescription] = useState('Triggered via UI');
  const [loading, setLoading] = useState(false);

  if (!isOpen) return null;

  const handleSubmit = async () => {
    if (runType === 'add_resources') {
      // 跳转到添加资源页面
      navigate(`/workspaces/${workspaceId}/add-resources`);
      onClose();
      return;
    }

    // 执行Plan或Plan+Apply
    try {
      setLoading(true);
      
      // 创建Plan任务，包含description和run_type
      const response: any = await api.post(`/workspaces/${workspaceId}/tasks/plan`, {
        description: description.trim() || undefined,
        run_type: runType  // 传递run_type: "plan" 或 "plan_and_apply"
      });
      
      // 获取创建的任务ID
      const taskId = response.data?.task?.id || response.task?.id;
      
      showToast(
        runType === 'plan' 
          ? 'Plan任务创建成功' 
          : 'Plan+Apply任务创建成功',
        'success'
      );
      
      if (onSuccess) {
        onSuccess();
      }
      
      onClose();
      
      // 直接跳转到任务详情页
      if (taskId) {
        navigate(`/workspaces/${workspaceId}/tasks/${taskId}`);
      } else {
        // 如果没有获取到taskId，跳转到Runs标签页
        navigate(`/workspaces/${workspaceId}?tab=runs`);
      }
    } catch (error: any) {
      const message = extractErrorMessage(error);
      showToast(message, 'error');
    } finally {
      setLoading(false);
    }
  };

  const handleOverlayClick = (e: React.MouseEvent) => {
    if (e.target === e.currentTarget && !loading) {
      onClose();
    }
  };

  return (
    <div className={styles.overlay} onClick={handleOverlayClick}>
      <div className={styles.dialog}>
        <div className={styles.header}>
          <h2 className={styles.title}>Start a new run</h2>
          <button 
            className={styles.closeBtn} 
            onClick={onClose}
            disabled={loading}
          >
            ×
          </button>
        </div>

        <div className={styles.content}>
          <p className={styles.description}>
            Choose how you want to start this run:
          </p>

          {/* Description field */}
          <div className={styles.formGroup}>
            <label htmlFor="run-description" className={styles.label}>
              Description (optional)
            </label>
            <input
              id="run-description"
              type="text"
              className={styles.input}
              placeholder="Enter a description for this run..."
              value={description}
              onChange={(e) => setDescription(e.target.value)}
              disabled={loading}
            />
            <p className={styles.hint}>
              Add a brief description to help identify this run later.
            </p>
          </div>

          <div className={styles.options}>
            {/* Option 1: Plan */}
            <label className={`${styles.option} ${runType === 'plan' ? styles.optionSelected : ''}`}>
              <input
                type="radio"
                name="runType"
                value="plan"
                checked={runType === 'plan'}
                onChange={() => setRunType('plan')}
                disabled={loading}
              />
              <div className={styles.optionContent}>
                <div className={styles.optionTitle}>Plan</div>
                <div className={styles.optionDesc}>
                  Execute plan to preview changes. Uses existing resources in workspace.
                </div>
              </div>
            </label>

            {/* Option 2: Plan and apply */}
            <label className={`${styles.option} ${runType === 'plan_and_apply' ? styles.optionSelected : ''}`}>
              <input
                type="radio"
                name="runType"
                value="plan_and_apply"
                checked={runType === 'plan_and_apply'}
                onChange={() => setRunType('plan_and_apply')}
                disabled={loading}
              />
              <div className={styles.optionContent}>
                <div className={styles.optionTitle}>Plan and apply</div>
                <div className={styles.optionDesc}>
                  Execute complete workflow. Plan first, then apply based on workspace's Apply Method setting (Auto apply: On/Off).
                </div>
              </div>
            </label>

            {/* Option 3: Add resources */}
            <label className={`${styles.option} ${runType === 'add_resources' ? styles.optionSelected : ''}`}>
              <input
                type="radio"
                name="runType"
                value="add_resources"
                checked={runType === 'add_resources'}
                onChange={() => setRunType('add_resources')}
                disabled={loading}
              />
              <div className={styles.optionContent}>
                <div className={styles.optionTitle}>Add resources</div>
                <div className={styles.optionDesc}>
                  Select modules from library, configure them, and create new resource versions. Then choose to run Plan or Plan+Apply.
                </div>
              </div>
            </label>
          </div>
        </div>

        <div className={styles.footer}>
          <button
            className={styles.btnCancel}
            onClick={onClose}
            disabled={loading}
          >
            Cancel
          </button>
          <button
            className={styles.btnSubmit}
            onClick={handleSubmit}
            disabled={loading}
          >
            {loading ? 'Creating...' : 'Start Run'}
          </button>
        </div>
      </div>
    </div>
  );
};

export default NewRunDialog;

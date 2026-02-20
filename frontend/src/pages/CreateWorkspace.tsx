import React, { useState, useEffect } from 'react';
import { useNavigate } from 'react-router-dom';
import type { CreateWorkspaceRequest, WorkspaceFormData } from '../services/workspaces';
import { workspaceService } from '../services/workspaces';
import { useToast } from '../contexts/ToastContext';
import { extractErrorMessage, logError } from '../utils/errorHandler';
import styles from './CreateWorkspace.module.css';

const CreateWorkspace: React.FC = () => {
  const navigate = useNavigate();
  const { success, error: showError } = useToast();
  const [isSubmitting, setIsSubmitting] = useState(false);
  const [formData, setFormData] = useState<WorkspaceFormData | null>(null);
  const [isLoadingFormData, setIsLoadingFormData] = useState(true);
  const [errors, setErrors] = useState<Record<string, string>>({});

  const [workspaceData, setWorkspaceData] = useState<CreateWorkspaceRequest>({
    name: '',
    description: '',
    tags: {},
    // 使用默认值，在settings页面配置
    execution_mode: 'local',
    state_backend: 'local',
    auto_apply: false,
    terraform_version: 'latest',
    workdir: '/workspace',
  });

  const [tagPairs, setTagPairs] = useState<Array<{ key: string; value: string }>>([
    { key: '', value: '' }
  ]);

  const handleFieldChange = (field: keyof CreateWorkspaceRequest, value: any) => {
    setWorkspaceData(prev => ({
      ...prev,
      [field]: value,
    }));
    // 清除该字段的错误
    if (errors[field]) {
      setErrors(prev => {
        const newErrors = { ...prev };
        delete newErrors[field];
        return newErrors;
      });
    }
  };

  const handleTagChange = (index: number, field: 'key' | 'value', value: string) => {
    const newTagPairs = [...tagPairs];
    newTagPairs[index][field] = value;
    setTagPairs(newTagPairs);

    // 更新workspaceData中的tags
    const tags: Record<string, any> = {};
    newTagPairs.forEach(pair => {
      if (pair.key.trim()) {
        tags[pair.key.trim()] = pair.value.trim();
      }
    });
    setWorkspaceData(prev => ({ ...prev, tags }));
  };

  const addTagPair = () => {
    setTagPairs([...tagPairs, { key: '', value: '' }]);
  };

  const removeTagPair = (index: number) => {
    if (tagPairs.length > 1) {
      const newTagPairs = tagPairs.filter((_, i) => i !== index);
      setTagPairs(newTagPairs);

      // 更新workspaceData中的tags
      const tags: Record<string, any> = {};
      newTagPairs.forEach(pair => {
        if (pair.key.trim()) {
          tags[pair.key.trim()] = pair.value.trim();
        }
      });
      setWorkspaceData(prev => ({ ...prev, tags }));
    }
  };

  const validateForm = (): boolean => {
    const newErrors: Record<string, string> = {};

    // 名称验证
    if (!workspaceData.name.trim()) {
      newErrors.name = 'Workspace name is required';
    } else if (!/^[a-zA-Z0-9-_]+$/.test(workspaceData.name)) {
      newErrors.name = 'Name can only contain letters, numbers, dashes, and underscores';
    } else if (workspaceData.name.length < 3) {
      newErrors.name = 'Name must be at least 3 characters';
    } else if (workspaceData.name.length > 50) {
      newErrors.name = 'Name cannot exceed 50 characters';
    }

    setErrors(newErrors);
    return Object.keys(newErrors).length === 0;
  };

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    
    if (!validateForm()) {
      return;
    }

    setIsSubmitting(true);

    try {
      const response = await workspaceService.createWorkspace(workspaceData);
      
      success('Workspace创建成功');
      
      // 跳转到详情页
      navigate(`/workspaces/${response.data.id}`);
      
    } catch (err: any) {
      logError('创建Workspace', err);
      showError('创建失败: ' + extractErrorMessage(err));
    } finally {
      setIsSubmitting(false);
    }
  };

  return (
    <div className={styles.container}>
      <div className={styles.header}>
        <div className={styles.breadcrumb}>
          <button onClick={() => navigate('/workspaces')} className={styles.breadcrumbLink}>
            Workspaces
          </button>
          <span className={styles.breadcrumbSeparator}>/</span>
          <span className={styles.breadcrumbCurrent}>New Workspace</span>
        </div>
      </div>

      <div className={styles.content}>
        <div className={styles.titleSection}>
          <h1 className={styles.title}>Create a new Workspace</h1>
          <p className={styles.subtitle}>
            Workspaces determine how Terraform Cloud organizes infrastructure. A workspace contains your Terraform configuration (infrastructure as code), shared variable values, your current and historical Terraform state, and run logs.
          </p>
        </div>

        <form onSubmit={handleSubmit} className={styles.form}>
          {/* Workspace Name */}
          <div className={styles.formSection}>
            <div className={styles.field}>
              <label className={styles.label}>
                Workspace Name <span className={styles.required}>*</span>
              </label>
              <input
                type="text"
                value={workspaceData.name}
                onChange={(e) => handleFieldChange('name', e.target.value)}
                className={`${styles.input} ${errors.name ? styles.inputError : ''}`}
                placeholder="e.g. my-workspace"
                disabled={isSubmitting}
                autoFocus
              />
              {errors.name && (
                <div className={styles.error}>{errors.name}</div>
              )}
              <div className={styles.hint}>
                The name of your workspace is unique and used in tools, routing, and UI. Dashes, underscores, and alphanumeric characters are permitted.
              </div>
            </div>

            <div className={styles.field}>
              <label className={styles.label}>
                Description
                <span className={styles.optional}>Optional</span>
              </label>
              <textarea
                value={workspaceData.description}
                onChange={(e) => handleFieldChange('description', e.target.value)}
                className={styles.textarea}
                placeholder="A brief description of this workspace"
                rows={3}
                disabled={isSubmitting}
              />
            </div>
            
            <div className={styles.field}>
              <label className={styles.label}>
                Tags
                <span className={styles.optional}>Optional</span>
              </label>
              <div className={styles.tagList}>
                {tagPairs.map((pair, index) => (
                  <div key={index} className={styles.tagPair}>
                    <input
                      type="text"
                      value={pair.key}
                      onChange={(e) => handleTagChange(index, 'key', e.target.value)}
                      className={styles.tagInput}
                      placeholder="Key"
                      disabled={isSubmitting}
                    />
                    <span className={styles.tagSeparator}>=</span>
                    <input
                      type="text"
                      value={pair.value}
                      onChange={(e) => handleTagChange(index, 'value', e.target.value)}
                      className={styles.tagInput}
                      placeholder="Value"
                      disabled={isSubmitting}
                    />
                    {tagPairs.length > 1 && (
                      <button
                        type="button"
                        onClick={() => removeTagPair(index)}
                        className={styles.removeTagButton}
                        disabled={isSubmitting}
                      >
                        ✕
                      </button>
                    )}
                  </div>
                ))}
              </div>
              <button
                type="button"
                onClick={addTagPair}
                className={styles.addTagButton}
                disabled={isSubmitting}
              >
                + Add tag
              </button>
              <div className={styles.hint}>
                Tags help organize and filter workspaces. You can also configure this later in workspace settings.
              </div>
            </div>
          </div>

          {/* Actions */}
          <div className={styles.actions}>
            <button
              type="button"
              onClick={() => navigate('/workspaces')}
              className={styles.cancelButton}
              disabled={isSubmitting}
            >
              Cancel
            </button>
            <button
              type="submit"
              disabled={isSubmitting}
              className={styles.submitButton}
            >
              {isSubmitting ? 'Creating workspace...' : 'Create workspace'}
            </button>
          </div>
        </form>
      </div>
    </div>
  );
};

export default CreateWorkspace;

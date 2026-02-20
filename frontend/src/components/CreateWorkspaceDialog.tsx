import React, { useState, useEffect } from 'react';
import { useNavigate } from 'react-router-dom';
import type { CreateWorkspaceRequest, WorkspaceFormData } from '../services/workspaces';
import { workspaceService } from '../services/workspaces';
import { useToast } from '../contexts/ToastContext';
import { extractErrorMessage, logError } from '../utils/errorHandler';
import ConfirmDialog from './ConfirmDialog';
import styles from './CreateWorkspaceDialog.module.css';

interface CreateWorkspaceDialogProps {
  isOpen: boolean;
  onClose: () => void;
  onSuccess?: () => void;
}

const STORAGE_KEY = 'workspace_create_form';

const CreateWorkspaceDialog: React.FC<CreateWorkspaceDialogProps> = ({
  isOpen,
  onClose,
  onSuccess,
}) => {
  const navigate = useNavigate();
  const { success, error: showError } = useToast();
  const [isSubmitting, setIsSubmitting] = useState(false);
  const [formData, setFormData] = useState<WorkspaceFormData | null>(null);
  const [isLoadingFormData, setIsLoadingFormData] = useState(false);
  const [errors, setErrors] = useState<Record<string, string>>({});

  const [workspaceData, setWorkspaceData] = useState<CreateWorkspaceRequest>({
    name: '',
    description: '',
    execution_mode: 'local',
    state_backend: 'local',
    auto_apply: false,
    plan_only: false,
    terraform_version: 'latest',
    workdir: '/workspace',
  });

  // 加载表单数据
  useEffect(() => {
    if (isOpen && !formData) {
      loadFormData();
    }
  }, [isOpen]);

  // 从localStorage恢复表单数据
  useEffect(() => {
    if (isOpen) {
      const saved = localStorage.getItem(STORAGE_KEY);
      if (saved) {
        try {
          const parsed = JSON.parse(saved);
          setWorkspaceData(parsed);
        } catch (e) {
          console.error('Failed to parse saved form data:', e);
        }
      }
    }
  }, [isOpen]);

  // 保存表单数据到localStorage
  useEffect(() => {
    if (workspaceData.name || workspaceData.description) {
      localStorage.setItem(STORAGE_KEY, JSON.stringify(workspaceData));
    }
  }, [workspaceData]);

  const loadFormData = async () => {
    setIsLoadingFormData(true);
    try {
      const response = await workspaceService.getFormData();
      setFormData(response.data);
    } catch (err) {
      logError('加载表单数据', err);
      showError('加载表单数据失败: ' + extractErrorMessage(err));
    } finally {
      setIsLoadingFormData(false);
    }
  };

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

  const validateForm = (): boolean => {
    const newErrors: Record<string, string> = {};

    // 名称验证
    if (!workspaceData.name.trim()) {
      newErrors.name = '名称不能为空';
    } else if (!/^[a-zA-Z0-9-_]+$/.test(workspaceData.name)) {
      newErrors.name = '名称只能包含字母、数字、横线和下划线';
    } else if (workspaceData.name.length < 3) {
      newErrors.name = '名称至少需要3个字符';
    } else if (workspaceData.name.length > 50) {
      newErrors.name = '名称不能超过50个字符';
    }

    // Agent模式验证
    if (workspaceData.execution_mode === 'agent' && !workspaceData.agent_pool_id) {
      newErrors.agent_pool_id = '请选择Agent Pool';
    }

    // K8s模式验证
    if (workspaceData.execution_mode === 'k8s' && !workspaceData.k8s_config_id) {
      newErrors.k8s_config_id = '请选择K8s配置';
    }

    // State后端验证
    if (!workspaceData.state_backend) {
      newErrors.state_backend = '请选择State后端';
    }

    setErrors(newErrors);
    return Object.keys(newErrors).length === 0;
  };

  const handleConfirm = async () => {
    if (!validateForm()) {
      return; // 验证失败，保留用户输入
    }

    setIsSubmitting(true);

    try {
      const response = await workspaceService.createWorkspace(workspaceData);
      
      // 成功通知
      success('Workspace创建成功');
      
      // 清理localStorage
      localStorage.removeItem(STORAGE_KEY);
      
      // 关闭弹窗
      onClose();
      
      // 重置表单
      setWorkspaceData({
        name: '',
        description: '',
        execution_mode: 'local',
        state_backend: 'local',
        auto_apply: false,
        plan_only: false,
        terraform_version: 'latest',
        workdir: '/workspace',
      });
      setErrors({});
      
      // 回调
      if (onSuccess) {
        onSuccess();
      }
      
      // 跳转到详情页
      navigate(`/workspaces/${response.data.id}`);
      
    } catch (err: any) {
      logError('创建Workspace', err);
      showError('创建失败: ' + extractErrorMessage(err));
      // 保留用户输入，不清空表单
    } finally {
      setIsSubmitting(false);
    }
  };

  const handleClose = () => {
    // 关闭时不清空表单，保留用户输入
    onClose();
  };

  if (isLoadingFormData) {
    return (
      <ConfirmDialog
        isOpen={isOpen}
        onClose={handleClose}
        onConfirm={() => {}}
        title="创建Workspace"
        confirmText="创建"
        cancelText="取消"
        confirmDisabled={true}
      >
        <div className={styles.loading}>
          <div className={styles.spinner}></div>
          <p>加载表单数据中...</p>
        </div>
      </ConfirmDialog>
    );
  }

  return (
    <ConfirmDialog
      isOpen={isOpen}
      onClose={handleClose}
      onConfirm={handleConfirm}
      title="创建Workspace"
      confirmText={isSubmitting ? '创建中...' : '创建'}
      cancelText="取消"
      confirmDisabled={isSubmitting}
    >
      <form className={styles.form} onSubmit={(e) => e.preventDefault()}>
        {/* 基础信息 */}
        <div className={styles.section}>
          <h3 className={styles.sectionTitle}>基础信息</h3>
          
          <div className={styles.field}>
            <label className={styles.label}>
              名称 <span className={styles.required}>*</span>
            </label>
            <input
              type="text"
              value={workspaceData.name}
              onChange={(e) => handleFieldChange('name', e.target.value)}
              className={`${styles.input} ${errors.name ? styles.inputError : ''}`}
              placeholder="例如: production-infra"
              disabled={isSubmitting}
            />
            {errors.name && (
              <div className={styles.error}>{errors.name}</div>
            )}
            <div className={styles.hint}>
              只能包含字母、数字、横线和下划线，3-50个字符
            </div>
          </div>

          <div className={styles.field}>
            <label className={styles.label}>描述</label>
            <textarea
              value={workspaceData.description}
              onChange={(e) => handleFieldChange('description', e.target.value)}
              className={styles.textarea}
              placeholder="描述此Workspace的用途"
              rows={3}
              disabled={isSubmitting}
            />
          </div>
        </div>

        {/* 执行配置 */}
        <div className={styles.section}>
          <h3 className={styles.sectionTitle}>执行配置</h3>
          
          <div className={styles.field}>
            <label className={styles.label}>
              执行模式 <span className={styles.required}>*</span>
            </label>
            <select
              value={workspaceData.execution_mode}
              onChange={(e) => handleFieldChange('execution_mode', e.target.value as 'local' | 'agent' | 'k8s')}
              className={styles.select}
              disabled={isSubmitting}
            >
              {formData?.execution_modes.map(mode => (
                <option key={mode.value} value={mode.value}>
                  {mode.label}
                </option>
              ))}
            </select>
            {formData?.execution_modes.find(m => m.value === workspaceData.execution_mode)?.description && (
              <div className={styles.hint}>
                {formData.execution_modes.find(m => m.value === workspaceData.execution_mode)?.description}
              </div>
            )}
          </div>

          {/* Agent Pool选择 */}
          {workspaceData.execution_mode === 'agent' && (
            <div className={styles.field}>
              <label className={styles.label}>
                Agent Pool <span className={styles.required}>*</span>
              </label>
              <select
                value={workspaceData.agent_pool_id || ''}
                onChange={(e) => handleFieldChange('agent_pool_id', e.target.value ? Number(e.target.value) : undefined)}
                className={`${styles.select} ${errors.agent_pool_id ? styles.inputError : ''}`}
                disabled={isSubmitting}
              >
                <option value="">选择Agent Pool</option>
                {formData?.agent_pools.map(pool => (
                  <option key={pool.id} value={pool.id}>
                    {pool.name}
                  </option>
                ))}
              </select>
              {errors.agent_pool_id && (
                <div className={styles.error}>{errors.agent_pool_id}</div>
              )}
            </div>
          )}

          {/* K8s配置选择 */}
          {workspaceData.execution_mode === 'k8s' && (
            <div className={styles.field}>
              <label className={styles.label}>
                K8s配置 <span className={styles.required}>*</span>
              </label>
              <select
                value={workspaceData.k8s_config_id || ''}
                onChange={(e) => handleFieldChange('k8s_config_id', e.target.value ? Number(e.target.value) : undefined)}
                className={`${styles.select} ${errors.k8s_config_id ? styles.inputError : ''}`}
                disabled={isSubmitting}
              >
                <option value="">选择K8s配置</option>
                {formData?.k8s_configs.map(config => (
                  <option key={config.id} value={config.id}>
                    {config.name}
                  </option>
                ))}
              </select>
              {errors.k8s_config_id && (
                <div className={styles.error}>{errors.k8s_config_id}</div>
              )}
              {formData?.k8s_configs.length === 0 && (
                <div className={styles.hint}>
                  暂无可用的K8s配置，请先在管理页面添加
                </div>
              )}
            </div>
          )}

          <div className={styles.field}>
            <label className={styles.label}>Terraform版本</label>
            <select
              value={workspaceData.terraform_version}
              onChange={(e) => handleFieldChange('terraform_version', e.target.value)}
              className={styles.select}
              disabled={isSubmitting}
            >
              {formData?.terraform_versions.map(version => (
                <option key={version} value={version}>
                  {version}
                </option>
              ))}
            </select>
          </div>

          <div className={styles.field}>
            <label className={styles.label}>
              State后端 <span className={styles.required}>*</span>
            </label>
            <select
              value={workspaceData.state_backend}
              onChange={(e) => handleFieldChange('state_backend', e.target.value)}
              className={`${styles.select} ${errors.state_backend ? styles.inputError : ''}`}
              disabled={isSubmitting}
            >
              {formData?.state_backends.map(backend => (
                <option key={backend.value} value={backend.value}>
                  {backend.label}
                </option>
              ))}
            </select>
            {formData?.state_backends.find(b => b.value === workspaceData.state_backend)?.description && (
              <div className={styles.hint}>
                {formData.state_backends.find(b => b.value === workspaceData.state_backend)?.description}
              </div>
            )}
            {errors.state_backend && (
              <div className={styles.error}>{errors.state_backend}</div>
            )}
          </div>

          <div className={styles.field}>
            <label className={styles.label}>工作目录</label>
            <input
              type="text"
              value={workspaceData.workdir}
              onChange={(e) => handleFieldChange('workdir', e.target.value)}
              className={styles.input}
              placeholder="/workspace"
              disabled={isSubmitting}
            />
            <div className={styles.hint}>
              Terraform执行的工作目录
            </div>
          </div>
        </div>

        {/* 自动化配置 */}
        <div className={styles.section}>
          <h3 className={styles.sectionTitle}>自动化配置</h3>
          
          <div className={styles.field}>
            <label className={styles.switchLabel}>
              <input
                type="checkbox"
                checked={workspaceData.auto_apply}
                onChange={(e) => handleFieldChange('auto_apply', e.target.checked)}
                className={styles.checkbox}
                disabled={isSubmitting}
              />
              <span>自动Apply</span>
            </label>
            <div className={styles.hint}>
              Plan成功后自动执行Apply
            </div>
          </div>

          <div className={styles.field}>
            <label className={styles.switchLabel}>
              <input
                type="checkbox"
                checked={workspaceData.plan_only}
                onChange={(e) => handleFieldChange('plan_only', e.target.checked)}
                className={styles.checkbox}
                disabled={isSubmitting}
              />
              <span>仅Plan模式</span>
            </label>
            <div className={styles.hint}>
              只执行Plan，不允许Apply（用于只读审查）
            </div>
          </div>
        </div>
      </form>
    </ConfirmDialog>
  );
};

export default CreateWorkspaceDialog;

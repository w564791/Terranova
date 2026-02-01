import React, { useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { useToast } from '../contexts/ToastContext';
import { extractErrorMessage } from '../utils/errorHandler';
import api from '../services/api';
import styles from './NewRunDialog.module.css';

interface ResourceRunDialogProps {
  isOpen: boolean;
  workspaceId: string;
  resourceName: string;
  resourceType?: string;
  onClose: () => void;
  onSuccess?: () => void;
}

type RunType = 'plan' | 'plan_and_apply';

const ResourceRunDialog: React.FC<ResourceRunDialogProps> = ({
  isOpen,
  workspaceId,
  resourceName,
  resourceType,
  onClose,
  onSuccess
}) => {
  const navigate = useNavigate();
  const { showToast } = useToast();
  const [runType, setRunType] = useState<RunType>('plan');
  const [description, setDescription] = useState('Custom Run');
  const [loading, setLoading] = useState(false);

  if (!isOpen) return null;

  const handleSubmit = async () => {
    console.log('🚀 开始执行handleSubmit');
    console.log('📝 resourceName:', resourceName);
    console.log('📝 workspaceId:', workspaceId);
    
    try {
      setLoading(true);
      
      // 1. 先添加或更新TF_CLI_ARGS变量
      // 构建完整的module名称：resource_type_resource_name
      const moduleName = resourceType ? `${resourceType}_${resourceName}` : resourceName;
      const targetValue = `--target=module.${moduleName}`;
      console.log('🎯 目标变量值:', targetValue);
      console.log('📝 resourceType:', resourceType);
      console.log('📝 moduleName:', moduleName);
      
      try {
        // 尝试获取现有变量
        const variablesResponse: any = await api.get(`/workspaces/${workspaceId}/variables`);
        // API返回格式：{code: 200, data: [...], timestamp: "..."}
        const variables = variablesResponse.data?.data || variablesResponse.data || [];
        
        console.log('📋 当前变量列表:', variables);
        console.log('🔍 查找TF_CLI_ARGS变量...');
        
        const existingVar = variables.find((v: any) => v.key === 'TF_CLI_ARGS');
        
        console.log('🔍 找到的变量:', existingVar);
        
        if (existingVar) {
          // 更新现有变量（必须包含version字段）
          await api.put(`/workspaces/${workspaceId}/variables/${existingVar.id}`, {
            version: existingVar.version,  // 添加版本号
            key: 'TF_CLI_ARGS',
            value: targetValue,
            category: 'env',
            variable_type: 'environment',
            sensitive: false,
            description: 'Auto-generated for resource-specific run'
          });
          console.log(' TF_CLI_ARGS变量已更新');
        } else {
          // 创建新变量
          try {
            await api.post(`/workspaces/${workspaceId}/variables`, {
              key: 'TF_CLI_ARGS',
              value: targetValue,
              category: 'env',
              variable_type: 'environment',
              sensitive: false,
              description: 'Auto-generated for resource-specific run'
            });
            console.log(' TF_CLI_ARGS变量已创建');
          } catch (createError: any) {
            console.log('❌ [ViewResource] 创建变量失败:', createError);
            
            // 如果创建失败是因为变量已存在，尝试重新获取并更新
            const errorMessage = createError?.response?.data?.message || createError?.message || '';
            console.log('🔍 [ViewResource] 最终错误消息:', errorMessage);
            if (errorMessage.includes('已存在') || errorMessage.includes('exist')) {
              console.log('🔄 [ViewResource] 检测到变量已存在，尝试重新获取并更新...');
              const retryResponse: any = await api.get(`/workspaces/${workspaceId}/variables`);
              const retryVariables = retryResponse.data?.data || retryResponse.data || [];
              console.log('🔄 [ViewResource] 重试获取到的变量列表:', retryVariables);
              const retryExistingVar = retryVariables.find((v: any) => v.key === 'TF_CLI_ARGS');
              
              if (retryExistingVar) {
                await api.put(`/workspaces/${workspaceId}/variables/${retryExistingVar.id}`, {
                  version: retryExistingVar.version,  // 添加版本号
                  key: 'TF_CLI_ARGS',
                  value: targetValue,
                  category: 'env',
                  variable_type: 'environment',
                  sensitive: false,
                  description: 'Auto-generated for resource-specific run'
                });
                console.log(' [ViewResource] TF_CLI_ARGS变量已更新（重试成功）');
              } else {
                console.log(' [ViewResource] 重试时仍未找到变量，尝试通过key查询...');
                try {
                  const singleVarResponse: any = await api.get(`/workspaces/${workspaceId}/variables/by-key/TF_CLI_ARGS`);
                  const singleVar = singleVarResponse.data?.variable || singleVarResponse.variable || singleVarResponse;
                  
                  if (singleVar && singleVar.id) {
                    await api.put(`/workspaces/${workspaceId}/variables/${singleVar.id}`, {
                      version: singleVar.version,  // 添加版本号
                      key: 'TF_CLI_ARGS',
                      value: targetValue,
                      category: 'env',
                      variable_type: 'environment',
                      sensitive: false,
                      description: 'Auto-generated for resource-specific run'
                    });
                    console.log(' [ViewResource] TF_CLI_ARGS变量已更新（通过key查询成功）');
                  } else {
                    console.warn(' [ViewResource] 无法找到变量但后端说已存在，忽略此错误继续执行');
                  }
                } catch (queryError) {
                  console.warn(' [ViewResource] 通过key查询变量也失败，忽略此错误继续执行:', queryError);
                }
              }
            } else {
              throw createError;
            }
          }
        }
      } catch (varError) {
        console.error('设置TF_CLI_ARGS变量失败:', varError);
        showToast('设置运行参数失败', 'error');
        return;
      }
      
      // 2. 创建Plan任务
      const response: any = await api.post(`/workspaces/${workspaceId}/tasks/plan`, {
        description: description.trim() || undefined,
        run_type: runType
      });
      
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
      
      // 跳转到任务详情页
      if (taskId) {
        navigate(`/workspaces/${workspaceId}/tasks/${taskId}`);
      } else {
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

          {/* 显示将要设置的变量信息 */}
          <div style={{
            padding: '12px 16px',
            background: '#f0f9ff',
            border: '1px solid #bae6fd',
            borderRadius: '8px',
            marginBottom: '20px'
          }}>
            <div style={{ fontSize: '13px', color: '#0369a1', marginBottom: '4px', fontWeight: 500 }}>
              运行参数
            </div>
            <div style={{ fontSize: '13px', color: '#075985' }}>
              将自动设置环境变量: <code style={{ 
                background: '#e0f2fe', 
                padding: '2px 6px', 
                borderRadius: '4px',
                fontFamily: 'monospace'
              }}>TF_CLI_ARGS=--target=module.{resourceName}</code>
            </div>
            <div style={{ fontSize: '12px', color: '#0c4a6e', marginTop: '4px' }}>
              此运行将只针对资源 <strong>{resourceName}</strong> 执行
            </div>
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

export default ResourceRunDialog;

import React, { useState, useEffect } from 'react';
import { useToast } from '../contexts/ToastContext';
import { extractErrorMessage } from '../utils/errorHandler';
import { processApiSchema } from '../utils/schemaTypeMapper';
import api from '../services/api';
import DynamicForm, { type FormSchema } from './DynamicForm';
import styles from './EditResourceDialog.module.css';

interface WorkspaceResource {
  id: number;
  workspace_id: number;
  resource_id: string;
  resource_type: string;
  resource_name: string;
  current_version_id?: number;
  current_version?: {
    id: number;
    version: number;
    tf_code: any;
    variables?: any;
    change_summary: string;
  };
  description?: string;
  is_active: boolean;
}

interface EditResourceDialogProps {
  resource: WorkspaceResource;
  onSave: () => void;
  onClose: () => void;
}

const EditResourceDialog: React.FC<EditResourceDialogProps> = ({
  resource,
  onSave,
  onClose
}) => {
  const { showToast } = useToast();
  const [formData, setFormData] = useState<any>({});
  const [changeSummary, setChangeSummary] = useState('');
  const [loading, setLoading] = useState(false);
  const [schemaLoading, setSchemaLoading] = useState(true);
  const [schema, setSchema] = useState<FormSchema | null>(null);
  const [changeSummaryError, setChangeSummaryError] = useState('');
  const [initialFieldsToShow, setInitialFieldsToShow] = useState<string[]>([]);

  useEffect(() => {
    loadResourceSchema();
  }, [resource]);

  // 监控formData变化
  useEffect(() => {
    console.log(' formData updated:', formData);
  }, [formData]);

  const loadResourceSchema = async () => {
    try {
      setSchemaLoading(true);
      
      // 从resource的tf_code中提取module配置
      const tfCode = resource.current_version?.tf_code || {};
      console.log('Resource TF Code:', tfCode);
      
      // 提取module块
      let moduleConfig = null;
      let moduleSource = '';
      
      if (tfCode.module) {
        // 找到第一个module配置
        const moduleKeys = Object.keys(tfCode.module);
        if (moduleKeys.length > 0) {
          const moduleKey = moduleKeys[0];
          const moduleArray = tfCode.module[moduleKey];
          if (Array.isArray(moduleArray) && moduleArray.length > 0) {
            moduleConfig = moduleArray[0];
            moduleSource = moduleConfig.source;
          }
        }
      }
      
      if (!moduleSource) {
        showToast('无法获取Module信息', 'error');
        setSchemaLoading(false);
        return;
      }
      
      // 根据source查找对应的module
      const modulesResponse = await api.get('/modules');
      const modules = modulesResponse.data.items || [];
      
      // 匹配module（通过module_source或source字段）
      const matchedModule = modules.find((m: any) => 
        m.module_source === moduleSource || m.source === moduleSource
      );
      
      if (!matchedModule) {
        showToast('找不到对应的Module', 'error');
        setSchemaLoading(false);
        return;
      }
      
      // 加载module的schema
      const schemaResponse = await api.get(`/modules/${matchedModule.id}/schemas`);
      console.log('Schema API Response:', schemaResponse.data);
      
      let schemasData = [];
      if (schemaResponse.data.data) {
        schemasData = Array.isArray(schemaResponse.data.data) 
          ? schemaResponse.data.data 
          : [schemaResponse.data.data];
      } else if (Array.isArray(schemaResponse.data)) {
        schemasData = schemaResponse.data;
      }
      
      if (schemasData.length > 0) {
        let activeSchema = schemasData.find((s: any) => s.status === 'active') || schemasData[0];
        
        // 解析schema_data
        if (typeof activeSchema.schema_data === 'string') {
          try {
            activeSchema.schema_data = JSON.parse(activeSchema.schema_data);
          } catch (e) {
            console.error('Schema解析错误:', e);
            activeSchema.schema_data = {};
          }
        }
        
        // 处理schema类型
        const processedSchema = processApiSchema(activeSchema);
        setSchema(processedSchema.schema_data);
        
        // 从module配置中提取表单数据（排除source字段）
        if (moduleConfig) {
          const { source, ...configData } = moduleConfig;
          console.log('📝 Extracted form data:', configData);
          console.log('📝 Module config:', moduleConfig);
          
          // 找出所有有值的字段名
          const fieldsWithValues = Object.keys(configData).filter(key => {
            const value = configData[key];
            // 检查值是否非空
            if (value === null || value === undefined || value === '') return false;
            if (Array.isArray(value) && value.length === 0) return false;
            if (typeof value === 'object' && Object.keys(value).length === 0) return false;
            return true;
          });
          
          console.log('🔑 Fields with values:', fieldsWithValues);
          setInitialFieldsToShow(fieldsWithValues);
          setFormData(configData);
        }
      } else {
        showToast('该Module暂无Schema定义', 'warning');
      }
    } catch (error: any) {
      console.error('加载Schema失败:', error);
      showToast(extractErrorMessage(error), 'error');
    } finally {
      setSchemaLoading(false);
    }
  };

  const handleSave = async () => {
    // 验证变更摘要
    if (!changeSummary.trim()) {
      setChangeSummaryError('请输入变更摘要');
      showToast('请输入变更摘要', 'warning');
      return;
    }
    
    setLoading(true);
    try {
      // 从原始tf_code中提取module source
      const tfCode = resource.current_version?.tf_code || {};
      let moduleSource = '';
      
      if (tfCode.module) {
        const moduleKeys = Object.keys(tfCode.module);
        if (moduleKeys.length > 0) {
          const moduleKey = moduleKeys[0];
          const moduleArray = tfCode.module[moduleKey];
          if (Array.isArray(moduleArray) && moduleArray.length > 0) {
            moduleSource = moduleArray[0].source;
          }
        }
      }
      
      // 构建新的tf_code（保持原有结构）
      const updatedTFCode = {
        module: {
          [`${resource.resource_type}_${resource.resource_name}`]: [
            {
              source: moduleSource,
              ...formData
            }
          ]
        }
      };
      
      // 调用更新API
      await api.put(`/workspaces/${resource.workspace_id}/resources/${resource.id}`, {
        tf_code: updatedTFCode,
        variables: resource.current_version?.variables || {},
        change_summary: changeSummary.trim()
      });
      
      showToast('资源更新成功', 'success');
      onSave();
      onClose();
    } catch (error: any) {
      showToast(extractErrorMessage(error), 'error');
      // 保留用户输入，不清空表单
    } finally {
      setLoading(false);
    }
  };

  const handleChangeSummaryChange = (value: string) => {
    setChangeSummary(value);
    if (changeSummaryError && value.trim()) {
      setChangeSummaryError('');
    }
  };

  const handleOverlayClick = (e: React.MouseEvent) => {
    if (e.target === e.currentTarget) {
      onClose();
    }
  };

  return (
    <div className={styles.dialogOverlay} onClick={handleOverlayClick}>
      <div className={styles.dialogContainer}>
        {/* Header */}
        <div className={styles.dialogHeader}>
          <div className={styles.headerContent}>
            <h2 className={styles.dialogTitle}>编辑资源</h2>
            <div className={styles.resourceInfo}>
              <span className={styles.resourceType}>{resource.resource_type}</span>
              <span className={styles.resourceSeparator}>·</span>
              <span className={styles.resourceName}>{resource.resource_name}</span>
            </div>
          </div>
          <button 
            className={styles.closeButton}
            onClick={onClose}
            aria-label="关闭"
          >
            ×
          </button>
        </div>
        
        {/* Content */}
        <div className={styles.dialogContent}>
          {schemaLoading ? (
            <div className={styles.loadingState}>
              <div className={styles.spinner}></div>
              <p>加载Schema中...</p>
            </div>
          ) : schema ? (
            <>
              {/* Dynamic Form */}
              <div className={styles.formSection}>
                <DynamicForm
                  schema={schema}
                  values={formData}
                  onChange={setFormData}
                  initialFieldsToShow={initialFieldsToShow}
                />
              </div>
              
              {/* Change Summary */}
              <div className={styles.changeSummarySection}>
                <label className={styles.changeSummaryLabel}>
                  变更摘要 <span className={styles.required}>*</span>
                </label>
                <input
                  type="text"
                  placeholder="描述本次修改的内容，例如：更新bucket配置、启用版本控制等"
                  value={changeSummary}
                  onChange={(e) => handleChangeSummaryChange(e.target.value)}
                  className={`${styles.changeSummaryInput} ${changeSummaryError ? styles.inputError : ''}`}
                  disabled={loading}
                />
                {changeSummaryError && (
                  <div className={styles.errorMessage}>{changeSummaryError}</div>
                )}
                <div className={styles.changeSummaryHint}>
                  变更摘要将记录在版本历史中，帮助团队了解每次修改的目的
                </div>
              </div>
            </>
          ) : (
            <div className={styles.emptyState}>
              <p>该Module暂无Schema定义</p>
              <p className={styles.emptyStateHint}>请先在Module管理页面生成Schema</p>
            </div>
          )}
        </div>
        
        {/* Footer */}
        <div className={styles.dialogFooter}>
          <button 
            onClick={onClose} 
            className={styles.btnCancel}
            disabled={loading}
          >
            取消
          </button>
          <button 
            onClick={handleSave} 
            className={styles.btnPrimary}
            disabled={loading || schemaLoading || !schema}
          >
            {loading ? (
              <>
                <span className={styles.btnSpinner}></span>
                保存中...
              </>
            ) : (
              '保存修改'
            )}
          </button>
        </div>
      </div>
    </div>
  );
};

export default EditResourceDialog;

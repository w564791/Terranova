import React, { useState, useEffect } from 'react';
import { useParams } from 'react-router-dom';
import { useToast } from '../contexts/ToastContext';
import { extractErrorMessage } from '../utils/errorHandler';
import api from '../services/api';
import ConfirmDialog from '../components/ConfirmDialog';
import styles from './ProviderSettings.module.css';

interface ProviderConfig {
  type: string;
  alias?: string;
  authMethod: 'iam_role' | 'aksk' | 'assume_role';
  region: string;
  accessKey?: string;
  secretKey?: string;
  roleArn?: string;
  versionConstraint?: '~>' | '>=' | '>' | '=' | '<=' | '<';
  version?: string;
  advancedParams?: Record<string, any>;
}

interface ProviderSettingsProps {
  workspaceId: string;
}

const ProviderSettings: React.FC<ProviderSettingsProps> = ({ workspaceId }) => {
  const { showToast } = useToast();
  const [providers, setProviders] = useState<ProviderConfig[]>([]);
  const [loading, setLoading] = useState(true);
  const [hasChanges, setHasChanges] = useState(false);
  const [editingIndex, setEditingIndex] = useState<number | null>(null);
  const [showForm, setShowForm] = useState(false);
  const [deleteDialogOpen, setDeleteDialogOpen] = useState(false);
  const [providerToDelete, setProviderToDelete] = useState<number | null>(null);

  useEffect(() => {
    fetchProviderConfig();
  }, [workspaceId]);

  const fetchProviderConfig = async () => {
    try {
      setLoading(true);
      const response = await api.get(`/workspaces/${workspaceId}`);
      
      // API响应格式: { code: 200, data: { ...workspace }, timestamp: ... }
      // api.ts的拦截器返回response.data，所以这里response就是{ code: 200, data: {...}, timestamp: ... }
      const workspace = response.data || response;
      
      console.log('Fetched workspace:', workspace);
      console.log('Fetched workspace provider_config:', workspace.provider_config);
      
      if (workspace.provider_config) {
        const config = workspace.provider_config;
        const providerList: ProviderConfig[] = [];
        
        if (config.provider) {
          Object.entries(config.provider).forEach(([type, configs]: [string, any]) => {
            if (Array.isArray(configs)) {
              configs.forEach((cfg: any) => {
                const parsed = parseProviderConfig(type, cfg, config.terraform);
                console.log('Parsed provider config:', parsed);
                providerList.push(parsed);
              });
            }
          });
        }
        
        console.log('Total providers loaded:', providerList.length);
        setProviders(providerList);
      } else {
        // 如果没有provider_config，清空列表
        console.log('No provider_config found, clearing list');
        setProviders([]);
      }
    } catch (error) {
      console.error('Failed to fetch provider config:', error);
      showToast(extractErrorMessage(error), 'error');
    } finally {
      setLoading(false);
    }
  };

  const parseProviderConfig = (type: string, config: any, terraformConfig: any): ProviderConfig => {
    const provider: ProviderConfig = {
      type,
      alias: config.alias,
      region: config.region,
      authMethod: 'iam_role',
      advancedParams: {}
    };

    // 判断认证方式
    if (config.access_key && config.secret_key) {
      provider.authMethod = 'aksk';
      provider.accessKey = config.access_key;
      provider.secretKey = config.secret_key;
    } else if (config.assume_role) {
      provider.authMethod = 'assume_role';
      provider.roleArn = config.assume_role[0]?.role_arn;
    }

    // 提取版本信息
    if (terraformConfig && Array.isArray(terraformConfig)) {
      terraformConfig.forEach((tf: any) => {
        if (tf.required_providers && Array.isArray(tf.required_providers)) {
          tf.required_providers.forEach((rp: any) => {
            if (rp[type]) {
              const versionStr = rp[type].version || '';
              const match = versionStr.match(/^([~><=]+)\s*(.+)$/);
              if (match) {
                provider.versionConstraint = match[1] as any;
                provider.version = match[2];
              }
            }
          });
        }
      });
    }

    // 提取高级参数
    const standardFields = ['alias', 'region', 'access_key', 'secret_key', 'assume_role'];
    Object.entries(config).forEach(([key, value]) => {
      if (!standardFields.includes(key)) {
        provider.advancedParams![key] = value;
      }
    });

    return provider;
  };

  const handleAddProvider = () => {
    setEditingIndex(null);
    setShowForm(true);
  };

  const handleEdit = (index: number) => {
    setEditingIndex(index);
    setShowForm(true);
  };

  const handleDeleteClick = (index: number) => {
    setProviderToDelete(index);
    setDeleteDialogOpen(true);
  };

  const handleDeleteConfirm = () => {
    if (providerToDelete !== null) {
      const newProviders = providers.filter((_, i) => i !== providerToDelete);
      setProviders(newProviders);
      setHasChanges(true);
      setDeleteDialogOpen(false);
      setProviderToDelete(null);
      showToast('Provider已删除（未保存）', 'info');
    }
  };

  const handleSaveProvider = (provider: ProviderConfig) => {
    if (editingIndex !== null) {
      // 更新
      const newProviders = [...providers];
      newProviders[editingIndex] = provider;
      setProviders(newProviders);
      showToast('Provider已更新（未保存）', 'info');
    } else {
      // 添加
      setProviders([...providers, provider]);
      showToast('Provider已添加（未保存）', 'info');
    }
    
    setHasChanges(true);
    setShowForm(false);
    setEditingIndex(null);
  };

  const handleCancel = () => {
    setShowForm(false);
    setEditingIndex(null);
  };

  const buildSaveData = () => {
    const providerMap: Record<string, any[]> = {};
    const requiredProviders: any = {};
    
    providers.forEach(p => {
      if (!providerMap[p.type]) {
        providerMap[p.type] = [];
      }
      
      const config: any = {
        region: p.region,
        ...p.advancedParams
      };
      
      if (p.alias) {
        config.alias = p.alias;
      }
      
      if (p.authMethod === 'aksk') {
        config.access_key = p.accessKey;
        config.secret_key = p.secretKey;
      } else if (p.authMethod === 'assume_role' && p.roleArn) {
        config.assume_role = [{ role_arn: p.roleArn }];
      }
      
      providerMap[p.type].push(config);
      
      // 构建版本约束
      if (p.version) {
        const constraint = p.versionConstraint || '~>';
        // 如果是精确版本（=），不需要添加约束符号
        const versionStr = constraint === '=' ? p.version : `${constraint} ${p.version}`;
        requiredProviders[p.type] = {
          source: `hashicorp/${p.type}`,
          version: versionStr
        };
      }
    });
    
    // 只有在有版本约束时才包含terraform块
    // 空的terraform块会导致Terraform尝试读取backend state，在首次运行时会失败
    const result: any = {
      provider: providerMap
    };
    
    if (Object.keys(requiredProviders).length > 0) {
      result.terraform = [{ required_providers: [requiredProviders] }];
    }
    
    return result;
  };

  const handleSave = async () => {
    try {
      const providerConfig = buildSaveData();
      
      console.log('Saving provider config:', JSON.stringify(providerConfig, null, 2));
      
      // 保存时不设置loading，避免阻塞UI
      await api.patch(`/workspaces/${workspaceId}`, {
        provider_config: providerConfig
      });
      
      showToast('Provider配置已保存', 'success');
      setHasChanges(false);
      
      // 重新加载以确认保存成功
      await fetchProviderConfig();
    } catch (error) {
      console.error('Failed to save provider config:', error);
      showToast(extractErrorMessage(error), 'error');
    }
  };

  if (loading && providers.length === 0) {
    return <div className={styles.loading}>加载中...</div>;
  }

  return (
    <div className={styles.container}>
      {/* 页面标题 */}
      <div className={styles.pageHeader}>
        <h2 className={styles.pageTitle}>Provider Configuration</h2>
        <p className={styles.pageDescription}>
          Configure Terraform providers and their authentication methods. 
          These settings will be used to generate provider.tf.json during execution.
        </p>
      </div>

      {/* Provider列表 */}
      {!showForm && (
        <>
          <div className={styles.providerList}>
            {providers.map((provider, index) => (
              <ProviderCard
                key={index}
                provider={provider}
                onEdit={() => handleEdit(index)}
                onDelete={() => handleDeleteClick(index)}
              />
            ))}
          </div>

          {/* 添加Provider按钮 */}
          <button onClick={handleAddProvider} className={styles.addButton}>
            + Add Provider
          </button>

          {/* 保存按钮 */}
          {providers.length > 0 && (
            <div className={styles.actions}>
              <button 
                onClick={handleSave} 
                className={styles.saveButton}
                disabled={!hasChanges || loading}
              >
                {loading ? 'Saving...' : 'Save Settings'}
              </button>
              {hasChanges && (
                <span className={styles.unsavedHint}>You have unsaved changes</span>
              )}
            </div>
          )}
        </>
      )}

      {/* Provider表单 */}
      {showForm && (
        <ProviderForm
          provider={editingIndex !== null ? providers[editingIndex] : undefined}
          onSave={handleSaveProvider}
          onCancel={handleCancel}
        />
      )}

      {/* 删除确认对话框 */}
      <ConfirmDialog
        isOpen={deleteDialogOpen}
        title={`Delete ${providerToDelete !== null ? providers[providerToDelete]?.type : ''} provider`}
        confirmText="Yes, delete provider"
        cancelText="Cancel"
        onConfirm={handleDeleteConfirm}
        onCancel={() => setDeleteDialogOpen(false)}
        variant="danger"
      >
        <div style={{ marginBottom: '16px' }}>
          <p style={{ margin: '0 0 12px 0', color: 'var(--color-gray-700)', fontSize: '14px', lineHeight: '1.5' }}>
            Deleting this provider configuration will remove it from the workspace. 
            {providerToDelete !== null && providers[providerToDelete]?.alias && (
              <> The alias <strong>{providers[providerToDelete].alias}</strong> will be removed.</>
            )}
          </p>
          <p style={{ margin: 0, color: 'var(--color-gray-700)', fontSize: '14px', lineHeight: '1.5' }}>
            This operation <strong>cannot be undone</strong>. Are you sure?
          </p>
        </div>
      </ConfirmDialog>
    </div>
  );
};

// Provider卡片组件
const ProviderCard: React.FC<{
  provider: ProviderConfig;
  onEdit: () => void;
  onDelete: () => void;
}> = ({ provider, onEdit, onDelete }) => {
  return (
    <div className={styles.providerCard}>
      <div className={styles.cardHeader}>
        <div className={styles.cardTitle}>
          <span className={styles.providerIcon}>☁️</span>
          <span className={styles.providerName}>{provider.type.toUpperCase()}</span>
          {provider.alias && (
            <span className={styles.aliasBadge}>{provider.alias}</span>
          )}
        </div>
        <div className={styles.cardActions}>
          <button onClick={onEdit} className={styles.editButton}>
            Edit
          </button>
          <button onClick={onDelete} className={styles.deleteButton}>
            Delete
          </button>
        </div>
      </div>

      <div className={styles.cardContent}>
        <div className={styles.configRow}>
          <span className={styles.configLabel}>Authentication:</span>
          <span className={styles.configValue}>
            {provider.authMethod === 'iam_role' && 'IAM Role'}
            {provider.authMethod === 'aksk' && 'Access Key / Secret Key'}
            {provider.authMethod === 'assume_role' && 'Assume Role'}
          </span>
        </div>

        {provider.region && (
          <div className={styles.configRow}>
            <span className={styles.configLabel}>Region:</span>
            <span className={styles.configValue}>{provider.region}</span>
          </div>
        )}

        {provider.version && (
          <div className={styles.configRow}>
            <span className={styles.configLabel}>Version:</span>
            <span className={styles.configValue}>
              {provider.versionConstraint} {provider.version}
            </span>
          </div>
        )}

        {provider.advancedParams && Object.keys(provider.advancedParams).length > 0 && (
          <div className={styles.configRow}>
            <span className={styles.configLabel}>Advanced:</span>
            <span className={styles.configValue}>
              {Object.keys(provider.advancedParams).length} parameters
            </span>
          </div>
        )}
      </div>
    </div>
  );
};

// Provider表单组件
const ProviderForm: React.FC<{
  provider?: ProviderConfig;
  onSave: (provider: ProviderConfig) => void;
  onCancel: () => void;
}> = ({ provider, onSave, onCancel }) => {
  const [formData, setFormData] = useState<ProviderConfig>({
    type: provider?.type || 'aws',
    alias: provider?.alias || '',
    authMethod: provider?.authMethod || 'iam_role',
    region: provider?.region || '',
    accessKey: provider?.accessKey || '',
    secretKey: provider?.secretKey || '',
    roleArn: provider?.roleArn || '',
    versionConstraint: provider?.versionConstraint || '~>',
    version: provider?.version || '',
    advancedParams: provider?.advancedParams || {}
  });

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    
    if (!formData.region.trim()) {
      alert('Region is required');
      return;
    }
    
    onSave(formData);
  };

  const [newParamKey, setNewParamKey] = useState('');
  const [newParamValue, setNewParamValue] = useState('');
  const [showAddParam, setShowAddParam] = useState(false);

  const handleAddParam = () => {
    if (!newParamKey.trim()) {
      return;
    }
    
    // 检查key是否已存在
    if (formData.advancedParams && newParamKey.trim() in formData.advancedParams) {
      alert('Parameter name already exists');
      return;
    }
    
    // 解析value
    let parsedValue: any = newParamValue;
    try {
      parsedValue = JSON.parse(newParamValue);
    } catch {
      if (newParamValue === 'true') parsedValue = true;
      else if (newParamValue === 'false') parsedValue = false;
      else if (/^\d+$/.test(newParamValue)) parsedValue = parseInt(newParamValue, 10);
      else if (/^\d+\.\d+$/.test(newParamValue)) parsedValue = parseFloat(newParamValue);
    }
    
    setFormData({
      ...formData,
      advancedParams: {
        ...formData.advancedParams,
        [newParamKey.trim()]: parsedValue
      }
    });
    
    // 清空输入并关闭添加表单，显示"Add Parameter"按钮
    setNewParamKey('');
    setNewParamValue('');
    setShowAddParam(false);
  };

  const handleCancelAddParam = () => {
    setShowAddParam(false);
    setNewParamKey('');
    setNewParamValue('');
  };

  const handleRemoveParam = (key: string) => {
    const newParams = { ...formData.advancedParams };
    delete newParams[key];
    setFormData({ ...formData, advancedParams: newParams });
  };

  const handleParamValueChange = (key: string, value: string) => {
    let parsedValue: any = value;
    
    // 尝试解析JSON
    try {
      parsedValue = JSON.parse(value);
    } catch {
      // 尝试解析为数字或布尔值
      if (value === 'true') parsedValue = true;
      else if (value === 'false') parsedValue = false;
      else if (/^\d+$/.test(value)) parsedValue = parseInt(value, 10);
      else if (/^\d+\.\d+$/.test(value)) parsedValue = parseFloat(value);
    }
    
    setFormData({
      ...formData,
      advancedParams: {
        ...formData.advancedParams,
        [key]: parsedValue
      }
    });
  };

  return (
    <form onSubmit={handleSubmit} className={styles.providerForm}>
      <h3 className={styles.formTitle}>
        {provider ? 'Edit Provider' : 'Add Provider'}
      </h3>

      {/* Provider类型 */}
      <div className={styles.formSection}>
        <label className={styles.formLabel}>Provider Type</label>
        <select
          value={formData.type}
          onChange={(e) => setFormData({ ...formData, type: e.target.value })}
          className={styles.select}
        >
          <option value="aws">AWS</option>
          <option value="azure" disabled>Azure (Coming Soon)</option>
          <option value="google" disabled>Google Cloud (Coming Soon)</option>
        </select>
      </div>

      {/* Alias */}
      <div className={styles.formSection}>
        <label className={styles.formLabel}>Alias (Optional)</label>
        <input
          type="text"
          value={formData.alias}
          onChange={(e) => setFormData({ ...formData, alias: e.target.value })}
          className={styles.input}
          placeholder="e.g., us-east, production"
        />
        <div className={styles.hint}>
          Use alias when configuring multiple instances of the same provider
        </div>
      </div>

      {/* 认证方式 */}
      <div className={styles.formSection}>
        <label className={styles.formLabel}>Authentication Method</label>
        <div className={styles.radioGroup}>
          <label className={styles.radioLabel}>
            <input
              type="radio"
              value="iam_role"
              checked={formData.authMethod === 'iam_role'}
              onChange={(e) => setFormData({ ...formData, authMethod: e.target.value as any })}
            />
            <div>
              <strong>IAM Role</strong>
              <p>Use IAM role attached to EC2 instance or ECS task (recommended)</p>
            </div>
          </label>

          <label className={styles.radioLabel}>
            <input
              type="radio"
              value="aksk"
              checked={formData.authMethod === 'aksk'}
              onChange={(e) => setFormData({ ...formData, authMethod: e.target.value as any })}
            />
            <div>
              <strong>Access Key / Secret Key</strong>
              <p>Use static credentials (not recommended for production)</p>
            </div>
          </label>

          <label className={styles.radioLabel}>
            <input
              type="radio"
              value="assume_role"
              checked={formData.authMethod === 'assume_role'}
              onChange={(e) => setFormData({ ...formData, authMethod: e.target.value as any })}
            />
            <div>
              <strong>Assume Role</strong>
              <p>Assume a role in another AWS account</p>
            </div>
          </label>
        </div>
      </div>

      {/* Region */}
      <div className={styles.formSection}>
        <label className={styles.formLabel}>Region *</label>
        <input
          type="text"
          value={formData.region}
          onChange={(e) => setFormData({ ...formData, region: e.target.value })}
          className={styles.input}
          placeholder="e.g., us-east-1, ap-northeast-1"
          required
        />
      </div>

      {/* AKSK字段 */}
      {formData.authMethod === 'aksk' && (
        <>
          <div className={styles.formSection}>
            <label className={styles.formLabel}>Access Key *</label>
            <input
              type="text"
              value={formData.accessKey}
              onChange={(e) => setFormData({ ...formData, accessKey: e.target.value })}
              className={styles.input}
              placeholder="AKIAIOSFODNN7EXAMPLE"
              required
            />
          </div>

          <div className={styles.formSection}>
            <label className={styles.formLabel}>Secret Key *</label>
            <input
              type="password"
              value={formData.secretKey}
              onChange={(e) => setFormData({ ...formData, secretKey: e.target.value })}
              className={styles.input}
              placeholder="wJalrXUtnFEMI/K7MDENG..."
              required
            />
            <div className={styles.warning}>
              <span className={styles.warningIcon}></span>
              <span>Secret key will be stored in database. Consider using IAM role instead.</span>
            </div>
          </div>
        </>
      )}

      {/* Assume Role字段 */}
      {formData.authMethod === 'assume_role' && (
        <div className={styles.formSection}>
          <label className={styles.formLabel}>Role ARN *</label>
          <input
            type="text"
            value={formData.roleArn}
            onChange={(e) => setFormData({ ...formData, roleArn: e.target.value })}
            className={styles.input}
            placeholder="arn:aws:iam::123456789012:role/TerraformRole"
            required
          />
        </div>
      )}

      {/* 版本约束 */}
      <div className={styles.formSection}>
        <label className={styles.formLabel}>Version Constraint (Optional)</label>
        <div className={styles.versionConstraint}>
          <select
            value={formData.versionConstraint}
            onChange={(e) => setFormData({ ...formData, versionConstraint: e.target.value as any })}
            className={styles.constraintSelect}
          >
            <option value="~>">{'~>'} (Pessimistic)</option>
            <option value=">=">{'>='} (Greater or equal)</option>
            <option value=">">{'>'}  (Greater than)</option>
            <option value="=">=  (Exact)</option>
            <option value="<=">{'<='} (Less or equal)</option>
            <option value="<">{'<'}  (Less than)</option>
          </select>
          <input
            type="text"
            value={formData.version}
            onChange={(e) => setFormData({ ...formData, version: e.target.value })}
            className={styles.versionInput}
            placeholder="6.0"
          />
        </div>
        <div className={styles.hint}>
          Example: ~{'>'}  6.0 means {'>'}= 6.0.0 and {'<'} 7.0.0
        </div>
      </div>

      {/* 高级参数 */}
      <div className={styles.formSection}>
        <label className={styles.formLabel}>Advanced Parameters (Optional)</label>
        <div className={styles.advancedParams}>
          {/* 已有参数列表 - 只读显示，可删除 */}
          {Object.entries(formData.advancedParams || {}).map(([key, value]) => (
            <div key={key} className={styles.paramDisplayRow}>
              <div className={styles.paramKeyDisplay}>{key}</div>
              <div className={styles.paramValueDisplay}>
                {typeof value === 'object' ? JSON.stringify(value) : String(value)}
              </div>
              <button
                type="button"
                onClick={() => handleRemoveParam(key)}
                className={styles.removeParamButton}
                title="Delete parameter"
              >
                ×
              </button>
            </div>
          ))}
          
          {/* 添加新参数表单 */}
          {showAddParam && (
            <div className={styles.addParamForm}>
              <input
                type="text"
                value={newParamKey}
                onChange={(e) => setNewParamKey(e.target.value)}
                onKeyPress={(e) => {
                  if (e.key === 'Enter') {
                    e.preventDefault();
                    // 移动到value输入框
                    const valueInputs = document.querySelectorAll('textarea[placeholder*="value"]');
                    if (valueInputs.length > 0) {
                      (valueInputs[0] as HTMLTextAreaElement).focus();
                    }
                  }
                }}
                className={styles.newParamKey}
                placeholder="parameter name"
                autoFocus
              />
              <textarea
                value={newParamValue}
                onChange={(e) => setNewParamValue(e.target.value)}
                onKeyPress={(e) => {
                  if (e.key === 'Enter' && !e.shiftKey) {
                    e.preventDefault();
                    handleAddParam();
                  }
                }}
                className={styles.newParamValue}
                placeholder="value (string, number, boolean, or JSON)"
                rows={2}
              />
              <div className={styles.addParamActions}>
                <button
                  type="button"
                  onClick={handleAddParam}
                  className={styles.confirmAddButton}
                  disabled={!newParamKey.trim()}
                >
                  Add
                </button>
                <button
                  type="button"
                  onClick={handleCancelAddParam}
                  className={styles.cancelAddButton}
                >
                  Cancel
                </button>
              </div>
            </div>
          )}
          
          {/* Add Parameter按钮 - 只在未显示输入表单时显示 */}
          {!showAddParam && (
            <button
              type="button"
              onClick={() => setShowAddParam(true)}
              className={styles.addParamButton}
            >
              + Add Parameter
            </button>
          )}
        </div>
        <div className={styles.hint}>
          Add any provider-specific parameters. Values can be strings, numbers, booleans, or JSON objects.
        </div>
      </div>

      {/* 表单操作 */}
      <div className={styles.formActions}>
        <button type="submit" className={styles.primaryButton}>
          {provider ? 'Update Provider' : 'Add Provider'}
        </button>
        <button type="button" onClick={onCancel} className={styles.cancelButton}>
          Cancel
        </button>
      </div>
    </form>
  );
};

export default ProviderSettings;

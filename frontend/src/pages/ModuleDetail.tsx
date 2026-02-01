import React, { useState, useEffect } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import { useToast } from '../contexts/ToastContext';
import { extractErrorMessage } from '../utils/errorHandler';
import ConfirmDialog from '../components/ConfirmDialog';
import type { ModuleVersion, CreateModuleVersionRequest } from '../services/moduleVersions';
import { listVersions, createVersion, setDefaultVersion, deleteVersion } from '../services/moduleVersions';
import type { AIPrompt } from '../services/modules';
import { 
  getModuleVersionSkill, 
  generateModuleVersionSkill, 
  updateModuleVersionSkillCustomContent,
  inheritModuleVersionSkill,
  type ModuleVersionSkill 
} from '../services/skill';
import api from '../services/api';
import styles from './ModuleDetail.module.css';

interface Module {
  id: number;
  name: string;
  provider: string;
  source: string;
  module_source?: string;
  version: string;
  description: string;
  status: string;
  ai_prompts?: AIPrompt[];
  created_at: string;
  updated_at: string;
}

// 生成 UUID
const generateUUID = (): string => {
  return 'xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx'.replace(/[xy]/g, function(c) {
    const r = Math.random() * 16 | 0;
    const v = c === 'x' ? r : (r & 0x3 | 0x8);
    return v.toString(16);
  });
};

const ModuleDetail: React.FC = () => {
  const { id } = useParams<{ id: string }>();
  const navigate = useNavigate();
  const { showToast } = useToast();
  const [module, setModule] = useState<Module | null>(null);
  const [loading, setLoading] = useState(true);
  
  // 版本相关状态
  const [versions, setVersions] = useState<ModuleVersion[]>([]);
  const [selectedVersion, setSelectedVersion] = useState<ModuleVersion | null>(null);
  const [showVersionDropdown, setShowVersionDropdown] = useState(false);
  const [showAddVersionForm, setShowAddVersionForm] = useState(false);
  const [createVersionForm, setCreateVersionForm] = useState<CreateModuleVersionRequest>({
    version: '',
    inherit_schema_from: '',
    set_as_default: false,
  });
  const [creatingVersion, setCreatingVersion] = useState(false);
  
  // AI 提示词相关状态
  const [showAddPromptForm, setShowAddPromptForm] = useState(false);
  const [editingPromptId, setEditingPromptId] = useState<string | null>(null);
  const [promptForm, setPromptForm] = useState({ title: '', prompt: '' });
  const [savingPrompt, setSavingPrompt] = useState(false);
  
  // 模块编辑相关状态
  const [showEditModuleForm, setShowEditModuleForm] = useState(false);
  const [moduleForm, setModuleForm] = useState({
    module_source: '',
    description: '',
    version: '',
    branch: ''
  });
  const [savingModule, setSavingModule] = useState(false);
  
  // Module Version Skill 相关状态
  const [versionSkill, setVersionSkill] = useState<ModuleVersionSkill | null>(null);
  const [loadingSkill, setLoadingSkill] = useState(false);
  const [generatingSkill, setGeneratingSkill] = useState(false);
  
  const [confirmDialog, setConfirmDialog] = useState<{
    isOpen: boolean;
    title: string;
    message: string;
    onConfirm: () => void;
  }>({
    isOpen: false,
    title: '',
    message: '',
    onConfirm: () => {}
  });

  useEffect(() => {
    const fetchData = async () => {
      if (!id) return;
      
      try {
        // 先获取模块信息
        const moduleResponse = await api.get(`/modules/${id}`);
        const moduleData = moduleResponse.data;
        setModule(moduleData);
        
        // 初始化模块编辑表单
        setModuleForm({
          module_source: moduleData.module_source || '',
          description: moduleData.description || '',
          version: moduleData.version || '',
          branch: moduleData.branch || 'main'
        });
        
        // 然后获取版本列表
        console.log('[ModuleDetail] Loading versions for module:', moduleData.id);
        const versionsResponse = await listVersions(moduleData.id);
        console.log('[ModuleDetail] Versions response:', versionsResponse);
        
        const items = versionsResponse?.items || [];
        console.log('[ModuleDetail] Version items:', items, 'length:', items.length);
        
        setVersions(items);
        console.log('[ModuleDetail] setVersions called with', items.length, 'items');
        
        const defaultVersion = items.find(v => v.is_default);
        if (defaultVersion) {
          setSelectedVersion(defaultVersion);
        } else if (items.length > 0) {
          setSelectedVersion(items[0]);
        }
      } catch (error) {
        const message = extractErrorMessage(error);
        showToast(message, 'error');
        navigate('/modules');
      } finally {
        setLoading(false);
      }
    };

    fetchData();
  }, [id, navigate, showToast]);

  const handleDelete = () => {
    if (!module) return;

    setConfirmDialog({
      isOpen: true,
      title: '删除模块',
      message: '确定要删除这个模块吗？此操作不可撤销。',
      onConfirm: async () => {
        try {
          await api.delete(`/modules/${module.id}`);
          showToast('模块删除成功', 'success');
          navigate('/modules');
        } catch (error) {
          const message = extractErrorMessage(error);
          showToast(message, 'error');
        } finally {
          setConfirmDialog({ ...confirmDialog, isOpen: false });
        }
      }
    });
  };

  const handleToggleStatus = () => {
    if (!module) return;

    const newStatus = module.status === 'active' ? 'inactive' : 'active';
    const action = newStatus === 'active' ? '启用' : '停用';
    
    setConfirmDialog({
      isOpen: true,
      title: `${action}模块`,
      message: `确定要${action}这个模块吗？`,
      onConfirm: async () => {
        try {
          await api.patch(`/modules/${module.id}`, { status: newStatus });
          setModule({ ...module, status: newStatus });
          showToast(`模块已${action}`, 'success');
        } catch (error) {
          const message = extractErrorMessage(error);
          showToast(message, 'error');
        } finally {
          setConfirmDialog({ ...confirmDialog, isOpen: false });
        }
      }
    });
  };

  const handleSelectVersion = (version: ModuleVersion) => {
    setSelectedVersion(version);
    setShowVersionDropdown(false);
  };

  const handleSetDefaultVersion = async (version: ModuleVersion) => {
    if (!module) return;
    try {
      await setDefaultVersion(module.id, version.id);
      showToast('默认版本设置成功', 'success');
      // 重新加载版本列表
      const response = await listVersions(module.id);
      setVersions(response.items || []);
      const defaultVersion = response.items?.find(v => v.is_default);
      if (defaultVersion) setSelectedVersion(defaultVersion);
    } catch (error) {
      showToast(extractErrorMessage(error), 'error');
    }
  };

  const handleDeleteVersion = async (version: ModuleVersion) => {
    if (!module) return;
    
    setConfirmDialog({
      isOpen: true,
      title: '删除版本',
      message: `确定要删除版本 ${version.version} 吗？此操作不可撤销。`,
      onConfirm: async () => {
        try {
          await deleteVersion(module.id, version.id);
          showToast(`版本 ${version.version} 删除成功`, 'success');
          // 重新加载版本列表
          const response = await listVersions(module.id);
          setVersions(response.items || []);
          // 如果删除的是当前选中的版本，切换到默认版本
          if (selectedVersion?.id === version.id) {
            const defaultVersion = response.items?.find(v => v.is_default);
            if (defaultVersion) {
              setSelectedVersion(defaultVersion);
            } else if (response.items?.length > 0) {
              setSelectedVersion(response.items[0]);
            }
          }
        } catch (error) {
          showToast(extractErrorMessage(error), 'error');
        } finally {
          setConfirmDialog({ ...confirmDialog, isOpen: false });
        }
      }
    });
  };

  // 验证版本号格式 (x.y.z，xyz都是数字)
  const isValidVersion = (version: string): boolean => {
    const versionRegex = /^\d+\.\d+\.\d+$/;
    return versionRegex.test(version);
  };

  // AI 提示词相关处理函数
  const handleOpenAddPrompt = () => {
    setEditingPromptId(null);
    setPromptForm({ title: '', prompt: '' });
    setShowAddPromptForm(true);
  };

  const handleOpenEditPrompt = (prompt: AIPrompt) => {
    setShowAddPromptForm(false);
    setEditingPromptId(prompt.id);
    setPromptForm({ title: prompt.title, prompt: prompt.prompt });
  };

  const handleCancelPrompt = () => {
    setShowAddPromptForm(false);
    setEditingPromptId(null);
    setPromptForm({ title: '', prompt: '' });
  };

  const handleSavePrompt = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!module) return;
    if (!promptForm.title.trim() || !promptForm.prompt.trim()) {
      showToast('请填写标题和提示词内容', 'error');
      return;
    }

    setSavingPrompt(true);
    try {
      const currentPrompts = module.ai_prompts || [];
      let newPrompts: AIPrompt[];

      if (editingPromptId) {
        // 编辑现有提示词
        newPrompts = currentPrompts.map(p => 
          p.id === editingPromptId 
            ? { ...p, title: promptForm.title, prompt: promptForm.prompt }
            : p
        );
      } else {
        // 添加新提示词
        const newPrompt: AIPrompt = {
          id: generateUUID(),
          title: promptForm.title,
          prompt: promptForm.prompt,
          created_at: new Date().toISOString(),
        };
        newPrompts = [...currentPrompts, newPrompt];
      }

      await api.put(`/modules/${module.id}`, { ai_prompts: newPrompts });
      setModule({ ...module, ai_prompts: newPrompts });
      setShowAddPromptForm(false);
      setEditingPromptId(null);
      setPromptForm({ title: '', prompt: '' });
      showToast(editingPromptId ? '提示词已更新' : '提示词已添加', 'success');
    } catch (error) {
      showToast(extractErrorMessage(error), 'error');
    } finally {
      setSavingPrompt(false);
    }
  };

  const handleDeletePrompt = (prompt: AIPrompt) => {
    if (!module) return;

    setConfirmDialog({
      isOpen: true,
      title: '删除提示词',
      message: `确定要删除提示词"${prompt.title}"吗？`,
      onConfirm: async () => {
        try {
          const currentPrompts = module.ai_prompts || [];
          const newPrompts = currentPrompts.filter(p => p.id !== prompt.id);
          await api.put(`/modules/${module.id}`, { ai_prompts: newPrompts });
          setModule({ ...module, ai_prompts: newPrompts });
          showToast('提示词已删除', 'success');
        } catch (error) {
          showToast(extractErrorMessage(error), 'error');
        } finally {
          setConfirmDialog({ ...confirmDialog, isOpen: false });
        }
      }
    });
  };

  const handleCreateVersion = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!module || !createVersionForm.version) {
      showToast('请输入版本号', 'error');
      return;
    }
    
    // 验证版本号格式
    if (!isValidVersion(createVersionForm.version)) {
      showToast('版本号格式错误，请使用 x.y.z 格式（如 4.2.0）', 'error');
      return;
    }
    
    try {
      setCreatingVersion(true);
      const newVersion = await createVersion(module.id, createVersionForm);
      
      // 安全地获取版本号
      const displayVersion = newVersion?.version || createVersionForm.version;
      showToast(`版本 ${displayVersion} 创建成功`, 'success');
      
      setShowAddVersionForm(false);
      setCreateVersionForm({ version: '', inherit_schema_from: '', set_as_default: false });
      
      // 重新加载版本列表
      const response = await listVersions(module.id);
      setVersions(response.items || []);
      
      // 如果新版本是默认版本，选中它
      if (newVersion?.is_default) {
        setSelectedVersion(newVersion);
      } else {
        // 否则选中默认版本
        const defaultVersion = response.items?.find(v => v.is_default);
        if (defaultVersion) {
          setSelectedVersion(defaultVersion);
        }
      }
    } catch (error) {
      showToast(extractErrorMessage(error), 'error');
    } finally {
      setCreatingVersion(false);
    }
  };

  const handleCancelVersion = () => {
    setShowAddVersionForm(false);
    setCreateVersionForm({ version: '', inherit_schema_from: '', set_as_default: false });
  };

  // 模块编辑相关处理函数
  const handleOpenEditModule = () => {
    if (module) {
      setModuleForm({
        module_source: module.module_source || '',
        description: module.description || '',
        version: module.version || '',
        branch: (module as any).branch || 'main'
      });
    }
    setShowEditModuleForm(true);
  };

  const handleCancelEditModule = () => {
    setShowEditModuleForm(false);
  };

  const handleSaveModule = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!module) return;

    setSavingModule(true);
    try {
      await api.put(`/modules/${module.id}`, moduleForm);
      setModule({ 
        ...module, 
        module_source: moduleForm.module_source,
        description: moduleForm.description,
        version: moduleForm.version
      });
      setShowEditModuleForm(false);
      showToast('模块信息已更新', 'success');
    } catch (error) {
      showToast(extractErrorMessage(error), 'error');
    } finally {
      setSavingModule(false);
    }
  };

  if (loading) {
    return (
      <div className={styles.container}>
        <div className={styles.loading}>
          <div className={styles.spinner}></div>
          <span>加载中...</span>
        </div>
      </div>
    );
  }

  if (!module) {
    return (
      <div className={styles.container}>
        <div className={styles.error}>模块不存在</div>
      </div>
    );
  }

  // 渲染提示词表单
  const renderPromptForm = (isEdit: boolean = false) => (
    <form onSubmit={handleSavePrompt} className={styles.inlineForm}>
      <div className={styles.inlineFormHeader}>
        <h4 className={styles.inlineFormTitle}>
          {isEdit ? '编辑提示词' : '添加新提示词'}
        </h4>
      </div>
      
      <div className={styles.inlineFormRow}>
        <div className={styles.inlineFormGroup}>
          <label className={styles.inlineFormLabel}>标题 *</label>
          <input
            type="text"
            value={promptForm.title}
            onChange={e => setPromptForm({ ...promptForm, title: e.target.value })}
            placeholder="例如：创建生产环境 EC2"
            className={styles.inlineFormInput}
            required
          />
        </div>
      </div>

      <div className={styles.inlineFormGroup}>
        <label className={styles.inlineFormLabel}>提示词内容 *</label>
        <textarea
          value={promptForm.prompt}
          onChange={e => setPromptForm({ ...promptForm, prompt: e.target.value })}
          placeholder="例如：在 exchange VPC 的东京1a创建一台 ec2，安全组使用 java-private，主机名称使用 xxx，使用 t3.medium 类型"
          className={styles.inlineFormTextarea}
          rows={3}
          required
        />
        <p className={styles.inlineFormHint}>用户可以点击此提示词快速填充到 AI 助手输入框</p>
      </div>

      <div className={styles.inlineFormActions}>
        <button type="submit" className={styles.inlineFormSubmit} disabled={savingPrompt}>
          {savingPrompt ? '保存中...' : (isEdit ? '更新提示词' : '添加提示词')}
        </button>
        <button type="button" onClick={handleCancelPrompt} className={styles.inlineFormCancel}>
          取消
        </button>
      </div>
    </form>
  );

  return (
    <div className={styles.pageWrapper}>
      <div className={styles.container}>
        {/* 顶部导航 */}
        <div className={styles.topNav}>
        <button onClick={() => navigate('/modules')} className={styles.backButton}>
          <span className={styles.backIcon}>←</span>
          返回模块列表
        </button>
        <div className={styles.topActions}>
          <button 
            onClick={handleOpenEditModule} 
            className={styles.topBtn}
            disabled={showEditModuleForm}
          >
            {showEditModuleForm ? '编辑中...' : '编辑'}
          </button>
          <button 
            onClick={handleToggleStatus} 
            className={`${styles.topBtn} ${module.status === 'active' ? styles.warning : styles.success}`}
          >
            {module.status === 'active' ? '停用' : '启用'}
          </button>
          <button 
            onClick={handleDelete} 
            className={`${styles.topBtn} ${styles.danger}`}
            disabled={module.status !== 'inactive'}
          >
            删除
          </button>
        </div>
      </div>

      {/* 内联编辑模块表单 */}
      {showEditModuleForm && (
        <form onSubmit={handleSaveModule} className={styles.inlineForm}>
          <div className={styles.inlineFormHeader}>
            <h4 className={styles.inlineFormTitle}>编辑模块信息</h4>
          </div>
          
          <div className={styles.inlineFormRow}>
            <div className={styles.inlineFormGroup} style={{ flex: 2 }}>
              <label className={styles.inlineFormLabel}>Module Source</label>
              <input
                type="text"
                value={moduleForm.module_source}
                onChange={e => setModuleForm({ ...moduleForm, module_source: e.target.value })}
                placeholder="例如: terraform-aws-modules/vpc/aws"
                className={styles.inlineFormInput}
              />
            </div>
            
            <div className={styles.inlineFormGroup} style={{ flex: 1 }}>
              <label className={styles.inlineFormLabel}>版本</label>
              <input
                type="text"
                value={moduleForm.version}
                onChange={e => setModuleForm({ ...moduleForm, version: e.target.value })}
                placeholder="输入版本号"
                className={styles.inlineFormInput}
              />
            </div>
            
            <div className={styles.inlineFormGroup} style={{ flex: 1 }}>
              <label className={styles.inlineFormLabel}>分支</label>
              <input
                type="text"
                value={moduleForm.branch}
                onChange={e => setModuleForm({ ...moduleForm, branch: e.target.value })}
                placeholder="main"
                className={styles.inlineFormInput}
              />
            </div>
          </div>

          <div className={styles.inlineFormGroup}>
            <label className={styles.inlineFormLabel}>描述</label>
            <textarea
              value={moduleForm.description}
              onChange={e => setModuleForm({ ...moduleForm, description: e.target.value })}
              placeholder="输入模块描述"
              className={styles.inlineFormTextarea}
              rows={3}
            />
          </div>

          <div className={styles.inlineFormActions}>
            <button type="submit" className={styles.inlineFormSubmit} disabled={savingModule}>
              {savingModule ? '保存中...' : '保存更改'}
            </button>
            <button type="button" onClick={handleCancelEditModule} className={styles.inlineFormCancel}>
              取消
            </button>
          </div>
        </form>
      )}

      {/* 模块头部信息 */}
      <div className={styles.moduleHeader}>
        <div className={styles.headerLeft}>
          <div className={styles.moduleMeta}>
            <span className={styles.providerBadge}>{module.provider}</span>
            <span className={`${styles.statusDot} ${styles[module.status]}`}></span>
            <span className={styles.statusText}>{module.status === 'active' ? '运行中' : '已停用'}</span>
          </div>
          <h1 className={styles.moduleName}>{module.name}</h1>
          {module.description && (
            <p className={styles.moduleDescription}>{module.description}</p>
          )}
        </div>
        
        {/* 版本选择器 - 集成到头部 */}
        <div className={styles.versionSelector}>
          <div className={styles.versionLabel}>TF Module Version</div>
          <div className={styles.versionDropdownWrapper}>
            <button 
              className={styles.versionButton}
              onClick={() => setShowVersionDropdown(!showVersionDropdown)}
            >
              <span className={styles.versionNumber}>
                {selectedVersion?.version || module.version}
              </span>
              {selectedVersion?.is_default && (
                <span className={styles.defaultTag}>默认</span>
              )}
              <span className={styles.dropdownArrow}>▼</span>
            </button>
            
            {showVersionDropdown && (() => {
              console.log('[Render] versions:', versions, 'length:', versions.length);
              return (
              <div className={styles.versionDropdown}>
                {versions.length === 0 && <div style={{padding: '12px', color: '#999'}}>加载中...</div>}
                {versions.map(version => (
                  <div
                    key={version.id}
                    className={`${styles.versionItem} ${selectedVersion?.id === version.id ? styles.active : ''}`}
                    onClick={() => handleSelectVersion(version)}
                  >
                    <div className={styles.versionItemInfo}>
                      <span className={styles.versionItemNumber}>{version.version}</span>
                      {version.is_default && <span className={styles.defaultTag}>默认</span>}
                    </div>
                    <div className={styles.versionItemMeta}>
                      {version.active_schema_version 
                        ? `Schema v${version.active_schema_version}` 
                        : '无 Schema'} · {version.demo_count || 0} Demo
                    </div>
                    {!version.is_default && (
                      <div className={styles.versionActions}>
                        <button
                          className={styles.setDefaultBtn}
                          onClick={(e) => {
                            e.stopPropagation();
                            handleSetDefaultVersion(version);
                          }}
                        >
                          设为默认
                        </button>
                        <button
                          className={styles.deleteVersionBtn}
                          onClick={(e) => {
                            e.stopPropagation();
                            handleDeleteVersion(version);
                          }}
                        >
                          删除
                        </button>
                      </div>
                    )}
                  </div>
                ))}
                <div 
                  className={styles.addVersionItem}
                  onClick={() => {
                    setShowVersionDropdown(false);
                    setShowAddVersionForm(true);
                  }}
                >
                  + 添加新版本
                </div>
              </div>
              );
            })()}
          </div>
        </div>
      </div>

      {/* 内联添加版本表单 */}
      {showAddVersionForm && (
        <form onSubmit={handleCreateVersion} className={styles.inlineForm}>
          <div className={styles.inlineFormHeader}>
            <h4 className={styles.inlineFormTitle}>创建新版本</h4>
          </div>
          
          <div className={styles.inlineFormRow}>
            <div className={styles.inlineFormGroup} style={{ flex: 1 }}>
              <label className={styles.inlineFormLabel}>版本号 *</label>
              <input
                type="text"
                value={createVersionForm.version}
                onChange={e => setCreateVersionForm({ ...createVersionForm, version: e.target.value })}
                placeholder="格式: x.y.z（如 4.2.0）"
                className={styles.inlineFormInput}
                required
              />
            </div>
            
            {versions.length > 0 && (
              <div className={styles.inlineFormGroup} style={{ flex: 1 }}>
                <label className={styles.inlineFormLabel}>继承 Schema</label>
                <select
                  value={createVersionForm.inherit_schema_from || ''}
                  onChange={e => setCreateVersionForm({ ...createVersionForm, inherit_schema_from: e.target.value })}
                  className={styles.inlineFormSelect}
                >
                  <option value="">不继承（从空白开始）</option>
                  {versions.filter(v => v.active_schema_version).map(v => (
                    <option key={v.id} value={v.id}>
                      {v.version} (Schema v{v.active_schema_version})
                    </option>
                  ))}
                </select>
              </div>
            )}
            
            <div className={styles.inlineFormCheckbox}>
              <label className={styles.inlineCheckboxLabel}>
                <input
                  type="checkbox"
                  checked={createVersionForm.set_as_default}
                  onChange={e => setCreateVersionForm({ ...createVersionForm, set_as_default: e.target.checked })}
                />
                <span>设为默认版本</span>
              </label>
            </div>
          </div>

          <div className={styles.inlineFormActions}>
            <button type="submit" className={styles.inlineFormSubmit} disabled={creatingVersion}>
              {creatingVersion ? '创建中...' : '创建版本'}
            </button>
            <button type="button" onClick={handleCancelVersion} className={styles.inlineFormCancel}>
              取消
            </button>
          </div>
        </form>
      )}

      {/* Source 信息 */}
      <div className={styles.sourceBar}>
        <span className={styles.sourceLabel}>Source</span>
        <code className={styles.sourceCode}>{module.module_source || module.source}</code>
      </div>

      {/* 快捷操作区 */}
      <div className={styles.quickActions}>
        {/* AI Skill 卡片 */}
        <div 
          className={styles.quickAction}
          style={{ borderColor: '#722ed1', backgroundColor: '#f9f0ff' }}
          onClick={() => {
            const url = selectedVersion 
              ? `/modules/${module.id}/skill?version_id=${selectedVersion.id}`
              : `/modules/${module.id}/skill`;
            navigate(url);
          }}
        >
          <div className={styles.quickActionIcon} style={{ color: '#722ed1' }}>
            <svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
              <path d="M9 18h6M10 22h4M12 2a7 7 0 0 1 7 7c0 2.38-1.19 4.47-3 5.74V17a1 1 0 0 1-1 1H9a1 1 0 0 1-1-1v-2.26C6.19 13.47 5 11.38 5 9a7 7 0 0 1 7-7z"/>
            </svg>
          </div>
          <div className={styles.quickActionText}>
            <span className={styles.quickActionTitle} style={{ color: '#722ed1' }}>AI Skill</span>
            <span className={styles.quickActionDesc}>配置 AI 知识</span>
          </div>
          <span className={styles.quickActionArrow}>→</span>
        </div>
        {/* Schema 配置卡片 */}
        <div 
          className={styles.quickAction}
          onClick={() => {
            const url = selectedVersion 
              ? `/modules/${module.id}/schemas?version_id=${selectedVersion.id}`
              : `/modules/${module.id}/schemas`;
            navigate(url);
          }}
        >
          <div className={styles.quickActionIcon}>
            <svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
              <path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"></path>
              <polyline points="14 2 14 8 20 8"></polyline>
              <line x1="16" y1="13" x2="8" y2="13"></line>
              <line x1="16" y1="17" x2="8" y2="17"></line>
              <polyline points="10 9 9 9 8 9"></polyline>
            </svg>
          </div>
          <div className={styles.quickActionText}>
            <span className={styles.quickActionTitle}>Schema 配置</span>
            <span className={styles.quickActionDesc}>
              {selectedVersion?.active_schema_version 
                ? `Schema v${selectedVersion.active_schema_version}` 
                : '管理表单 Schema'}
            </span>
          </div>
          <span className={styles.quickActionArrow}>→</span>
        </div>
        <div 
          className={styles.quickAction}
          onClick={() => {
            const url = selectedVersion 
              ? `/modules/${module.id}/demos?version_id=${selectedVersion.id}`
              : `/modules/${module.id}/demos`;
            navigate(url);
          }}
        >
          <div className={styles.quickActionIcon}>
            <svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
              <circle cx="12" cy="12" r="10"></circle>
              <polygon points="10 8 16 12 10 16 10 8"></polygon>
            </svg>
          </div>
          <div className={styles.quickActionText}>
            <span className={styles.quickActionTitle}>Demo 列表</span>
            <span className={styles.quickActionDesc}>
              {selectedVersion?.demo_count 
                ? `${selectedVersion.demo_count} 个 Demo` 
                : '查看演示配置'}
            </span>
          </div>
          <span className={styles.quickActionArrow}>→</span>
        </div>
      </div>

      {/* AI 提示词区块 */}
      <div className={styles.promptsSection}>
        <div className={styles.promptsHeader}>
          <div className={styles.promptsTitle}>
            <svg className={styles.promptsIcon} width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
              <path d="M9 18h6M10 22h4M12 2a7 7 0 0 1 7 7c0 2.38-1.19 4.47-3 5.74V17a1 1 0 0 1-1 1H9a1 1 0 0 1-1-1v-2.26C6.19 13.47 5 11.38 5 9a7 7 0 0 1 7-7z"/>
            </svg>
            <span>AI 助手提示词 ({module.ai_prompts?.length || 0})</span>
          </div>
          {!showAddPromptForm && !editingPromptId && (
            <button className={styles.addPromptBtn} onClick={handleOpenAddPrompt}>
              + 添加提示词
            </button>
          )}
        </div>
        
        {/* 提示词列表 */}
        <div className={styles.promptsList}>
          {module.ai_prompts?.map(prompt => (
            <React.Fragment key={prompt.id}>
              {editingPromptId === prompt.id ? (
                // 编辑表单
                renderPromptForm(true)
              ) : (
                // 提示词卡片
                <div className={styles.promptCard}>
                  <div className={styles.promptCardHeader}>
                    <h4 className={styles.promptCardTitle}>{prompt.title}</h4>
                    <div className={styles.promptCardActions}>
                      <button 
                        className={styles.promptEditBtn}
                        onClick={() => handleOpenEditPrompt(prompt)}
                      >
                        编辑
                      </button>
                      <button 
                        className={styles.promptDeleteBtn}
                        onClick={() => handleDeletePrompt(prompt)}
                      >
                        删除
                      </button>
                    </div>
                  </div>
                  <p className={styles.promptCardContent}>{prompt.prompt}</p>
                </div>
              )}
            </React.Fragment>
          ))}
          
          {/* 添加表单 */}
          {showAddPromptForm && !editingPromptId && renderPromptForm(false)}
          
          {/* 空状态 */}
          {(!module.ai_prompts || module.ai_prompts.length === 0) && !showAddPromptForm && (
            <div className={styles.promptsEmpty}>
              <svg className={styles.promptsEmptyIcon} width="48" height="48" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5">
                <path d="M9 18h6M10 22h4M12 2a7 7 0 0 1 7 7c0 2.38-1.19 4.47-3 5.74V17a1 1 0 0 1-1 1H9a1 1 0 0 1-1-1v-2.26C6.19 13.47 5 11.38 5 9a7 7 0 0 1 7-7z"/>
              </svg>
              <p>添加提示词，帮助用户更好地使用 AI 助手</p>
              <button className={styles.addFirstPromptBtn} onClick={handleOpenAddPrompt}>
                添加第一个提示词
              </button>
            </div>
          )}
        </div>
      </div>

      {/* 模块信息 */}
      <div className={styles.infoBar}>
        <div className={styles.infoItem}>
          <span className={styles.infoLabel}>ID</span>
          <span className={styles.infoValue}>{module.id}</span>
        </div>
        <div className={styles.infoItem}>
          <span className={styles.infoLabel}>创建时间</span>
          <span className={styles.infoValue}>{new Date(module.created_at).toLocaleDateString()}</span>
        </div>
        <div className={styles.infoItem}>
          <span className={styles.infoLabel}>更新时间</span>
          <span className={styles.infoValue}>{new Date(module.updated_at).toLocaleDateString()}</span>
        </div>
      </div>

        <ConfirmDialog
          isOpen={confirmDialog.isOpen}
          title={confirmDialog.title}
          message={confirmDialog.message}
          onConfirm={confirmDialog.onConfirm}
          onCancel={() => setConfirmDialog({ ...confirmDialog, isOpen: false })}
        />
      </div>
    </div>
  );
};

export default ModuleDetail;

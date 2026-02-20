import React, { useState, useEffect } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import { workspaceService, type Workspace } from '../services/workspaces';
import { workspacePoolAPI, type PoolWithAgentCount } from '../services/agent';
import { adminService, type TerraformVersion, detectEngineTypeFromURL, getEngineDisplayName } from '../services/admin';
import { getProjects, getWorkspaceProject, setWorkspaceProject, removeWorkspaceFromProject, type Project } from '../services/projects';
import { useToast } from '../contexts/ToastContext';
import { extractErrorMessage, logError } from '../utils/errorHandler';
import ProviderSettings from './ProviderSettings';
import WorkspaceRunTaskConfig from '../components/WorkspaceRunTaskConfig';
import WorkspaceRunTriggerConfig from '../components/WorkspaceRunTriggerConfig';
import WorkspaceNotificationConfig from '../components/WorkspaceNotificationConfig';
import { StateUpload } from '../components/StateUpload';
import { StateVersionHistory } from '../components/StateVersionHistory';
import styles from './WorkspaceSettings.module.css';

type SettingsTab = 'general' | 'locking' | 'provider' | 'state' | 'run-tasks' | 'run-triggers' | 'notifications' | 'destruction';

interface WorkspaceSettingsProps {
  section: SettingsTab;
}

const WorkspaceSettings: React.FC<WorkspaceSettingsProps> = React.memo(({ section }) => {
  const { id } = useParams<{ id: string }>();
  const navigate = useNavigate();
  const { success, error: showError } = useToast();
  
  // 使用传入的section prop
  const currentTab = section;
  
  const [loading, setLoading] = useState(true);
  const [settings, setSettings] = useState<Workspace | null>(null);
  
  // Project 状态
  const [projects, setProjects] = useState<Project[]>([]);
  const [currentProjectId, setCurrentProjectId] = useState<number | null>(null);
  const [projectLoading, setProjectLoading] = useState(false);
  
  // General Settings状态
  const [name, setName] = useState('');
  const [description, setDescription] = useState('');
  const [tagPairs, setTagPairs] = useState<Array<{ key: string; value: string }>>([
    { key: '', value: '' }
  ]);
  const [executionMode, setExecutionMode] = useState<'local' | 'agent' | 'k8s'>('local');
  const [agentPoolId, setAgentPoolId] = useState<number | undefined>();
  const [k8sConfigId, setK8sConfigId] = useState<number | undefined>();
  const [availablePools, setAvailablePools] = useState<PoolWithAgentCount[]>([]);
  const [currentPoolId, setCurrentPoolId] = useState<string | undefined>();
  const [terraformVersion, setTerraformVersion] = useState('latest');
  const [availableTerraformVersions, setAvailableTerraformVersions] = useState<TerraformVersion[]>([]);
  const [workdir, setWorkdir] = useState('/workspace');
  const [autoApply, setAutoApply] = useState(false);
  const [uiMode, setUiMode] = useState<'console' | 'structured'>('console');
  const [showUnchangedResources, setShowUnchangedResources] = useState(false);
  
  // Locking状态
  const [isLocked, setIsLocked] = useState(false);
  const [lockReason, setLockReason] = useState('');
  const [showLockReason, setShowLockReason] = useState(false);
  
  // 对话框状态
  const [isDeleteDialogOpen, setIsDeleteDialogOpen] = useState(false);
  const [deleteConfirmName, setDeleteConfirmName] = useState('');
  
  // 错误状态
  const [nameError, setNameError] = useState('');
  
  // 表单修改状态
  const [hasChanges, setHasChanges] = useState(false);

  useEffect(() => {
    loadSettings();
    loadAvailablePools();
    loadAvailableTerraformVersions();
    loadProjects();
    loadCurrentProject();
  }, [id]);

  const loadProjects = async () => {
    try {
      const projectList = await getProjects();
      setProjects(projectList);
    } catch (err) {
      console.error('Failed to load projects:', err);
    }
  };

  const loadCurrentProject = async () => {
    try {
      setProjectLoading(true);
      const project = await getWorkspaceProject(id!);
      setCurrentProjectId(project?.id || null);
    } catch (err) {
      console.error('Failed to load current project:', err);
      setCurrentProjectId(null);
    } finally {
      setProjectLoading(false);
    }
  };

  const handleProjectChange = async (projectId: number | null) => {
    try {
      if (projectId === null) {
        await removeWorkspaceFromProject(id!);
        setCurrentProjectId(null);
        success('已从项目中移除');
      } else {
        await setWorkspaceProject(id!, projectId);
        setCurrentProjectId(projectId);
        success('项目已更新');
      }
    } catch (err) {
      logError('更新项目', err);
      showError('更新项目失败: ' + extractErrorMessage(err));
    }
  };

  const loadAvailablePools = async () => {
    try {
      const poolsData = await workspacePoolAPI.getAvailablePools(id!);
      setAvailablePools(poolsData.pools || []);
      
      // Load current pool (404 is expected if no pool is set)
      try {
        const currentData = await workspacePoolAPI.getCurrentPool(id!);
        setCurrentPoolId(currentData.pool.pool_id);
      } catch (error: any) {
        // 404 means no current pool set, which is normal
        if (error.response?.status === 404) {
          setCurrentPoolId(undefined);
        } else {
          console.error('Failed to load current pool:', error);
        }
      }
    } catch (error: any) {
      // Only log if it's not a 404 (no pools available is also normal)
      if (error.response?.status !== 404) {
        console.error('Failed to load available pools:', error);
      }
    }
  };

  const loadAvailableTerraformVersions = async () => {
    try {
      const response = await adminService.getTerraformVersions({ enabled: true });
      setAvailableTerraformVersions(response.items || []);
    } catch (error: any) {
      console.error('Failed to load terraform versions:', error);
      // Don't show error to user, just log it
    }
  };

  const loadSettings = async () => {
    try {
      setLoading(true);
      const response = await workspaceService.getWorkspace(id!);
      const workspace = response.data;
      
      setSettings(workspace);
      setName(workspace.name);
      setDescription(workspace.description || '');
      setExecutionMode(workspace.execution_mode);
      setAgentPoolId(workspace.agent_pool_id);
      setK8sConfigId(workspace.k8s_config_id);
      setCurrentPoolId(workspace.current_pool_id);
      setTerraformVersion(workspace.terraform_version || 'latest');
      setWorkdir(workspace.workdir || '/workspace');
      setAutoApply(workspace.auto_apply || false);
      setUiMode(workspace.ui_mode || 'console');
      setShowUnchangedResources(workspace.show_unchanged_resources || false);
      setIsLocked(workspace.is_locked || false);
      // 只在workspace被锁定时才更新lockReason，避免覆盖用户正在输入的内容
      if (workspace.is_locked) {
        setLockReason(workspace.lock_reason || '');
      }
      
      // 转换tags为tagPairs
      if (workspace.tags && Object.keys(workspace.tags).length > 0) {
        const pairs = Object.entries(workspace.tags).map(([key, value]) => ({
          key,
          value: String(value)
        }));
        setTagPairs(pairs);
      }
    } catch (err) {
      logError('加载设置', err);
      showError('加载设置失败: ' + extractErrorMessage(err));
    } finally {
      setLoading(false);
    }
  };


  // 标签管理
  const handleTagChange = (index: number, field: 'key' | 'value', value: string) => {
    const newTagPairs = [...tagPairs];
    newTagPairs[index][field] = value;
    setTagPairs(newTagPairs);
    setHasChanges(true);
  };

  const addTagPair = () => {
    setTagPairs([...tagPairs, { key: '', value: '' }]);
  };

  const removeTagPair = (index: number) => {
    if (tagPairs.length > 1) {
      setTagPairs(tagPairs.filter((_, i) => i !== index));
      setHasChanges(true);
    }
  };

  // General Settings保存
  const handleSaveGeneral = async () => {
    // 验证名称
    if (!name.trim()) {
      setNameError('名称不能为空');
      return;
    }
    if (!/^[a-zA-Z0-9-_]+$/.test(name)) {
      setNameError('名称只能包含字母、数字、横线和下划线');
      return;
    }
    if (name.length < 3 || name.length > 50) {
      setNameError('名称长度必须在3-50个字符之间');
      return;
    }

    // 转换tags
    const tags: Record<string, any> = {};
    tagPairs.forEach(pair => {
      if (pair.key.trim()) {
        tags[pair.key.trim()] = pair.value.trim();
      }
    });

    try {
      // 保存workspace基本设置
      await workspaceService.updateWorkspace(id!, {
        name,
        description,
        tags,
        execution_mode: executionMode,
        agent_pool_id: executionMode === 'agent' ? agentPoolId : undefined,
        k8s_config_id: executionMode === 'k8s' ? k8sConfigId : undefined,
        terraform_version: terraformVersion,
        workdir,
        auto_apply: autoApply,
        ui_mode: uiMode,
        show_unchanged_resources: showUnchangedResources,
      });

      // 如果选择了pool，设置current pool
      if (currentPoolId && (executionMode === 'agent' || executionMode === 'k8s')) {
        await workspacePoolAPI.setCurrentPool(id!, currentPoolId);
      }
      
      success('设置已保存');
      setNameError('');
      setHasChanges(false);
      
      // 重新加载以获取最新数据
      loadSettings();
      loadAvailablePools();
    } catch (err: any) {
      logError('保存设置', err);
      if (err.message.includes('already exists')) {
        setNameError('名称已存在');
      } else {
        showError('保存设置失败: ' + extractErrorMessage(err));
      }
    }
  };

  // 锁定功能
  const handleLockToggle = () => {
    if (isLocked) {
      handleUnlock();
    } else {
      setLockReason(''); // 清空之前的lock reason
      setShowLockReason(true);
    }
  };

  const handleLock = async () => {
    // Lock Reason是必填字段
    if (!lockReason.trim()) {
      showError('Lock Reason is required');
      return;
    }

    try {
      await workspaceService.lockWorkspace(id!, lockReason);
      setIsLocked(true);
      setShowLockReason(false);
      success('Workspace已锁定');
      
      // 重新加载settings以获取locked_at和locked_by_username
      const response = await workspaceService.getWorkspace(id!);
      setSettings(response.data);
    } catch (err) {
      logError('锁定Workspace', err);
      showError('锁定失败: ' + extractErrorMessage(err));
    }
  };

  const handleUnlock = async () => {
    try {
      await workspaceService.unlockWorkspace(id!);
      setIsLocked(false);
      setLockReason(''); // 清空lock reason
      setShowLockReason(false); // 确保隐藏输入框
      success('Workspace已解锁');
      // 不调用loadSettings()，避免不必要的状态重置
    } catch (err) {
      logError('解锁Workspace', err);
      showError('解锁失败: ' + extractErrorMessage(err));
    }
  };

  // 删除Workspace
  const handleDeleteWorkspace = () => {
    if (isLocked) {
      showError('Workspace已锁定，请先解锁才能删除');
      return;
    }
    setIsDeleteDialogOpen(true);
  };

  const confirmDeleteWorkspace = async () => {
    if (deleteConfirmName !== name) {
      showError('名称不匹配');
      return;
    }

    try {
      await workspaceService.deleteWorkspace(id!);
      success('Workspace已删除');
      navigate('/workspaces');
    } catch (err) {
      logError('删除Workspace', err);
      showError('删除失败: ' + extractErrorMessage(err));
    }
  };

  if (loading) {
    return (
      <div className={styles.container}>
        <div className={styles.loading}>
          <div className={styles.spinner}></div>
          <p>加载中...</p>
        </div>
      </div>
    );
  }

  if (!settings) {
    return (
      <div className={styles.container}>
        <div className={styles.error}>Workspace不存在</div>
      </div>
    );
  }

  return (
    <>
      <div className={styles.settingsContent}>
        {/* General Settings */}
        {currentTab === 'general' && (
            <div className={styles.tabContent}>
              <div className={styles.contentHeader}>
                <h1 className={styles.contentTitle}>General Settings</h1>
                <p className={styles.contentDescription}>
                  Configure basic workspace settings including name, execution mode, and automation options.
                </p>
              </div>

              {/* 基本信息 */}
              <div className={styles.section}>
                <h2 className={styles.sectionTitle}>Basic Information</h2>
                
                <div className={styles.field}>
                  <label className={styles.label}>
                    Name <span className={styles.required}>*</span>
                  </label>
                  <input
                    type="text"
                    value={name}
                    onChange={(e) => {
                      setName(e.target.value);
                      setHasChanges(true);
                    }}
                    className={`${styles.input} ${nameError ? styles.inputError : ''}`}
                    placeholder="workspace-name"
                  />
                  {nameError && (
                    <div className={styles.errorText}>{nameError}</div>
                  )}
                  <div className={styles.hint}>
                    Name can only contain letters, numbers, hyphens, and underscores
                  </div>
                </div>

                <div className={styles.field}>
                  <label className={styles.label}>Description</label>
                  <textarea
                    value={description}
                    onChange={(e) => {
                      setDescription(e.target.value);
                      setHasChanges(true);
                    }}
                    className={styles.textarea}
                    rows={3}
                    placeholder="Describe the purpose of this workspace..."
                  />
                </div>

                {/* Project 选择 */}
                <div className={styles.field}>
                  <label className={styles.label}>Project</label>
                  <select
                    value={currentProjectId || ''}
                    onChange={(e) => {
                      const value = e.target.value;
                      handleProjectChange(value ? parseInt(value, 10) : null);
                    }}
                    className={styles.select}
                    disabled={projectLoading}
                  >
                    <option value="">No Project (Default)</option>
                    {projects.map(project => (
                      <option key={project.id} value={project.id}>
                        {project.display_name || project.name}
                        {project.is_default ? ' (Default)' : ''}
                      </option>
                    ))}
                  </select>
                  <div className={styles.hint}>
                    Assign this workspace to a project for better organization
                  </div>
                </div>

                <div className={styles.field}>
                  <label className={styles.label}>Tags</label>
                  <div className={styles.tagList}>
                    {tagPairs.map((pair, index) => (
                      <div key={index} className={styles.tagPair}>
                        <input
                          type="text"
                          value={pair.key}
                          onChange={(e) => handleTagChange(index, 'key', e.target.value)}
                          className={styles.tagInput}
                          placeholder="Key"
                        />
                        <span className={styles.tagSeparator}>=</span>
                        <input
                          type="text"
                          value={pair.value}
                          onChange={(e) => handleTagChange(index, 'value', e.target.value)}
                          className={styles.tagInput}
                          placeholder="Value"
                        />
                        {tagPairs.length > 1 && (
                          <button
                            type="button"
                            onClick={() => removeTagPair(index)}
                            className={styles.removeTagButton}
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
                  >
                    + Add tag
                  </button>
                  <div className={styles.hint}>
                    Tags help organize and filter workspaces
                  </div>
                </div>
              </div>

              {/* 执行配置 */}
              <div className={styles.section}>
                <h2 className={styles.sectionTitle}>Execution Configuration</h2>

                <div className={styles.field}>
                  <label className={styles.label}>
                    Execution Mode <span className={styles.required}>*</span>
                  </label>
                  <select
                    value={executionMode}
                    onChange={(e) => {
                      setExecutionMode(e.target.value as any);
                      setHasChanges(true);
                    }}
                    className={styles.select}
                  >
                    <option value="local">Local - Execute locally</option>
                    <option value="agent">Agent - Execute on remote agent</option>
                    <option value="k8s">K8s - Execute in Kubernetes</option>
                  </select>
                  <div className={styles.hint}>
                    Changing execution mode will affect all future runs
                  </div>
                </div>

                {(executionMode === 'agent' || executionMode === 'k8s') && (
                  <div className={styles.field}>
                    <label className={styles.label}>
                      Agent Pool <span className={styles.required}>*</span>
                    </label>
                    <select
                      value={currentPoolId || ''}
                      onChange={(e) => {
                        setCurrentPoolId(e.target.value || undefined);
                        setHasChanges(true);
                      }}
                      className={styles.select}
                    >
                      <option value="">Select a pool...</option>
                      {availablePools
                        .filter(pool => {
                          // Filter pools based on execution mode
                          if (executionMode === 'agent') return pool.pool_type === 'static';
                          if (executionMode === 'k8s') return pool.pool_type === 'k8s';
                          return true;
                        })
                        .map(pool => (
                          <option key={pool.pool_id} value={pool.pool_id}>
                            {pool.name} ({pool.pool_type}) - {pool.agent_count} agents, {pool.online_count} online
                          </option>
                        ))}
                    </select>
                    <div className={styles.hint}>
                      {executionMode === 'agent' 
                        ? 'Select a static pool for agent execution'
                        : 'Select a K8s pool for Kubernetes execution'
                      }
                    </div>
                    {availablePools.length === 0 && (
                      <div className={styles.warningText}>
                         No pools available. Please contact administrator to grant pool access to this workspace.
                      </div>
                    )}
                  </div>
                )}

                <div className={styles.field}>
                  <label className={styles.label}>IaC Engine</label>
                  <select
                    value={terraformVersion}
                    onChange={(e) => {
                      setTerraformVersion(e.target.value);
                      setHasChanges(true);
                    }}
                    className={styles.select}
                  >
                    <option value="latest">Latest (default)</option>
                    {availableTerraformVersions.map((version) => {
                      const engineType = detectEngineTypeFromURL(version.download_url);
                      const engineName = getEngineDisplayName(engineType);
                      return (
                        <option key={version.id} value={version.version}>
                          [{engineName}] {version.version}
                          {version.is_default ? ' (Default)' : ''}
                          {version.deprecated ? ' (Deprecated)' : ''}
                        </option>
                      );
                    })}
                  </select>
                  <div className={styles.hint}>
                    Select Terraform or OpenTofu version. Version change will take effect on next run.
                  </div>
                  {availableTerraformVersions.length === 0 && (
                    <div className={styles.warningText}>
                       No IaC engine versions configured. Please configure versions in Global Settings.
                    </div>
                  )}
                </div>

                <div className={styles.field}>
                  <label className={styles.label}>Terraform Working Directory</label>
                  <input
                    type="text"
                    value={workdir}
                    onChange={(e) => {
                      setWorkdir(e.target.value);
                      setHasChanges(true);
                    }}
                    className={styles.input}
                    placeholder="/"
                  />
                  <div className={styles.hint}>
                    Directory path where Terraform configuration files are located
                  </div>
                </div>
              </div>

              {/* Apply Method */}
              <div className={styles.section}>
                <h2 className={styles.sectionTitle}>Apply Method</h2>

                <div className={styles.field}>
                  <label className={styles.label}>Auto Apply</label>
                  <select
                    value={autoApply ? 'auto' : 'manual'}
                    onChange={(e) => {
                      setAutoApply(e.target.value === 'auto');
                      setHasChanges(true);
                    }}
                    className={styles.select}
                  >
                    <option value="manual">Manual - Require approval</option>
                    <option value="auto">Auto - Automatically apply</option>
                  </select>
                  <div className={styles.hint}>
                    {autoApply
                      ? 'Automatically apply changes after successful plan'
                      : 'Require manual approval before applying changes'
                    }
                  </div>
                </div>
              </div>

              {/* User Interface */}
              <div className={styles.section}>
                <h2 className={styles.sectionTitle}>User Interface</h2>

                <div className={styles.field}>
                  <label className={styles.label}>Run Output Display</label>
                  <select
                    value={uiMode}
                    onChange={(e) => {
                      setUiMode(e.target.value as 'console' | 'structured');
                      setHasChanges(true);
                    }}
                    className={styles.select}
                  >
                    <option value="console">Console UI</option>
                    <option value="structured">Structured Run Output</option>
                  </select>
                  <div className={styles.hint}>
                    {uiMode === 'console'
                      ? 'Display complete Terraform log output in console format'
                      : 'Display structured resource changes with stage-based tabs'
                    }
                  </div>
                </div>

                <div className={styles.field}>
                  <label className={styles.label}>Show Unchanged Resources</label>
                  <div className={styles.toggleContainer}>
                    <label className={styles.toggle}>
                      <input
                        type="checkbox"
                        checked={showUnchangedResources}
                        onChange={(e) => {
                          setShowUnchangedResources(e.target.checked);
                          setHasChanges(true);
                        }}
                      />
                      <span className={styles.toggleSlider}></span>
                    </label>
                    <span className={styles.toggleLabel}>
                      {showUnchangedResources ? 'Enabled' : 'Disabled'}
                    </span>
                  </div>
                  <div className={styles.hint}>
                    {showUnchangedResources
                      ? ' Warning: Including full plan data may cause performance issues with large infrastructures'
                      : 'Recommended: Excludes plan_json from API responses to improve performance'
                    }
                  </div>
                </div>
              </div>

              {/* 保存按钮 */}
              <div className={styles.saveSection}>
                <button
                  onClick={handleSaveGeneral}
                  className={styles.saveButton}
                  disabled={!hasChanges}
                >
                  Save Settings
                </button>
                {hasChanges && (
                  <span className={styles.unsavedHint}>You have unsaved changes</span>
                )}
              </div>
            </div>
          )}

          {/* Locking */}
          {currentTab === 'locking' && (
            <div className={styles.tabContent}>
              <div className={styles.contentHeader}>
                <h1 className={styles.contentTitle}>Locking</h1>
                <p className={styles.contentDescription}>
                  Lock this workspace to prevent runs and state changes.
                </p>
              </div>

              <div className={styles.section}>
                <div className={styles.lockStatus}>
                  <div className={styles.statusBadge}>
                    {isLocked ? (
                      <>
                        <span className={styles.lockedIcon}>🔒</span>
                        <span>Locked</span>
                      </>
                    ) : (
                      <>
                        <span className={styles.unlockedIcon}>🔓</span>
                        <span>Unlocked</span>
                      </>
                    )}
                  </div>

                  {isLocked && (
                    <div className={styles.lockInfo}>
                      {settings.locked_by_username && (
                        <div className={styles.lockInfoItem}>
                          <span className={styles.lockInfoLabel}>Locked by:</span>
                          <span className={styles.lockInfoValue}>{settings.locked_by_username}</span>
                        </div>
                      )}
                      {settings.locked_at && (
                        <div className={styles.lockInfoItem}>
                          <span className={styles.lockInfoLabel}>Locked at:</span>
                          <span className={styles.lockInfoValue}>
                            {new Date(settings.locked_at).toLocaleString()}
                          </span>
                        </div>
                      )}
                      {lockReason && (
                        <div className={styles.lockInfoItem}>
                          <span className={styles.lockInfoLabel}>Reason:</span>
                          <span className={styles.lockInfoValue}>{lockReason}</span>
                        </div>
                      )}
                    </div>
                  )}
                </div>

                {!isLocked && showLockReason && (
                  <div className={styles.field}>
                    <label className={styles.label}>
                      Lock Reason <span className={styles.required}>*</span>
                    </label>
                    <textarea
                      value={lockReason}
                      onChange={(e) => setLockReason(e.target.value)}
                      className={styles.textarea}
                      rows={2}
                      placeholder="Explain why you're locking this workspace... (Required)"
                      required
                    />
                    <div className={styles.hint}>
                      Lock reason is required and cannot be empty
                    </div>
                  </div>
                )}

                <div className={styles.actions}>
                  {isLocked ? (
                    <button
                      type="button"
                      onClick={handleUnlock}
                      className={styles.dangerButton}
                    >
                      Unlock Workspace
                    </button>
                  ) : showLockReason ? (
                    <>
                      <button
                        type="button"
                        onClick={(e) => {
                          e.preventDefault();
                          e.stopPropagation();
                          setShowLockReason(false);
                        }}
                        className={styles.cancelButton}
                      >
                        Cancel
                      </button>
                      <button
                        type="button"
                        onClick={(e) => {
                          e.preventDefault();
                          e.stopPropagation();
                          handleLock();
                        }}
                        className={styles.primaryButton}
                        disabled={!lockReason.trim()}
                      >
                        Confirm Lock
                      </button>
                    </>
                  ) : (
                    <button
                      type="button"
                      onClick={handleLockToggle}
                      className={styles.primaryButton}
                    >
                      Lock Workspace
                    </button>
                  )}
                </div>

                <div className={styles.hint}>
                  Locking prevents all plan and apply operations
                </div>
              </div>
            </div>
          )}

          {/* Provider */}
          {currentTab === 'provider' && (
            <div className={styles.tabContent}>
              <div className={styles.contentHeader}>
                <h1 className={styles.contentTitle}>Provider Configuration</h1>
                <p className={styles.contentDescription}>
                  Configure Terraform providers and their authentication methods.
                </p>
              </div>

              <div className={styles.section}>
                <ProviderSettings workspaceId={id!} />
              </div>
            </div>
          )}

          {/* State Management */}
          {currentTab === 'state' && (
            <div className={styles.tabContent}>
              <div className={styles.contentHeader}>
                <h1 className={styles.contentTitle}>State Management</h1>
                <p className={styles.contentDescription}>
                  Upload, download, and manage Terraform state versions. You can also rollback to previous versions.
                </p>
              </div>

              <div className={styles.section}>
                <StateUpload 
                  workspaceId={id!} 
                  onUploadSuccess={() => {
                    // 刷新版本历史
                    // StateVersionHistory 组件会自动刷新
                  }}
                />
              </div>

              <div className={styles.section}>
                <StateVersionHistory workspaceId={id!} />
              </div>
            </div>
          )}

          {/* Run Tasks */}
          {currentTab === 'run-tasks' && (
            <div className={styles.tabContent}>
              <div className={styles.contentHeader}>
                <h1 className={styles.contentTitle}>Run Tasks</h1>
                <p className={styles.contentDescription}>
                  Configure run tasks for this workspace. Run tasks allow external services to pass or fail Terraform runs.
                </p>
              </div>

              <div className={styles.section}>
                <WorkspaceRunTaskConfig workspaceId={id!} />
              </div>
            </div>
          )}

          {/* Run Triggers */}
          {currentTab === 'run-triggers' && (
            <div className={styles.tabContent}>
              <div className={styles.contentHeader}>
                <h1 className={styles.contentTitle}>Run Triggers</h1>
                <p className={styles.contentDescription}>
                  Configure which workspaces should be triggered when this workspace's apply completes successfully.
                </p>
              </div>

              <div className={styles.section}>
                <WorkspaceRunTriggerConfig workspaceId={id!} />
              </div>
            </div>
          )}

          {/* Notifications */}
          {currentTab === 'notifications' && (
            <div className={styles.tabContent}>
              <div className={styles.contentHeader}>
                <h1 className={styles.contentTitle}>Notifications</h1>
                <p className={styles.contentDescription}>
                  Configure webhook and Lark robot notifications for workspace events.
                </p>
              </div>

              <div className={styles.section}>
                <WorkspaceNotificationConfig workspaceId={id!} />
              </div>
            </div>
          )}

          {/* Destruction and Deletion */}
          {currentTab === 'destruction' && (
            <div className={styles.tabContent}>
              <div className={styles.contentHeader}>
                <h1 className={styles.contentTitle}>Destruction and Deletion</h1>
                <p className={styles.contentDescription}>
                  Permanently delete this workspace and optionally destroy all managed infrastructure.
                </p>
              </div>

              <div className={styles.section}>
                <div className={styles.dangerZone}>
                  <div className={styles.dangerItem}>
                    <div className={styles.dangerInfo}>
                      <h4 className={styles.dangerTitle}>Delete this Workspace</h4>
                      <p className={styles.dangerDescription}>
                        Once deleted, this workspace cannot be recovered. Make sure you have backed up any important data.
                      </p>
                    </div>
                    <button
                      onClick={handleDeleteWorkspace}
                      className={styles.dangerButton}
                      disabled={isLocked}
                    >
                      Delete Workspace
                    </button>
                  </div>

                  {isLocked && (
                    <div className={styles.dangerWarning}>
                      <span className={styles.warningIcon}></span>
                      <span>Workspace is locked. Unlock it before deletion.</span>
                    </div>
                  )}
                </div>
              </div>
          </div>
        )}
      </div>

      {/* 删除确认对话框 */}
      {isDeleteDialogOpen && (
        <div className={styles.dialogOverlay} onClick={() => {
          setIsDeleteDialogOpen(false);
          setDeleteConfirmName('');
        }}>
          <div className={styles.dialogContent} onClick={(e) => e.stopPropagation()}>
            <div className={styles.dialogHeader}>
              <h3 className={styles.dialogTitle}>Delete Workspace</h3>
            </div>
            
            <div className={styles.dialogBody}>
              <div className={styles.deleteConfirm}>
                <p className={styles.deleteWarning}>
                   This action cannot be undone! All data will be permanently lost.
                </p>
                <p>Please type the workspace name to confirm deletion:</p>
                <input
                  type="text"
                  value={deleteConfirmName}
                  onChange={(e) => setDeleteConfirmName(e.target.value)}
                  className={styles.confirmInput}
                  placeholder={name}
                  autoFocus
                />
              </div>
            </div>
            
            <div className={styles.dialogFooter}>
              <button
                onClick={() => {
                  setIsDeleteDialogOpen(false);
                  setDeleteConfirmName('');
                }}
                className={styles.cancelButton}
              >
                Cancel
              </button>
              <button
                onClick={confirmDeleteWorkspace}
                className={styles.deleteButton}
                disabled={deleteConfirmName !== name}
              >
                Delete
              </button>
            </div>
          </div>
        </div>
      )}
    </>
  );
});

WorkspaceSettings.displayName = 'WorkspaceSettings';

export default WorkspaceSettings;

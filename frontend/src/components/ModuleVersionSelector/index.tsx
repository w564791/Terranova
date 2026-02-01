import React, { useState, useEffect } from 'react';
import { useToast } from '../../contexts/ToastContext';
import { extractErrorMessage } from '../../utils/errorHandler';
import type { ModuleVersion, CreateModuleVersionRequest } from '../../services/moduleVersions';
import {
  listVersions,
  createVersion,
  setDefaultVersion,
  deleteVersion,
  inheritDemos,
} from '../../services/moduleVersions';
import styles from './ModuleVersionSelector.module.css';

interface ModuleVersionSelectorProps {
  moduleId: number;
  currentVersion?: string;  // 当前模块的版本号
  onVersionChange?: (version: ModuleVersion | null) => void;
  onVersionCreated?: (version: ModuleVersion) => void;
}

const ModuleVersionSelector: React.FC<ModuleVersionSelectorProps> = ({
  moduleId,
  currentVersion,
  onVersionChange,
  onVersionCreated,
}) => {
  const { showToast } = useToast();
  const [versions, setVersions] = useState<ModuleVersion[]>([]);
  const [selectedVersion, setSelectedVersion] = useState<ModuleVersion | null>(null);
  const [loading, setLoading] = useState(true);
  const [showDropdown, setShowDropdown] = useState(false);
  const [showCreateModal, setShowCreateModal] = useState(false);
  const [showInheritModal, setShowInheritModal] = useState(false);
  const [createForm, setCreateForm] = useState<CreateModuleVersionRequest>({
    version: '',
    inherit_schema_from: '',
    set_as_default: false,
  });
  const [creating, setCreating] = useState(false);

  // 加载版本列表
  const loadVersions = async () => {
    try {
      setLoading(true);
      const response = await listVersions(moduleId);
      setVersions(response.items || []);
      
      // 选择默认版本
      const defaultVersion = response.items?.find(v => v.is_default);
      if (defaultVersion) {
        setSelectedVersion(defaultVersion);
        onVersionChange?.(defaultVersion);
      } else if (response.items?.length > 0) {
        setSelectedVersion(response.items[0]);
        onVersionChange?.(response.items[0]);
      }
    } catch (error) {
      // 如果没有版本数据，不显示错误（可能是还没有执行迁移）
      console.log('No versions found:', error);
      setVersions([]);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    loadVersions();
  }, [moduleId]);

  // 选择版本
  const handleSelectVersion = (version: ModuleVersion) => {
    setSelectedVersion(version);
    setShowDropdown(false);
    onVersionChange?.(version);
  };

  // 设置默认版本
  const handleSetDefault = async (version: ModuleVersion) => {
    try {
      await setDefaultVersion(moduleId, version.id);
      showToast('默认版本设置成功', 'success');
      loadVersions();
    } catch (error) {
      showToast(extractErrorMessage(error), 'error');
    }
  };

  // 删除版本
  const handleDeleteVersion = async (version: ModuleVersion) => {
    if (version.is_default) {
      showToast('不能删除默认版本，请先设置其他版本为默认', 'error');
      return;
    }

    if (!confirm(`确定要删除版本 ${version.version} 吗？`)) {
      return;
    }

    try {
      await deleteVersion(moduleId, version.id);
      showToast('版本删除成功', 'success');
      loadVersions();
    } catch (error) {
      showToast(extractErrorMessage(error), 'error');
    }
  };

  // 创建新版本
  const handleCreateVersion = async () => {
    if (!createForm.version) {
      showToast('请输入版本号', 'error');
      return;
    }

    try {
      setCreating(true);
      const newVersion = await createVersion(moduleId, createForm);
      showToast('版本创建成功', 'success');
      setShowCreateModal(false);
      setCreateForm({ version: '', inherit_schema_from: '', set_as_default: false });
      loadVersions();
      onVersionCreated?.(newVersion);
    } catch (error) {
      showToast(extractErrorMessage(error), 'error');
    } finally {
      setCreating(false);
    }
  };

  // 继承 Demos
  const handleInheritDemos = async (fromVersionId: string) => {
    if (!selectedVersion) return;

    try {
      const result = await inheritDemos(moduleId, selectedVersion.id, {
        from_version_id: fromVersionId,
      });
      showToast(`成功继承 ${result.inherited_count} 个 Demo`, 'success');
      setShowInheritModal(false);
      loadVersions();
    } catch (error) {
      showToast(extractErrorMessage(error), 'error');
    }
  };

  // 如果没有版本数据，显示提示
  if (!loading && versions.length === 0) {
    return (
      <div className={styles.container}>
        <div className={styles.noVersions}>
          <span className={styles.noVersionsText}>暂无版本数据</span>
          <button
            className={styles.createButton}
            onClick={() => setShowCreateModal(true)}
          >
            + 创建版本
          </button>
        </div>

        {/* 创建版本弹窗 */}
        {showCreateModal && (
          <div className={styles.modalOverlay} onClick={() => setShowCreateModal(false)}>
            <div className={styles.modal} onClick={e => e.stopPropagation()}>
              <h3>创建新版本</h3>
              <div className={styles.formGroup}>
                <label>版本号 *</label>
                <input
                  type="text"
                  value={createForm.version}
                  onChange={e => setCreateForm({ ...createForm, version: e.target.value })}
                  placeholder={currentVersion || '例如: 6.1.5'}
                />
              </div>
              <div className={styles.formGroup}>
                <label>
                  <input
                    type="checkbox"
                    checked={createForm.set_as_default}
                    onChange={e => setCreateForm({ ...createForm, set_as_default: e.target.checked })}
                  />
                  设为默认版本
                </label>
              </div>
              <div className={styles.modalActions}>
                <button onClick={() => setShowCreateModal(false)}>取消</button>
                <button
                  className={styles.primaryButton}
                  onClick={handleCreateVersion}
                  disabled={creating}
                >
                  {creating ? '创建中...' : '创建'}
                </button>
              </div>
            </div>
          </div>
        )}
      </div>
    );
  }

  return (
    <div className={styles.container}>
      <div className={styles.selectorWrapper}>
        <label className={styles.label}>TF Module 版本:</label>
        
        <div className={styles.selector}>
          <button
            className={styles.selectorButton}
            onClick={() => setShowDropdown(!showDropdown)}
            disabled={loading}
          >
            {loading ? (
              '加载中...'
            ) : selectedVersion ? (
              <>
                <span className={styles.versionText}>
                  v{selectedVersion.version}
                  {selectedVersion.is_default && (
                    <span className={styles.defaultBadge}>默认</span>
                  )}
                </span>
                <span className={styles.versionMeta}>
                  {selectedVersion.schema_count || 0} Schema · {selectedVersion.demo_count || 0} Demo
                </span>
              </>
            ) : (
              '选择版本'
            )}
            <span className={styles.arrow}>▼</span>
          </button>

          {showDropdown && (
            <div className={styles.dropdown}>
              {versions.map(version => (
                <div
                  key={version.id}
                  className={`${styles.dropdownItem} ${
                    selectedVersion?.id === version.id ? styles.selected : ''
                  }`}
                >
                  <div
                    className={styles.versionInfo}
                    onClick={() => handleSelectVersion(version)}
                  >
                    <span className={styles.versionName}>
                      v{version.version}
                      {version.is_default && (
                        <span className={styles.defaultBadge}>默认</span>
                      )}
                    </span>
                    <span className={styles.versionStats}>
                      {version.schema_count || 0} Schema · {version.demo_count || 0} Demo
                    </span>
                  </div>
                  <div className={styles.versionActions}>
                    {!version.is_default && (
                      <button
                        className={styles.actionButton}
                        onClick={(e) => {
                          e.stopPropagation();
                          handleSetDefault(version);
                        }}
                        title="设为默认"
                      >
                        ★
                      </button>
                    )}
                    {!version.is_default && (
                      <button
                        className={styles.actionButton}
                        onClick={(e) => {
                          e.stopPropagation();
                          handleDeleteVersion(version);
                        }}
                        title="删除"
                      >
                        🗑
                      </button>
                    )}
                  </div>
                </div>
              ))}
              
              <div
                className={styles.dropdownItem}
                onClick={() => {
                  setShowDropdown(false);
                  setShowCreateModal(true);
                }}
              >
                <span className={styles.addVersion}>+ 添加新版本</span>
              </div>
            </div>
          )}
        </div>

        {selectedVersion && (
          <button
            className={selectedVersion.is_default ? styles.defaultButton : styles.setDefaultButton}
            onClick={() => !selectedVersion.is_default && handleSetDefault(selectedVersion)}
            disabled={selectedVersion.is_default}
          >
            {selectedVersion.is_default ? '✓ 已是默认' : '设为默认'}
          </button>
        )}

        {selectedVersion && selectedVersion.demo_count === 0 && versions.length > 1 && (
          <button
            className={styles.inheritButton}
            onClick={() => setShowInheritModal(true)}
          >
            继承 Demo
          </button>
        )}
      </div>

      {/* 创建版本弹窗 */}
      {showCreateModal && (
        <div className={styles.modalOverlay} onClick={() => setShowCreateModal(false)}>
          <div className={styles.modal} onClick={e => e.stopPropagation()}>
            <h3>创建新 Terraform Module 版本</h3>
            
            <div className={styles.formGroup}>
              <label>版本号 *</label>
              <input
                type="text"
                value={createForm.version}
                onChange={e => setCreateForm({ ...createForm, version: e.target.value })}
                placeholder="例如: 6.2.0"
              />
            </div>

            {versions.length > 0 && (
              <div className={styles.formGroup}>
                <label>从现有版本继承 Schema</label>
                <select
                  value={createForm.inherit_schema_from || ''}
                  onChange={e => setCreateForm({ ...createForm, inherit_schema_from: e.target.value })}
                >
                  <option value="">不继承</option>
                  {versions.map(v => (
                    <option key={v.id} value={v.id}>
                      v{v.version} {v.is_default ? '(默认)' : ''} - {v.schema_count || 0} Schema
                    </option>
                  ))}
                </select>
              </div>
            )}

            <div className={styles.formGroup}>
              <label>
                <input
                  type="checkbox"
                  checked={createForm.set_as_default}
                  onChange={e => setCreateForm({ ...createForm, set_as_default: e.target.checked })}
                />
                设为默认版本
              </label>
            </div>

            <div className={styles.modalActions}>
              <button onClick={() => setShowCreateModal(false)}>取消</button>
              <button
                className={styles.primaryButton}
                onClick={handleCreateVersion}
                disabled={creating}
              >
                {creating ? '创建中...' : '创建'}
              </button>
            </div>
          </div>
        </div>
      )}

      {/* 继承 Demo 弹窗 */}
      {showInheritModal && selectedVersion && (
        <div className={styles.modalOverlay} onClick={() => setShowInheritModal(false)}>
          <div className={styles.modal} onClick={e => e.stopPropagation()}>
            <h3>继承 Demo 配置</h3>
            <p className={styles.modalDescription}>
              选择要从哪个版本继承 Demo 配置到 v{selectedVersion.version}
            </p>
            
            <div className={styles.versionList}>
              {versions
                .filter(v => v.id !== selectedVersion.id && (v.demo_count || 0) > 0)
                .map(v => (
                  <div
                    key={v.id}
                    className={styles.inheritOption}
                    onClick={() => handleInheritDemos(v.id)}
                  >
                    <span>v{v.version}</span>
                    <span className={styles.demoCount}>{v.demo_count} Demo</span>
                  </div>
                ))}
            </div>

            {versions.filter(v => v.id !== selectedVersion.id && (v.demo_count || 0) > 0).length === 0 && (
              <p className={styles.noData}>没有可继承的 Demo</p>
            )}

            <div className={styles.modalActions}>
              <button onClick={() => setShowInheritModal(false)}>取消</button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
};

export default ModuleVersionSelector;

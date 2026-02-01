import React, { useState, useEffect } from 'react';
import { useParams, useNavigate, useSearchParams } from 'react-router-dom';
import { useToast } from '../contexts/ToastContext';
import { extractErrorMessage } from '../utils/errorHandler';
import { listVersions, type ModuleVersion } from '../services/moduleVersions';
import { 
  getModuleVersionSkill, 
  generateModuleVersionSkill, 
  updateModuleVersionSkillCustomContent,
  inheritModuleVersionSkill,
  type ModuleVersionSkill 
} from '../services/skill';
import api from '../services/api';
import styles from './ModuleVersionSkill.module.css';

interface Module {
  id: number;
  name: string;
  provider: string;
  default_version_id?: string;
}

const ModuleVersionSkillPage: React.FC = () => {
  const { moduleId } = useParams<{ moduleId: string }>();
  const [searchParams] = useSearchParams();
  const urlVersionId = searchParams.get('version_id');
  const navigate = useNavigate();
  const { showToast } = useToast();
  
  const [module, setModule] = useState<Module | null>(null);
  const [versions, setVersions] = useState<ModuleVersion[]>([]);
  const [selectedVersionId, setSelectedVersionId] = useState<string>('');
  const [skill, setSkill] = useState<ModuleVersionSkill | null>(null);
  const [loading, setLoading] = useState(true);
  const [loadingSkill, setLoadingSkill] = useState(false);
  const [generating, setGenerating] = useState(false);
  const [saving, setSaving] = useState(false);
  
  // 编辑状态
  const [isEditing, setIsEditing] = useState(false);
  const [customContent, setCustomContent] = useState('');
  
  // 继承弹窗
  const [showInheritModal, setShowInheritModal] = useState(false);
  const [inheritSourceVersionId, setInheritSourceVersionId] = useState('');
  const [inheriting, setInheriting] = useState(false);

  // 获取有 Skill 的版本列表（用于继承选择）
  const versionsWithSkill = versions.filter(v => 
    v.id !== selectedVersionId
  );

  useEffect(() => {
    const fetchData = async () => {
      try {
        // 获取模块信息
        const moduleRes = await api.get(`/modules/${moduleId}`);
        setModule(moduleRes.data);

        // 获取版本列表
        const versionsRes = await listVersions(Number(moduleId));
        setVersions(versionsRes.items || []);

        // 设置选中的版本
        if (urlVersionId) {
          const validVersion = versionsRes.items?.find(v => v.id === urlVersionId);
          if (validVersion) {
            setSelectedVersionId(urlVersionId);
          } else {
            const defaultVersionId = moduleRes.data.default_version_id;
            if (defaultVersionId) {
              setSelectedVersionId(defaultVersionId);
            } else if (versionsRes.items?.length > 0) {
              setSelectedVersionId(versionsRes.items[0].id);
            }
          }
        } else {
          const defaultVersionId = moduleRes.data.default_version_id;
          if (defaultVersionId) {
            setSelectedVersionId(defaultVersionId);
          } else if (versionsRes.items?.length > 0) {
            setSelectedVersionId(versionsRes.items[0].id);
          }
        }
      } catch (error) {
        const message = extractErrorMessage(error);
        showToast(message, 'error');
        navigate('/modules');
      } finally {
        setLoading(false);
      }
    };

    if (moduleId) {
      fetchData();
    }
  }, [moduleId, urlVersionId, navigate, showToast]);

  // 当选中版本变化时，加载该版本的 Skill
  useEffect(() => {
    const fetchSkill = async () => {
      if (!selectedVersionId) return;
      
      setLoadingSkill(true);
      try {
        const skillData = await getModuleVersionSkill(selectedVersionId);
        setSkill(skillData);
        setCustomContent(skillData.custom_content || '');
      } catch (error) {
        console.error('Failed to load skill:', error);
        setSkill(null);
        setCustomContent('');
      } finally {
        setLoadingSkill(false);
      }
    };

    fetchSkill();
  }, [selectedVersionId]);

  const handleGenerate = async () => {
    if (!selectedVersionId) return;
    
    setGenerating(true);
    try {
      const generatedSkill = await generateModuleVersionSkill(selectedVersionId);
      setSkill(generatedSkill);
      showToast('Skill 生成成功', 'success');
    } catch (error) {
      showToast(extractErrorMessage(error), 'error');
    } finally {
      setGenerating(false);
    }
  };

  const handleSaveCustomContent = async () => {
    if (!selectedVersionId) return;
    
    setSaving(true);
    try {
      const updatedSkill = await updateModuleVersionSkillCustomContent(selectedVersionId, customContent);
      setSkill(updatedSkill);
      setIsEditing(false);
      showToast('自定义内容已保存', 'success');
    } catch (error) {
      showToast(extractErrorMessage(error), 'error');
    } finally {
      setSaving(false);
    }
  };

  const handleInherit = async () => {
    if (!selectedVersionId || !inheritSourceVersionId) return;
    
    setInheriting(true);
    try {
      const inheritedSkill = await inheritModuleVersionSkill(selectedVersionId, inheritSourceVersionId);
      setSkill(inheritedSkill);
      setCustomContent(inheritedSkill.custom_content || '');
      setShowInheritModal(false);
      setInheritSourceVersionId('');
      showToast('Skill 继承成功', 'success');
    } catch (error) {
      showToast(extractErrorMessage(error), 'error');
    } finally {
      setInheriting(false);
    }
  };

  const selectedVersion = versions.find(v => v.id === selectedVersionId);

  if (loading) {
    return (
      <div className={styles.pageWrapper}>
        <div className={styles.container}>
          <div className={styles.loading}>
            <div className={styles.spinner}></div>
          </div>
        </div>
      </div>
    );
  }

  if (!module) {
    return (
      <div className={styles.pageWrapper}>
        <div className={styles.container}>
          <div className={styles.error}>模块不存在</div>
        </div>
      </div>
    );
  }

  return (
    <div className={styles.pageWrapper}>
      <div className={styles.container}>
        {/* 面包屑导航 */}
        <nav className={styles.breadcrumb}>
          <span onClick={() => navigate('/modules')} className={styles.breadcrumbLink}>模块</span>
          <span className={styles.breadcrumbSep}>/</span>
          <span onClick={() => navigate(`/modules/${moduleId}`)} className={styles.breadcrumbLink}>{module.name}</span>
          <span className={styles.breadcrumbSep}>/</span>
          <span className={styles.breadcrumbCurrent}>AI Skill</span>
        </nav>

        {/* 页面头部 */}
        <div className={styles.pageHeader}>
          <div className={styles.headerInfo}>
            <h1 className={styles.pageTitle}>
              AI Skill 配置
            </h1>
            {/* 版本选择器 */}
            <div className={styles.versionSelector}>
              <label>版本：</label>
              <select
                value={selectedVersionId}
                onChange={(e) => setSelectedVersionId(e.target.value)}
                className={styles.versionSelect}
              >
                {versions.map(v => (
                  <option key={v.id} value={v.id}>
                    {v.version} {v.is_default ? '(默认)' : ''} 
                    {v.active_schema_version ? ` - Schema v${v.active_schema_version}` : ' - 无 Schema'}
                  </option>
                ))}
              </select>
            </div>
          </div>
          <div className={styles.headerActions}>
            {versionsWithSkill.length > 0 && (
              <button 
                onClick={() => setShowInheritModal(true)} 
                className={styles.secondaryBtn}
              >
                从其他版本继承
              </button>
            )}
            <button 
              onClick={handleGenerate}
              disabled={generating || !selectedVersion?.active_schema_version}
              className={styles.primaryBtn}
              title={!selectedVersion?.active_schema_version ? '需要先配置 Schema' : ''}
            >
              {generating ? '生成中...' : '根据 Schema 生成'}
            </button>
          </div>
        </div>

        {/* Skill 内容区 */}
        {loadingSkill ? (
          <div className={styles.loadingSection}>
            <div className={styles.spinner}></div>
            <span>加载中...</span>
          </div>
        ) : (
          <div className={styles.skillContent}>
            {/* Schema 生成的内容 */}
            <div className={styles.section}>
              <div className={styles.sectionHeader}>
                <h2 className={styles.sectionTitle}>
                  Schema 生成的 Skill
                </h2>
                {skill?.schema_generated_at && (
                  <span className={styles.sectionMeta}>
                    生成于 {new Date(skill.schema_generated_at).toLocaleString('zh-CN')}
                    {skill.schema_version_used && ` · Schema v${skill.schema_version_used}`}
                  </span>
                )}
              </div>
              <div className={styles.sectionBody}>
                {skill?.schema_generated_content ? (
                  <div className={styles.markdownContent}>
                    <pre>{skill.schema_generated_content}</pre>
                  </div>
                ) : (
                  <div className={styles.emptyContent}>
                    <p>尚未生成 Skill 内容</p>
                    {selectedVersion?.active_schema_version ? (
                      <button onClick={handleGenerate} className={styles.generateBtn} disabled={generating}>
                        {generating ? '生成中...' : '点击生成'}
                      </button>
                    ) : (
                      <p className={styles.hint}>请先为该版本配置 Schema</p>
                    )}
                  </div>
                )}
              </div>
            </div>

            {/* 用户自定义内容 */}
            <div className={styles.section}>
              <div className={styles.sectionHeader}>
                <h2 className={styles.sectionTitle}>
                  自定义补充内容
                </h2>
                {!isEditing && (
                  <button 
                    onClick={() => setIsEditing(true)} 
                    className={styles.editBtn}
                  >
                    编辑
                  </button>
                )}
              </div>
              <div className={styles.sectionBody}>
                {isEditing ? (
                  <div className={styles.editForm}>
                    <textarea
                      value={customContent}
                      onChange={(e) => setCustomContent(e.target.value)}
                      placeholder="在此添加自定义的 Skill 内容，将与 Schema 生成的内容合并..."
                      className={styles.textarea}
                      rows={10}
                    />
                    <div className={styles.editActions}>
                      <button 
                        onClick={() => {
                          setIsEditing(false);
                          setCustomContent(skill?.custom_content || '');
                        }} 
                        className={styles.cancelBtn}
                      >
                        取消
                      </button>
                      <button 
                        onClick={handleSaveCustomContent}
                        disabled={saving}
                        className={styles.saveBtn}
                      >
                        {saving ? '保存中...' : '保存'}
                      </button>
                    </div>
                  </div>
                ) : (
                  <div className={styles.customContent}>
                    {skill?.custom_content ? (
                      <pre>{skill.custom_content}</pre>
                    ) : (
                      <p className={styles.placeholder}>暂无自定义内容，点击编辑添加</p>
                    )}
                  </div>
                )}
              </div>
            </div>

            {/* 继承信息 */}
            {skill?.inherited_from_version_id && (
              <div className={styles.inheritInfo}>
                此 Skill 继承自其他版本
              </div>
            )}
          </div>
        )}
      </div>

      {/* 继承弹窗 */}
      {showInheritModal && (
        <div className={styles.modalOverlay} onClick={() => setShowInheritModal(false)}>
          <div className={styles.modal} onClick={e => e.stopPropagation()}>
            <div className={styles.modalHeader}>
              <h2>继承 Skill</h2>
              <button 
                className={styles.closeBtn}
                onClick={() => setShowInheritModal(false)}
              >
                ×
              </button>
            </div>
            <div className={styles.modalBody}>
              <p className={styles.modalDesc}>
                将其他版本的 Skill 复制到当前版本 <strong>{selectedVersion?.version}</strong>
              </p>
              
              <div className={styles.formGroup}>
                <label>选择源版本</label>
                <select
                  value={inheritSourceVersionId}
                  onChange={(e) => setInheritSourceVersionId(e.target.value)}
                  className={styles.select}
                >
                  <option value="">请选择版本</option>
                  {versionsWithSkill.map(v => (
                    <option key={v.id} value={v.id}>
                      {v.version} {v.is_default ? '(默认)' : ''}
                    </option>
                  ))}
                </select>
              </div>
            </div>
            <div className={styles.modalFooter}>
              <button 
                className={styles.cancelBtn}
                onClick={() => setShowInheritModal(false)}
              >
                取消
              </button>
              <button 
                className={styles.primaryBtn}
                onClick={handleInherit}
                disabled={!inheritSourceVersionId || inheriting}
              >
                {inheriting ? '继承中...' : '确认继承'}
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
};

export default ModuleVersionSkillPage;
import { useState, useEffect } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import {
  getSkill,
  updateSkill,
  deleteSkill,
  activateSkill,
  deactivateSkill,
  type Skill,
  LAYER_LABELS,
  SOURCE_TYPE_LABELS,
} from '../services/skill';
import ReactMarkdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import ConfirmDialog from '../components/ConfirmDialog';
import styles from './SkillDetail.module.css';

// 去除 YAML frontmatter（--- 包裹的头部元数据）
const stripFrontmatter = (content: string): string => {
  const trimmed = content.trimStart();
  if (trimmed.startsWith('---')) {
    const endIndex = trimmed.indexOf('---', 3);
    if (endIndex !== -1) {
      return trimmed.slice(endIndex + 3).trimStart();
    }
  }
  return content;
};

const SkillDetail = () => {
  const { id } = useParams<{ id: string }>();
  const navigate = useNavigate();

  const [skill, setSkill] = useState<Skill | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [message, setMessage] = useState<{ type: 'success' | 'error'; text: string } | null>(null);
  const [isEditing, setIsEditing] = useState(false);
  const [saving, setSaving] = useState(false);
  const [deleteConfirm, setDeleteConfirm] = useState(false);

  // 编辑表单数据
  const [formData, setFormData] = useState<{
    display_name: string;
    description: string;
    content: string;
    version: string;
    priority: number;
    tags: string[];
    domain_tags: string[];
  }>({
    display_name: '',
    description: '',
    content: '',
    version: '',
    priority: 0,
    tags: [],
    domain_tags: [],
  });

  const [tagInput, setTagInput] = useState('');
  const [domainTagInput, setDomainTagInput] = useState('');

  useEffect(() => {
    if (id) {
      loadSkill();
    }
  }, [id]);

  const loadSkill = async () => {
    try {
      setLoading(true);
      setError(null);
      const data = await getSkill(id!);
      setSkill(data);
      resetFormData(data);
    } catch (err: any) {
      setError(err.response?.data?.error || '加载 Skill 失败');
    } finally {
      setLoading(false);
    }
  };

  const resetFormData = (s: Skill) => {
    setFormData({
      display_name: s.display_name,
      description: s.description || '',
      content: s.content,
      version: s.version,
      priority: s.priority,
      tags: s.metadata?.tags || [],
      domain_tags: s.metadata?.domain_tags || [],
    });
  };

  const handleStartEdit = () => {
    if (skill) {
      resetFormData(skill);
    }
    setIsEditing(true);
  };

  const handleCancelEdit = () => {
    if (skill) {
      resetFormData(skill);
    }
    setIsEditing(false);
  };

  const handleChange = (field: string, value: string | number | string[]) => {
    setFormData(prev => ({ ...prev, [field]: value }));
  };

  const handleAddTag = () => {
    const tag = tagInput.trim().toLowerCase();
    if (tag && !formData.tags.includes(tag)) {
      setFormData(prev => ({ ...prev, tags: [...prev.tags, tag] }));
      setTagInput('');
    }
  };

  const handleRemoveTag = (tag: string) => {
    setFormData(prev => ({ ...prev, tags: prev.tags.filter(t => t !== tag) }));
  };

  const handleAddDomainTag = () => {
    const tag = domainTagInput.trim().toLowerCase();
    if (tag && !formData.domain_tags.includes(tag)) {
      setFormData(prev => ({ ...prev, domain_tags: [...prev.domain_tags, tag] }));
      setDomainTagInput('');
    }
  };

  const handleRemoveDomainTag = (tag: string) => {
    setFormData(prev => ({ ...prev, domain_tags: prev.domain_tags.filter(t => t !== tag) }));
  };

  const handleTagKeyDown = (e: React.KeyboardEvent, type: 'tags' | 'domain_tags') => {
    if (e.key === 'Enter') {
      e.preventDefault();
      if (type === 'tags') {
        handleAddTag();
      } else {
        handleAddDomainTag();
      }
    }
  };

  const handleSave = async () => {
    if (!skill) return;

    if (!formData.display_name.trim()) {
      setMessage({ type: 'error', text: '请输入显示名称' });
      return;
    }
    if (!formData.content.trim()) {
      setMessage({ type: 'error', text: '请输入 Skill 内容' });
      return;
    }

    try {
      setSaving(true);
      const updated = await updateSkill(skill.id, {
        display_name: formData.display_name,
        description: formData.description,
        content: formData.content,
        version: formData.version,
        priority: formData.priority,
        metadata: {
          tags: formData.tags,
          domain_tags: formData.domain_tags,
        },
      });
      setSkill(updated);
      resetFormData(updated);
      setIsEditing(false);
      setMessage({ type: 'success', text: '保存成功' });
    } catch (err: any) {
      setMessage({ type: 'error', text: err.response?.data?.error || '保存失败' });
    } finally {
      setSaving(false);
    }
  };

  const handleToggleActive = async () => {
    if (!skill) return;
    try {
      if (skill.is_active) {
        const updated = await deactivateSkill(skill.id);
        setSkill(updated);
        setMessage({ type: 'success', text: `${skill.display_name} 已停用` });
      } else {
        const updated = await activateSkill(skill.id);
        setSkill(updated);
        setMessage({ type: 'success', text: `${skill.display_name} 已激活` });
      }
    } catch (err: any) {
      setMessage({ type: 'error', text: err.response?.data?.error || '操作失败' });
    }
  };

  const handleDeleteConfirm = async () => {
    if (!skill) return;
    try {
      await deleteSkill(skill.id, true);
      setMessage({ type: 'success', text: '删除成功' });
      setDeleteConfirm(false);
      // 返回列表页
      setTimeout(() => {
        navigate('/global/settings/ai-configs?tab=skills');
      }, 500);
    } catch (err: any) {
      setMessage({ type: 'error', text: err.response?.data?.error || '删除失败' });
    }
  };

  if (loading) {
    return (
      <div className={styles.container}>
        <div className={styles.loading}>加载中...</div>
      </div>
    );
  }

  if (error || !skill) {
    return (
      <div className={styles.container}>
        <div className={styles.errorPage}>
          <p>{error || 'Skill 不存在'}</p>
          <button
            className={styles.backBtn}
            onClick={() => navigate('/global/settings/ai-configs?tab=skills')}
          >
            返回列表
          </button>
        </div>
      </div>
    );
  }

  return (
    <div className={styles.container}>
      {/* 顶部导航 */}
      <div className={styles.breadcrumb}>
        <button
          className={styles.backLink}
          onClick={() => navigate('/global/settings/ai-configs?tab=skills')}
        >
          ← AI Skills
        </button>
        <span className={styles.breadcrumbSep}>/</span>
        <span className={styles.breadcrumbCurrent}>{skill.display_name}</span>
      </div>

      {/* 消息提示 */}
      {message && (
        <div className={`${styles.message} ${styles[message.type]}`}>
          {message.text}
          <button className={styles.messageClose} onClick={() => setMessage(null)}>×</button>
        </div>
      )}

      {/* 头部信息 */}
      <div className={styles.header}>
        <div className={styles.headerLeft}>
          <div className={styles.titleRow}>
            <span className={`${styles.layerBadge} ${styles[`layer_${skill.layer}`]}`}>
              {LAYER_LABELS[skill.layer]}
            </span>
            {isEditing ? (
              <input
                type="text"
                className={styles.titleInput}
                value={formData.display_name}
                onChange={e => handleChange('display_name', e.target.value)}
                placeholder="显示名称"
              />
            ) : (
              <h1 className={styles.title}>{skill.display_name}</h1>
            )}
            {!skill.is_active && <span className={styles.inactiveBadge}>已停用</span>}
            {skill.source_type !== 'manual' && (
              <span className={styles.sourceBadge}>
                {SOURCE_TYPE_LABELS[skill.source_type]}
              </span>
            )}
          </div>
          <div className={styles.metaRow}>
            <span className={styles.metaItem}>
              <span className={styles.metaLabel}>名称:</span> {skill.name}
            </span>
            <span className={styles.metaItem}>
              <span className={styles.metaLabel}>版本:</span>{' '}
              {isEditing ? (
                <input
                  type="text"
                  className={styles.inlineInput}
                  value={formData.version}
                  onChange={e => handleChange('version', e.target.value)}
                />
              ) : (
                skill.version
              )}
            </span>
            <span className={styles.metaItem}>
              <span className={styles.metaLabel}>优先级:</span>{' '}
              {isEditing ? (
                <input
                  type="number"
                  className={styles.inlineInput}
                  style={{ width: '60px' }}
                  value={formData.priority}
                  onChange={e => handleChange('priority', parseInt(e.target.value) || 0)}
                  min={0}
                  max={100}
                />
              ) : (
                skill.priority
              )}
            </span>
            <span className={styles.metaItem}>
              <span className={styles.metaLabel}>创建时间:</span>{' '}
              {new Date(skill.created_at).toLocaleString('zh-CN')}
            </span>
            <span className={styles.metaItem}>
              <span className={styles.metaLabel}>更新时间:</span>{' '}
              {new Date(skill.updated_at).toLocaleString('zh-CN')}
            </span>
          </div>
        </div>
        <div className={styles.headerActions}>
          {isEditing ? (
            <>
              <button className={styles.cancelBtn} onClick={handleCancelEdit}>
                取消
              </button>
              <button className={styles.saveBtn} onClick={handleSave} disabled={saving}>
                {saving ? '保存中...' : '保存'}
              </button>
            </>
          ) : (
            <>
              <button className={styles.editBtn} onClick={handleStartEdit}>
                编辑
              </button>
              <button
                className={`${styles.toggleBtn} ${skill.is_active ? styles.toggleBtnDeactivate : styles.toggleBtnActivate}`}
                onClick={handleToggleActive}
              >
                {skill.is_active ? '停用' : '激活'}
              </button>
              {skill.source_type === 'manual' && (
                <button
                  className={styles.deleteBtn}
                  onClick={() => setDeleteConfirm(true)}
                >
                  删除
                </button>
              )}
            </>
          )}
        </div>
      </div>

      {/* 描述 */}
      <div className={styles.section}>
        <h3 className={styles.sectionTitle}>描述</h3>
        {isEditing ? (
          <div className={styles.fieldGroup}>
            <input
              type="text"
              className={styles.descInput}
              value={formData.description}
              onChange={e => handleChange('description', e.target.value)}
              placeholder="简短描述 Skill 的用途和适用场景（用于 AI 智能选择）"
              maxLength={500}
            />
            <span className={styles.hint}>{formData.description.length}/500 字符</span>
          </div>
        ) : (
          <p className={styles.description}>
            {skill.description || <span className={styles.emptyText}>暂无描述</span>}
          </p>
        )}
      </div>

      {/* 标签 - Domain Skill */}
      {(skill.layer === 'domain' || (isEditing && formData.tags.length > 0)) && (
        <div className={styles.section}>
          <h3 className={styles.sectionTitle}>标签 (tags)</h3>
          {isEditing ? (
            <div className={styles.fieldGroup}>
              <span className={styles.hint}>用于被 Task Skill 发现，多个标签用回车分隔</span>
              <div className={styles.tagContainer}>
                {formData.tags.map(tag => (
                  <span key={tag} className={styles.tag}>
                    {tag}
                    <button type="button" onClick={() => handleRemoveTag(tag)}>×</button>
                  </span>
                ))}
                <input
                  type="text"
                  value={tagInput}
                  onChange={e => setTagInput(e.target.value)}
                  onKeyDown={e => handleTagKeyDown(e, 'tags')}
                  placeholder="输入标签后按回车"
                  className={styles.tagInput}
                />
              </div>
            </div>
          ) : (
            <div className={styles.tagList}>
              {(skill.metadata?.tags || []).length > 0 ? (
                (skill.metadata?.tags || []).map(tag => (
                  <span key={tag} className={styles.tagReadonly}>{tag}</span>
                ))
              ) : (
                <span className={styles.emptyText}>暂无标签</span>
              )}
            </div>
          )}
        </div>
      )}

      {/* Domain Tags - Task Skill */}
      {(skill.layer === 'task' || (isEditing && formData.domain_tags.length > 0)) && (
        <div className={styles.section}>
          <h3 className={styles.sectionTitle}>需要的领域标签 (domain_tags)</h3>
          {isEditing ? (
            <div className={styles.fieldGroup}>
              <span className={styles.hint}>用于自动发现 Domain Skills，多个标签用回车分隔</span>
              <div className={styles.tagContainer}>
                {formData.domain_tags.map(tag => (
                  <span key={tag} className={styles.tag}>
                    {tag}
                    <button type="button" onClick={() => handleRemoveDomainTag(tag)}>×</button>
                  </span>
                ))}
                <input
                  type="text"
                  value={domainTagInput}
                  onChange={e => setDomainTagInput(e.target.value)}
                  onKeyDown={e => handleTagKeyDown(e, 'domain_tags')}
                  placeholder="输入标签后按回车"
                  className={styles.tagInput}
                />
              </div>
            </div>
          ) : (
            <div className={styles.tagList}>
              {(skill.metadata?.domain_tags || []).length > 0 ? (
                (skill.metadata?.domain_tags || []).map(tag => (
                  <span key={tag} className={styles.tagReadonly}>{tag}</span>
                ))
              ) : (
                <span className={styles.emptyText}>暂无领域标签</span>
              )}
            </div>
          )}
        </div>
      )}

      {/* 内容 */}
      <div className={styles.section}>
        <h3 className={styles.sectionTitle}>内容 (Markdown)</h3>
        {isEditing ? (
          <textarea
            className={styles.contentEditor}
            value={formData.content}
            onChange={e => handleChange('content', e.target.value)}
            placeholder="# Skill 标题&#10;&#10;## 说明&#10;..."
            rows={30}
          />
        ) : (
          <div className={styles.contentView}>
            <ReactMarkdown remarkPlugins={[remarkGfm]}>{stripFrontmatter(skill.content)}</ReactMarkdown>
          </div>
        )}
      </div>

      {/* 删除确认 */}
      <ConfirmDialog
        isOpen={deleteConfirm}
        title="删除 Skill"
        message={`确定要删除 "${skill.display_name}" 吗？此操作不可恢复。`}
        confirmText="删除"
        cancelText="取消"
        type="danger"
        onConfirm={handleDeleteConfirm}
        onCancel={() => setDeleteConfirm(false)}
      />
    </div>
  );
};

export default SkillDetail;
import { useState } from 'react';
import { useNavigate } from 'react-router-dom';
import {
  createSkill,
  type SkillLayer,
  type CreateSkillRequest,
  LAYER_LABELS,
} from '../services/skill';
import styles from './SkillDetail.module.css';

const SkillCreate = () => {
  const navigate = useNavigate();
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const [formData, setFormData] = useState<{
    name: string;
    display_name: string;
    description: string;
    layer: SkillLayer;
    content: string;
    version: string;
    priority: number;
    tags: string[];
    domain_tags: string[];
  }>({
    name: '',
    display_name: '',
    description: '',
    layer: 'domain',
    content: '',
    version: '1.0.0',
    priority: 0,
    tags: [],
    domain_tags: [],
  });

  const [tagInput, setTagInput] = useState('');
  const [domainTagInput, setDomainTagInput] = useState('');

  const handleChange = (field: string, value: string | number | string[]) => {
    setFormData(prev => ({ ...prev, [field]: value }));
    setError(null);
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
    if (!formData.name.trim()) {
      setError('请输入 Skill 名称');
      return;
    }
    if (!formData.display_name.trim()) {
      setError('请输入显示名称');
      return;
    }
    if (!formData.content.trim()) {
      setError('请输入 Skill 内容');
      return;
    }

    try {
      setSaving(true);
      const request: CreateSkillRequest = {
        name: formData.name,
        display_name: formData.display_name,
        description: formData.description,
        layer: formData.layer,
        content: formData.content,
        version: formData.version,
        priority: formData.priority,
        metadata: {
          tags: formData.tags,
          domain_tags: formData.domain_tags,
        },
      };
      const created = await createSkill(request);
      // 创建成功后跳转到详情页
      navigate(`/global/settings/skills/${created.id}`);
    } catch (err: any) {
      setError(err.response?.data?.error || '创建失败');
    } finally {
      setSaving(false);
    }
  };

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
        <span className={styles.breadcrumbCurrent}>新建 Skill</span>
      </div>

      {/* 错误提示 */}
      {error && (
        <div className={`${styles.message} ${styles.error}`}>
          {error}
          <button className={styles.messageClose} onClick={() => setError(null)}>×</button>
        </div>
      )}

      {/* 头部 */}
      <div className={styles.header}>
        <div className={styles.headerLeft}>
          <div className={styles.titleRow}>
            <h1 className={styles.title}>新建 Skill</h1>
          </div>
        </div>
        <div className={styles.headerActions}>
          <button
            className={styles.cancelBtn}
            onClick={() => navigate('/global/settings/ai-configs?tab=skills')}
          >
            取消
          </button>
          <button className={styles.saveBtn} onClick={handleSave} disabled={saving}>
            {saving ? '创建中...' : '创建'}
          </button>
        </div>
      </div>

      {/* 基本信息 */}
      <div className={styles.section}>
        <h3 className={styles.sectionTitle}>基本信息</h3>
        <div className={styles.fieldGroup} style={{ gap: '16px' }}>
          <div style={{ display: 'flex', gap: '16px' }}>
            <div style={{ flex: 1 }}>
              <label style={{ display: 'block', fontSize: '14px', fontWeight: 500, color: '#333', marginBottom: '6px' }}>
                名称 (唯一标识) *
              </label>
              <input
                type="text"
                className={styles.descInput}
                value={formData.name}
                onChange={e => handleChange('name', e.target.value)}
                placeholder="例如: custom_domain_skill"
              />
            </div>
            <div style={{ flex: 1 }}>
              <label style={{ display: 'block', fontSize: '14px', fontWeight: 500, color: '#333', marginBottom: '6px' }}>
                显示名称 *
              </label>
              <input
                type="text"
                className={styles.descInput}
                value={formData.display_name}
                onChange={e => handleChange('display_name', e.target.value)}
                placeholder="例如: 自定义领域知识"
              />
            </div>
          </div>
          <div style={{ display: 'flex', gap: '16px' }}>
            <div style={{ flex: 1 }}>
              <label style={{ display: 'block', fontSize: '14px', fontWeight: 500, color: '#333', marginBottom: '6px' }}>
                层级
              </label>
              <select
                className={styles.descInput}
                value={formData.layer}
                onChange={e => handleChange('layer', e.target.value)}
              >
                {Object.entries(LAYER_LABELS).map(([value, label]) => (
                  <option key={value} value={value}>{label}</option>
                ))}
              </select>
            </div>
            <div style={{ flex: 1 }}>
              <label style={{ display: 'block', fontSize: '14px', fontWeight: 500, color: '#333', marginBottom: '6px' }}>
                版本
              </label>
              <input
                type="text"
                className={styles.descInput}
                value={formData.version}
                onChange={e => handleChange('version', e.target.value)}
                placeholder="1.0.0"
              />
            </div>
            <div style={{ flex: 1 }}>
              <label style={{ display: 'block', fontSize: '14px', fontWeight: 500, color: '#333', marginBottom: '6px' }}>
                优先级
              </label>
              <input
                type="number"
                className={styles.descInput}
                value={formData.priority}
                onChange={e => handleChange('priority', parseInt(e.target.value) || 0)}
                min={0}
                max={100}
              />
              <span className={styles.hint}>数字越小越先加载</span>
            </div>
          </div>
        </div>
      </div>

      {/* 描述 */}
      <div className={styles.section}>
        <h3 className={styles.sectionTitle}>描述</h3>
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
      </div>

      {/* 标签 - Domain Skill */}
      {formData.layer === 'domain' && (
        <div className={styles.section}>
          <h3 className={styles.sectionTitle}>标签 (tags)</h3>
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
        </div>
      )}

      {/* Domain Tags - Task Skill */}
      {formData.layer === 'task' && (
        <div className={styles.section}>
          <h3 className={styles.sectionTitle}>需要的领域标签 (domain_tags)</h3>
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
        </div>
      )}

      {/* 内容 */}
      <div className={styles.section}>
        <h3 className={styles.sectionTitle}>内容 (Markdown) *</h3>
        <textarea
          className={styles.contentEditor}
          value={formData.content}
          onChange={e => handleChange('content', e.target.value)}
          placeholder="# Skill 标题&#10;&#10;## 说明&#10;..."
          rows={30}
        />
      </div>
    </div>
  );
};

export default SkillCreate;
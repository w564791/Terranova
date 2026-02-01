import { useState, useEffect } from 'react';
import { 
  createSkill, 
  updateSkill, 
  type Skill, 
  type SkillLayer,
  type CreateSkillRequest,
  LAYER_LABELS 
} from '../services/skill';
import styles from './SkillEditor.module.css';

interface SkillEditorProps {
  skill: Skill | null;
  onClose: (saved: boolean) => void;
  readOnly?: boolean;
}

const SkillEditor = ({ skill, onClose, readOnly = false }: SkillEditorProps) => {
  const isEditing = !!skill;
  
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
  
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (skill) {
      setFormData({
        name: skill.name,
        display_name: skill.display_name,
        description: skill.description || '',
        layer: skill.layer,
        content: skill.content,
        version: skill.version,
        priority: skill.priority,
        tags: skill.metadata?.tags || [],
        domain_tags: skill.metadata?.domain_tags || [],
      });
    }
  }, [skill]);

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

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    
    // 验证
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
      
      if (isEditing) {
        await updateSkill(skill.id, {
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
      } else {
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
        await createSkill(request);
      }
      
      onClose(true);
    } catch (err: any) {
      setError(err.response?.data?.error || '保存失败');
    } finally {
      setSaving(false);
    }
  };

  return (
    <div className={styles.overlay} onClick={() => onClose(false)}>
      <div className={styles.modal} onClick={e => e.stopPropagation()}>
        <div className={styles.header}>
          <h2>{readOnly ? '查看 Skill' : (isEditing ? '编辑 Skill' : '新建 Skill')}</h2>
          <button className={styles.closeBtn} onClick={() => onClose(false)}>×</button>
        </div>
        
        <form onSubmit={handleSubmit} className={styles.form}>
          {error && <div className={styles.error}>{error}</div>}
          
          <div className={styles.row}>
            <div className={styles.field}>
              <label>名称 (唯一标识)</label>
              <input
                type="text"
                value={formData.name}
                onChange={e => handleChange('name', e.target.value)}
                placeholder="例如: custom_domain_skill"
                disabled={isEditing || readOnly}
              />
              {isEditing && !readOnly && <span className={styles.hint}>名称不可修改</span>}
            </div>
            
            <div className={styles.field}>
              <label>显示名称</label>
              <input
                type="text"
                value={formData.display_name}
                onChange={e => handleChange('display_name', e.target.value)}
                placeholder="例如: 自定义领域知识"
                disabled={readOnly}
              />
            </div>
          </div>
          
          <div className={styles.field}>
            <label>描述</label>
            <input
              type="text"
              value={formData.description}
              onChange={e => handleChange('description', e.target.value)}
              placeholder="简短描述 Skill 的用途和适用场景（用于 AI 智能选择）"
              maxLength={500}
              disabled={readOnly}
            />
            {!readOnly && (
              <span className={styles.hint}>
                {formData.description.length}/500 字符，用于 AI 智能选择 Domain Skills
              </span>
            )}
          </div>
          
          <div className={styles.row}>
            <div className={styles.field}>
              <label>层级</label>
              <select
                value={formData.layer}
                onChange={e => handleChange('layer', e.target.value)}
                disabled={isEditing || readOnly}
              >
                {Object.entries(LAYER_LABELS).map(([value, label]) => (
                  <option key={value} value={value}>{label}</option>
                ))}
              </select>
              {isEditing && !readOnly && <span className={styles.hint}>层级不可修改</span>}
            </div>
            
            <div className={styles.field}>
              <label>版本</label>
              <input
                type="text"
                value={formData.version}
                onChange={e => handleChange('version', e.target.value)}
                placeholder="1.0.0"
                disabled={readOnly}
              />
            </div>
            
            <div className={styles.field}>
              <label>优先级</label>
              <input
                type="number"
                value={formData.priority}
                onChange={e => handleChange('priority', parseInt(e.target.value) || 0)}
                min={0}
                max={100}
                disabled={readOnly}
              />
              {!readOnly && <span className={styles.hint}>数字越小越先加载</span>}
            </div>
          </div>
          
          {/* Domain Skill: 显示 tags 输入 */}
          {formData.layer === 'domain' && (
            <div className={styles.field}>
              <label>标签 (tags)</label>
              {!readOnly && <span className={styles.hint}>用于被 Task Skill 发现，多个标签用回车分隔</span>}
              <div className={styles.tagContainer}>
                {formData.tags.map(tag => (
                  <span key={tag} className={styles.tag}>
                    {tag}
                    {!readOnly && <button type="button" onClick={() => handleRemoveTag(tag)}>×</button>}
                  </span>
                ))}
                {!readOnly && (
                  <input
                    type="text"
                    value={tagInput}
                    onChange={e => setTagInput(e.target.value)}
                    onKeyDown={e => handleTagKeyDown(e, 'tags')}
                    placeholder="输入标签后按回车"
                    className={styles.tagInput}
                  />
                )}
              </div>
            </div>
          )}

          {/* Task Skill: 显示 domain_tags 输入 */}
          {formData.layer === 'task' && (
            <div className={styles.field}>
              <label>需要的领域标签 (domain_tags)</label>
              {!readOnly && <span className={styles.hint}>用于自动发现 Domain Skills，多个标签用回车分隔</span>}
              <div className={styles.tagContainer}>
                {formData.domain_tags.map(tag => (
                  <span key={tag} className={styles.tag}>
                    {tag}
                    {!readOnly && <button type="button" onClick={() => handleRemoveDomainTag(tag)}>×</button>}
                  </span>
                ))}
                {!readOnly && (
                  <input
                    type="text"
                    value={domainTagInput}
                    onChange={e => setDomainTagInput(e.target.value)}
                    onKeyDown={e => handleTagKeyDown(e, 'domain_tags')}
                    placeholder="输入标签后按回车"
                    className={styles.tagInput}
                  />
                )}
              </div>
            </div>
          )}

          <div className={styles.field}>
            <label>内容 (Markdown)</label>
            <textarea
              value={formData.content}
              onChange={e => handleChange('content', e.target.value)}
              placeholder="# Skill 标题&#10;&#10;## 说明&#10;..."
              rows={20}
              disabled={readOnly}
            />
          </div>
          
          <div className={styles.actions}>
            {readOnly ? (
              <button 
                type="button" 
                className={styles.cancelBtn}
                onClick={() => onClose(false)}
              >
                关闭
              </button>
            ) : (
              <>
                <button 
                  type="button" 
                  className={styles.cancelBtn}
                  onClick={() => onClose(false)}
                >
                  取消
                </button>
                <button 
                  type="submit" 
                  className={styles.saveBtn}
                  disabled={saving}
                >
                  {saving ? '保存中...' : '保存'}
                </button>
              </>
            )}
          </div>
        </form>
      </div>
    </div>
  );
};

export default SkillEditor;
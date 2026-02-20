import { useState, useEffect } from 'react';
import { 
  listSkills, 
  deleteSkill, 
  activateSkill, 
  deactivateSkill,
  type Skill, 
  type SkillLayer,
  LAYER_LABELS,
  SOURCE_TYPE_LABELS 
} from '../../services/skill';
import ConfirmDialog from '../../components/ConfirmDialog';
import SkillEditor from '../../components/SkillEditor';
import styles from './SkillManagement.module.css';

const SkillManagement = () => {
  const [skills, setSkills] = useState<Skill[]>([]);
  const [loading, setLoading] = useState(true);
  const [activeLayer, setActiveLayer] = useState<SkillLayer | 'all'>('all');
  const [message, setMessage] = useState<{ type: 'success' | 'error'; text: string } | null>(null);
  const [deleteConfirm, setDeleteConfirm] = useState<{ show: boolean; skill: Skill | null }>({
    show: false,
    skill: null,
  });
  const [editingSkill, setEditingSkill] = useState<Skill | null>(null);
  const [showCreateModal, setShowCreateModal] = useState(false);

  useEffect(() => {
    loadSkills();
  }, [activeLayer]);

  const loadSkills = async () => {
    try {
      setLoading(true);
      const params = activeLayer !== 'all' ? { layer: activeLayer } : {};
      const response = await listSkills(params);
      setSkills(response.skills || []);
    } catch (error: any) {
      setMessage({
        type: 'error',
        text: error.response?.data?.error || '加载 Skill 列表失败',
      });
      setSkills([]);
    } finally {
      setLoading(false);
    }
  };

  const handleToggleActive = async (skill: Skill) => {
    try {
      if (skill.is_active) {
        await deactivateSkill(skill.id);
        setMessage({ type: 'success', text: `${skill.display_name} 已停用` });
      } else {
        await activateSkill(skill.id);
        setMessage({ type: 'success', text: `${skill.display_name} 已激活` });
      }
      loadSkills();
    } catch (error: any) {
      setMessage({
        type: 'error',
        text: error.response?.data?.error || '操作失败',
      });
    }
  };

  const handleDeleteClick = (skill: Skill) => {
    setDeleteConfirm({ show: true, skill });
  };

  const handleDeleteConfirm = async () => {
    if (!deleteConfirm.skill) return;
    try {
      // 传递 hard=true 进行真实删除，而非仅禁用
      await deleteSkill(deleteConfirm.skill.id, true);
      setMessage({ type: 'success', text: '删除成功' });
      setDeleteConfirm({ show: false, skill: null });
      loadSkills();
    } catch (error: any) {
      setMessage({
        type: 'error',
        text: error.response?.data?.error || '删除失败',
      });
    }
  };

  const handleEditClick = (skill: Skill) => {
    setEditingSkill(skill);
  };

  const handleEditorClose = (saved: boolean) => {
    setEditingSkill(null);
    setShowCreateModal(false);
    if (saved) {
      loadSkills();
      setMessage({ type: 'success', text: '保存成功' });
    }
  };

  // 按层级分组
  const groupedSkills = {
    foundation: skills.filter(s => s.layer === 'foundation'),
    domain: skills.filter(s => s.layer === 'domain'),
    task: skills.filter(s => s.layer === 'task'),
  };

  const renderSkillCard = (skill: Skill) => (
    <div key={skill.id} className={`${styles.skillCard} ${!skill.is_active ? styles.inactive : ''}`}>
      <div className={styles.skillHeader}>
        <div className={styles.skillTitle}>
          <span className={`${styles.layerBadge} ${styles[skill.layer]}`}>
            {LAYER_LABELS[skill.layer]}
          </span>
          <span className={styles.skillName}>{skill.display_name}</span>
          {skill.source_type !== 'manual' && (
            <span className={styles.sourceBadge}>
              {SOURCE_TYPE_LABELS[skill.source_type]}
            </span>
          )}
        </div>
        <div className={styles.skillActions}>
          <button 
            className={styles.actionBtn}
            onClick={() => handleEditClick(skill)}
            title="编辑"
          >
            ✏️
          </button>
          <button 
            className={`${styles.actionBtn} ${skill.is_active ? styles.active : ''}`}
            onClick={() => handleToggleActive(skill)}
            title={skill.is_active ? '停用' : '激活'}
          >
            {skill.is_active ? '🟢' : '⚪'}
          </button>
          {skill.source_type === 'manual' && (
            <button 
              className={`${styles.actionBtn} ${styles.danger}`}
              onClick={() => handleDeleteClick(skill)}
              title="删除"
            >
              🗑️
            </button>
          )}
        </div>
      </div>
      <div className={styles.skillMeta}>
        <span className={styles.metaItem}>
          <span className={styles.metaLabel}>名称:</span> {skill.name}
        </span>
        <span className={styles.metaItem}>
          <span className={styles.metaLabel}>版本:</span> {skill.version}
        </span>
        <span className={styles.metaItem}>
          <span className={styles.metaLabel}>优先级:</span> {skill.priority}
        </span>
      </div>
      <div className={styles.skillContent}>
        <pre>{skill.content.substring(0, 200)}...</pre>
      </div>
    </div>
  );

  if (loading) {
    return <div className={styles.loading}>加载中...</div>;
  }

  return (
    <div className={styles.container}>
      <div className={styles.header}>
        <h1 className={styles.title}>Skill 管理</h1>
        <button 
          className={styles.createBtn}
          onClick={() => setShowCreateModal(true)}
        >
          + 新建 Skill
        </button>
      </div>

      {message && (
        <div className={`${styles.message} ${styles[message.type]}`}>
          {message.text}
          <button onClick={() => setMessage(null)}>×</button>
        </div>
      )}

      <div className={styles.tabs}>
        <button 
          className={`${styles.tab} ${activeLayer === 'all' ? styles.active : ''}`}
          onClick={() => setActiveLayer('all')}
        >
          全部 ({skills.length})
        </button>
        <button 
          className={`${styles.tab} ${activeLayer === 'foundation' ? styles.active : ''}`}
          onClick={() => setActiveLayer('foundation')}
        >
          基础层 ({groupedSkills.foundation.length})
        </button>
        <button 
          className={`${styles.tab} ${activeLayer === 'domain' ? styles.active : ''}`}
          onClick={() => setActiveLayer('domain')}
        >
          领域层 ({groupedSkills.domain.length})
        </button>
        <button 
          className={`${styles.tab} ${activeLayer === 'task' ? styles.active : ''}`}
          onClick={() => setActiveLayer('task')}
        >
          任务层 ({groupedSkills.task.length})
        </button>
      </div>

      {activeLayer === 'all' ? (
        <>
          {groupedSkills.foundation.length > 0 && (
            <div className={styles.section}>
              <h2 className={styles.sectionTitle}>
                🏛️ 基础层 (Foundation)
                <span className={styles.sectionDesc}>最通用的基础知识，所有功能复用</span>
              </h2>
              <div className={styles.skillList}>
                {groupedSkills.foundation.map(renderSkillCard)}
              </div>
            </div>
          )}

          {groupedSkills.domain.length > 0 && (
            <div className={styles.section}>
              <h2 className={styles.sectionTitle}>
                🎯 领域层 (Domain)
                <span className={styles.sectionDesc}>专业领域知识，部分功能复用</span>
              </h2>
              <div className={styles.skillList}>
                {groupedSkills.domain.map(renderSkillCard)}
              </div>
            </div>
          )}

          {groupedSkills.task.length > 0 && (
            <div className={styles.section}>
              <h2 className={styles.sectionTitle}>
                ⚡ 任务层 (Task)
                <span className={styles.sectionDesc}>特定功能的专属工作流程</span>
              </h2>
              <div className={styles.skillList}>
                {groupedSkills.task.map(renderSkillCard)}
              </div>
            </div>
          )}
        </>
      ) : (
        <div className={styles.skillList}>
          {skills.map(renderSkillCard)}
        </div>
      )}

      {skills.length === 0 && (
        <div className={styles.empty}>
          <p>暂无 Skill</p>
          <button 
            className={styles.createBtn}
            onClick={() => setShowCreateModal(true)}
          >
            创建第一个 Skill
          </button>
        </div>
      )}

      {/* 编辑/创建弹窗 */}
      {(editingSkill || showCreateModal) && (
        <SkillEditor
          skill={editingSkill}
          onClose={handleEditorClose}
        />
      )}

      {/* 删除确认弹窗 */}
      <ConfirmDialog
        isOpen={deleteConfirm.show}
        title="删除 Skill"
        message={`确定要删除 "${deleteConfirm.skill?.display_name}" 吗？此操作不可恢复。`}
        confirmText="删除"
        cancelText="取消"
        type="danger"
        onConfirm={handleDeleteConfirm}
        onCancel={() => setDeleteConfirm({ show: false, skill: null })}
      />
    </div>
  );
};

export default SkillManagement;
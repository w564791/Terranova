import { useState, useEffect } from 'react';
import { useNavigate, Link, useSearchParams } from 'react-router-dom';
import { 
  listAIConfigs, 
  deleteAIConfig, 
  batchUpdatePriorities,
  type AIConfig, 
  type PriorityUpdate,
  CAPABILITY_LABELS 
} from '../services/ai';
import { 
  listSkills, 
  deleteSkill, 
  activateSkill, 
  deactivateSkill,
  type Skill, 
  type SkillLayer,
  LAYER_LABELS,
  SOURCE_TYPE_LABELS 
} from '../services/skill';
import ConfirmDialog from '../components/ConfirmDialog';
import SkillEditor from '../components/SkillEditor';
import {
  DndContext,
  closestCenter,
  KeyboardSensor,
  PointerSensor,
  useSensor,
  useSensors,
  type DragEndEvent,
} from '@dnd-kit/core';
import {
  arrayMove,
  SortableContext,
  sortableKeyboardCoordinates,
  verticalListSortingStrategy,
  useSortable,
} from '@dnd-kit/sortable';
import { CSS } from '@dnd-kit/utilities';
import styles from './AIConfigList.module.css';

// 可排序的配置卡片组件
const SortableConfigCard = ({ config }: { config: AIConfig }) => {
  const {
    attributes,
    listeners,
    setNodeRef,
    transform,
    transition,
    isDragging,
  } = useSortable({ id: config.id });

  const style = {
    transform: CSS.Transform.toString(transform),
    transition,
    opacity: isDragging ? 0.5 : 1,
  };

  return (
    <Link 
      ref={setNodeRef}
      style={style}
      className={styles.card}
      to={`/global/settings/ai-configs/${config.id}/edit`}
    >
      <div className={styles.cardHeader}>
        <div className={styles.cardTitle} style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
          {!config.enabled && (
            <span 
              {...attributes} 
              {...listeners}
              onClick={(e) => e.stopPropagation()}
              style={{ 
                cursor: 'grab', 
                fontSize: '18px', 
                color: '#999',
                userSelect: 'none',
              }}
            >
              ⋮⋮
            </span>
          )}
          <span className={styles.serviceType}>{config.service_type.toUpperCase()}</span>
          {config.enabled && (
            <span className={styles.enabledBadge}>默认</span>
          )}
          {!config.enabled && config.capabilities && config.capabilities.length > 0 && (
            <span className={styles.disabledBadge}>
              {config.capabilities.includes('*') 
                ? '全部支持' 
                : config.capabilities.map(c => CAPABILITY_LABELS[c] || c).join('、')}
            </span>
          )}
          {!config.enabled && (!config.capabilities || config.capabilities.length === 0) && (
            <span className={styles.disabledBadge}>未配置</span>
          )}
        </div>
      </div>
      <div className={styles.cardBody}>
        {config.service_type === 'bedrock' && config.aws_region && (
          <div className={styles.infoRow}>
            <span className={styles.label}>Region:</span>
            <span className={styles.value}>{config.aws_region}</span>
          </div>
        )}
        {(config.service_type === 'openai' || 
          config.service_type === 'azure_openai' || 
          config.service_type === 'ollama') && config.base_url && (
          <div className={styles.infoRow}>
            <span className={styles.label}>Base URL:</span>
            <span className={styles.value}>{config.base_url}</span>
          </div>
        )}
        <div className={styles.infoRow}>
          <span className={styles.label}>Model:</span>
          <span className={styles.value}>{config.model_id}</span>
        </div>
        <div className={styles.infoRow}>
          <span className={styles.label}>频率限制:</span>
          <span className={styles.value}>{config.rate_limit_seconds} 秒</span>
        </div>
        {!config.enabled && (
          <div className={styles.infoRow}>
            <span className={styles.label}>优先级:</span>
            <span className={styles.value}>{config.priority}</span>
          </div>
        )}
        <div className={styles.infoRow}>
          <span className={styles.label}>创建时间:</span>
          <span className={styles.value}>
            {new Date(config.created_at).toLocaleString('zh-CN')}
          </span>
        </div>
      </div>
    </Link>
  );
};

// AI 配置列表组件
const ConfigsTab = () => {
  const navigate = useNavigate();
  const [configs, setConfigs] = useState<AIConfig[]>([]);
  const [loading, setLoading] = useState(true);
  const [message, setMessage] = useState<{ type: 'success' | 'error'; text: string } | null>(null);
  const [deleteConfirm, setDeleteConfirm] = useState<{ show: boolean; id: number | null }>({
    show: false,
    id: null,
  });
  const [deleting, setDeleting] = useState(false);

  const sensors = useSensors(
    useSensor(PointerSensor),
    useSensor(KeyboardSensor, {
      coordinateGetter: sortableKeyboardCoordinates,
    })
  );

  useEffect(() => {
    loadConfigs();
  }, []);

  const loadConfigs = async () => {
    try {
      setLoading(true);
      const data = await listAIConfigs();
      setConfigs(data || []);
    } catch (error: any) {
      setMessage({
        type: 'error',
        text: error.response?.data?.message || '加载配置列表失败',
      });
      setConfigs([]);
    } finally {
      setLoading(false);
    }
  };

  const handleDeleteConfirm = async () => {
    if (!deleteConfirm.id) return;
    try {
      setDeleting(true);
      await deleteAIConfig(deleteConfirm.id);
      setMessage({ type: 'success', text: '配置删除成功' });
      setDeleteConfirm({ show: false, id: null });
      loadConfigs();
    } catch (error: any) {
      setMessage({
        type: 'error',
        text: error.response?.data?.message || '删除配置失败',
      });
    } finally {
      setDeleting(false);
    }
  };

  const handleDragEnd = async (event: DragEndEvent) => {
    const { active, over } = event;
    if (!over || active.id === over.id) return;

    const oldIndex = configs.findIndex((c) => c.id === active.id);
    const newIndex = configs.findIndex((c) => c.id === over.id);
    if (oldIndex === -1 || newIndex === -1) return;

    const newConfigs = arrayMove(configs, oldIndex, newIndex);
    setConfigs(newConfigs);

    const nonDefaultConfigs = newConfigs.filter(c => !c.enabled);
    const updates: PriorityUpdate[] = nonDefaultConfigs.map((config, index) => ({
      id: config.id,
      priority: (nonDefaultConfigs.length - index) * 10,
    }));

    try {
      await batchUpdatePriorities(updates);
      setMessage({ type: 'success', text: '优先级更新成功' });
      setTimeout(() => loadConfigs(), 500);
    } catch (error: any) {
      setMessage({
        type: 'error',
        text: error.response?.data?.message || '更新优先级失败',
      });
      loadConfigs();
    }
  };

  const defaultConfigs = configs.filter(c => c.enabled);
  const specialConfigs = configs.filter(c => !c.enabled).sort((a, b) => {
    if (b.priority !== a.priority) return b.priority - a.priority;
    return a.id - b.id;
  });

  if (loading) {
    return <div className={styles.loading}>加载中...</div>;
  }

  return (
    <>
      <div className={styles.tabHeader}>
        <button
          className={styles.createButton}
          onClick={() => navigate('/global/settings/ai-configs/create')}
        >
          + 新增配置
        </button>
      </div>

      {message && (
        <div className={`${styles.message} ${styles[message.type]}`}>
          {message.text}
        </div>
      )}

      {(!configs || configs.length === 0) ? (
        <div className={styles.empty}>
          <p>暂无 AI 配置</p>
          <button
            className={styles.createButton}
            onClick={() => navigate('/global/settings/ai-configs/create')}
          >
            创建第一个配置
          </button>
        </div>
      ) : (
        <>
          {defaultConfigs.length > 0 && (
            <div style={{ marginBottom: '24px' }}>
              <h3 style={{ fontSize: '15px', fontWeight: 600, marginBottom: '12px', color: '#333' }}>
                默认配置（支持所有场景）
              </h3>
              <div className={styles.list}>
                {defaultConfigs.map((config) => (
                  <Link 
                    key={config.id} 
                    to={`/global/settings/ai-configs/${config.id}/edit`}
                    className={styles.card}
                  >
                    <div className={styles.cardHeader}>
                      <div className={styles.cardTitle}>
                        <span className={styles.serviceType}>{config.service_type.toUpperCase()}</span>
                        <span className={styles.enabledBadge}>默认</span>
                      </div>
                    </div>
                    <div className={styles.cardBody}>
                      {config.service_type === 'bedrock' && config.aws_region && (
                        <div className={styles.infoRow}>
                          <span className={styles.label}>Region:</span>
                          <span className={styles.value}>{config.aws_region}</span>
                        </div>
                      )}
                      {(config.service_type === 'openai' || 
                        config.service_type === 'azure_openai' || 
                        config.service_type === 'ollama') && config.base_url && (
                        <div className={styles.infoRow}>
                          <span className={styles.label}>Base URL:</span>
                          <span className={styles.value}>{config.base_url}</span>
                        </div>
                      )}
                      <div className={styles.infoRow}>
                        <span className={styles.label}>Model:</span>
                        <span className={styles.value}>{config.model_id}</span>
                      </div>
                      <div className={styles.infoRow}>
                        <span className={styles.label}>频率限制:</span>
                        <span className={styles.value}>{config.rate_limit_seconds} 秒</span>
                      </div>
                      <div className={styles.infoRow}>
                        <span className={styles.label}>创建时间:</span>
                        <span className={styles.value}>
                          {new Date(config.created_at).toLocaleString('zh-CN')}
                        </span>
                      </div>
                    </div>
                  </Link>
                ))}
              </div>
            </div>
          )}

          {specialConfigs.length > 0 && (
            <div>
              <h3 style={{ fontSize: '15px', fontWeight: 600, marginBottom: '12px', color: '#333' }}>
                专用配置（按优先级排序，可拖拽调整）
              </h3>
              <DndContext
                sensors={sensors}
                collisionDetection={closestCenter}
                onDragEnd={handleDragEnd}
              >
                <SortableContext
                  items={specialConfigs.map(c => c.id)}
                  strategy={verticalListSortingStrategy}
                >
                  <div className={styles.list}>
                    {specialConfigs.map((config) => (
                      <SortableConfigCard key={config.id} config={config} />
                    ))}
                  </div>
                </SortableContext>
              </DndContext>
            </div>
          )}
        </>
      )}

      <ConfirmDialog
        isOpen={deleteConfirm.show}
        title="删除 AI 配置"
        message="确定要删除此配置吗？删除后无法恢复。"
        confirmText="删除"
        cancelText="取消"
        type="danger"
        onConfirm={handleDeleteConfirm}
        onCancel={() => setDeleteConfirm({ show: false, id: null })}
        loading={deleting}
      />
    </>
  );
};

// 每个层级的分页状态
interface LayerPagination {
  skills: Skill[];
  total: number;
  page: number;
  pageSize: number;
  totalPages: number;
  loading: boolean;
}

// Skills 管理组件
const SkillsTab = () => {
  const [searchParams, setSearchParams] = useSearchParams();
  const [message, setMessage] = useState<{ type: 'success' | 'error'; text: string } | null>(null);
  const [deleteConfirm, setDeleteConfirm] = useState<{ show: boolean; skill: Skill | null }>({
    show: false,
    skill: null,
  });
  const [editingSkill, setEditingSkill] = useState<Skill | null>(null);
  const [viewingSkill, setViewingSkill] = useState<Skill | null>(null);
  const [showCreateModal, setShowCreateModal] = useState(false);

  // 搜索状态
  const [searchKeyword, setSearchKeyword] = useState('');
  const [searchResults, setSearchResults] = useState<Skill[]>([]);
  const [isSearching, setIsSearching] = useState(false);
  const [searchTotal, setSearchTotal] = useState(0);

  // 每个层级独立的分页状态
  const [foundationData, setFoundationData] = useState<LayerPagination>({
    skills: [], total: 0, page: 1, pageSize: 20, totalPages: 0, loading: true
  });
  const [domainData, setDomainData] = useState<LayerPagination>({
    skills: [], total: 0, page: 1, pageSize: 20, totalPages: 0, loading: true
  });
  const [taskData, setTaskData] = useState<LayerPagination>({
    skills: [], total: 0, page: 1, pageSize: 20, totalPages: 0, loading: true
  });

  // Get active layer from URL parameter
  const layerParam = searchParams.get('layer');
  const activeLayer: SkillLayer | 'all' = 
    layerParam === 'foundation' || layerParam === 'domain' || layerParam === 'task' 
      ? layerParam 
      : 'all';

  const handleLayerChange = (layer: SkillLayer | 'all') => {
    const newParams = new URLSearchParams(searchParams);
    newParams.set('tab', 'skills');
    if (layer === 'all') {
      newParams.delete('layer');
    } else {
      newParams.set('layer', layer);
    }
    setSearchParams(newParams);
  };

  // 加载指定层级的数据
  const loadLayerSkills = async (
    layer: SkillLayer, 
    page: number, 
    pageSize: number,
    setData: React.Dispatch<React.SetStateAction<LayerPagination>>
  ) => {
    try {
      setData(prev => ({ ...prev, loading: true }));
      const response = await listSkills({ layer, page, page_size: pageSize });
      setData({
        skills: response.skills || [],
        total: response.total,
        page: response.page,
        pageSize: response.page_size,
        totalPages: response.total_pages,
        loading: false
      });
    } catch (error: any) {
      setMessage({
        type: 'error',
        text: error.response?.data?.error || `加载 ${layer} 层 Skill 列表失败`,
      });
      setData(prev => ({ ...prev, loading: false, skills: [] }));
    }
  };

  // 初始加载所有层级
  useEffect(() => {
    loadLayerSkills('foundation', 1, 20, setFoundationData);
    loadLayerSkills('domain', 1, 20, setDomainData);
    loadLayerSkills('task', 1, 20, setTaskData);
  }, []);

  // 刷新所有层级
  const refreshAllLayers = () => {
    loadLayerSkills('foundation', foundationData.page, foundationData.pageSize, setFoundationData);
    loadLayerSkills('domain', domainData.page, domainData.pageSize, setDomainData);
    loadLayerSkills('task', taskData.page, taskData.pageSize, setTaskData);
  };

  // 处理分页变化
  const handlePageChange = (layer: SkillLayer, newPage: number) => {
    switch (layer) {
      case 'foundation':
        loadLayerSkills('foundation', newPage, foundationData.pageSize, setFoundationData);
        break;
      case 'domain':
        loadLayerSkills('domain', newPage, domainData.pageSize, setDomainData);
        break;
      case 'task':
        loadLayerSkills('task', newPage, taskData.pageSize, setTaskData);
        break;
    }
  };

  // 搜索功能
  const handleSearch = async () => {
    if (!searchKeyword.trim()) {
      setSearchResults([]);
      setIsSearching(false);
      setSearchTotal(0);
      return;
    }

    try {
      setIsSearching(true);
      const response = await listSkills({ search: searchKeyword.trim(), page_size: 100 });
      setSearchResults(response.skills || []);
      setSearchTotal(response.total);
    } catch (error: any) {
      setMessage({
        type: 'error',
        text: error.response?.data?.error || '搜索失败',
      });
      setSearchResults([]);
      setSearchTotal(0);
    }
  };

  // 清除搜索
  const clearSearch = () => {
    setSearchKeyword('');
    setSearchResults([]);
    setIsSearching(false);
    setSearchTotal(0);
  };

  // 监听搜索关键词变化（防抖）
  useEffect(() => {
    if (!searchKeyword.trim()) {
      setIsSearching(false);
      setSearchResults([]);
      setSearchTotal(0);
      return;
    }

    const timer = setTimeout(() => {
      handleSearch();
    }, 300);

    return () => clearTimeout(timer);
  }, [searchKeyword]);

  // 计算总数
  const totalCount = foundationData.total + domainData.total + taskData.total;
  const isLoading = foundationData.loading || domainData.loading || taskData.loading;

  // 根据当前层级获取显示数据
  const getDisplayData = (): { skills: Skill[]; pagination: LayerPagination | null; layer: SkillLayer | null } => {
    switch (activeLayer) {
      case 'foundation':
        return { skills: foundationData.skills, pagination: foundationData, layer: 'foundation' };
      case 'domain':
        return { skills: domainData.skills, pagination: domainData, layer: 'domain' };
      case 'task':
        return { skills: taskData.skills, pagination: taskData, layer: 'task' };
      default:
        return { skills: [], pagination: null, layer: null };
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
      refreshAllLayers();
    } catch (error: any) {
      setMessage({
        type: 'error',
        text: error.response?.data?.error || '操作失败',
      });
    }
  };

  const handleDeleteConfirm = async () => {
    if (!deleteConfirm.skill) return;
    try {
      // 传递 hard=true 进行真实删除，而非仅禁用
      await deleteSkill(deleteConfirm.skill.id, true);
      setMessage({ type: 'success', text: '删除成功' });
      setDeleteConfirm({ show: false, skill: null });
      refreshAllLayers();
    } catch (error: any) {
      setMessage({
        type: 'error',
        text: error.response?.data?.error || '删除失败',
      });
    }
  };

  const handleEditorClose = (saved: boolean) => {
    setEditingSkill(null);
    setShowCreateModal(false);
    if (saved) {
      refreshAllLayers();
      setMessage({ type: 'success', text: '保存成功' });
    }
  };

  // 渲染分页组件
  const renderPagination = (pagination: LayerPagination, layer: SkillLayer) => {
    if (pagination.totalPages <= 1) return null;
    
    return (
      <div className={styles.pagination} style={{ 
        display: 'flex', 
        justifyContent: 'center', 
        alignItems: 'center', 
        gap: '8px', 
        marginTop: '16px',
        padding: '12px 0'
      }}>
        <button 
          className={styles.paginationBtn}
          disabled={pagination.page <= 1}
          onClick={() => handlePageChange(layer, pagination.page - 1)}
          style={{
            padding: '6px 12px',
            border: '1px solid #d9d9d9',
            borderRadius: '4px',
            background: pagination.page <= 1 ? '#f5f5f5' : '#fff',
            cursor: pagination.page <= 1 ? 'not-allowed' : 'pointer',
            color: pagination.page <= 1 ? '#999' : '#333'
          }}
        >
          上一页
        </button>
        <span style={{ color: '#666', fontSize: '14px' }}>
          第 {pagination.page} / {pagination.totalPages} 页，共 {pagination.total} 条
        </span>
        <button 
          className={styles.paginationBtn}
          disabled={pagination.page >= pagination.totalPages}
          onClick={() => handlePageChange(layer, pagination.page + 1)}
          style={{
            padding: '6px 12px',
            border: '1px solid #d9d9d9',
            borderRadius: '4px',
            background: pagination.page >= pagination.totalPages ? '#f5f5f5' : '#fff',
            cursor: pagination.page >= pagination.totalPages ? 'not-allowed' : 'pointer',
            color: pagination.page >= pagination.totalPages ? '#999' : '#333'
          }}
        >
          下一页
        </button>
      </div>
    );
  };

  const renderSkillCard = (skill: Skill) => (
    <div 
      key={skill.id} 
      className={`${styles.skillCard} ${!skill.is_active ? styles.skillInactive : ''}`}
      onClick={() => setViewingSkill(skill)}
      style={{ cursor: 'pointer' }}
    >
      <div className={styles.skillHeader}>
        <div className={styles.skillTitle}>
          <span className={`${styles.layerBadge} ${styles[`layer_${skill.layer}`]}`}>
            {LAYER_LABELS[skill.layer]}
          </span>
          <span className={styles.skillName}>{skill.display_name}</span>
          {skill.source_type !== 'manual' && (
            <span className={styles.sourceBadge}>
              {SOURCE_TYPE_LABELS[skill.source_type]}
            </span>
          )}
        </div>
        <div className={styles.skillActions} onClick={(e) => e.stopPropagation()}>
          <button 
            className={styles.textBtn}
            onClick={() => setEditingSkill(skill)}
            title="编辑"
          >
            编辑
          </button>
          <button 
            className={`${styles.textBtn} ${skill.is_active ? styles.textBtnActive : styles.textBtnInactive}`}
            onClick={() => handleToggleActive(skill)}
            title={skill.is_active ? '停用' : '激活'}
          >
            {skill.is_active ? '停用' : '激活'}
          </button>
          {skill.source_type === 'manual' && (
            <button 
              className={`${styles.textBtn} ${styles.textBtnDanger}`}
              onClick={() => setDeleteConfirm({ show: true, skill })}
              title="删除"
            >
              删除
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

  // 渲染层级区块（带分页）
  const renderLayerSection = (
    title: string, 
    description: string, 
    data: LayerPagination, 
    layer: SkillLayer
  ) => {
    if (data.total === 0 && !data.loading) return null;
    
    return (
      <div className={styles.skillSection}>
        <h3 className={styles.skillSectionTitle}>
          {title}
          <span className={styles.skillSectionDesc}>{description}</span>
        </h3>
        {data.loading ? (
          <div style={{ padding: '20px', textAlign: 'center', color: '#999' }}>加载中...</div>
        ) : (
          <>
            <div className={styles.skillList}>
              {data.skills.map(renderSkillCard)}
            </div>
            {renderPagination(data, layer)}
          </>
        )}
      </div>
    );
  };

  // 获取当前层级的显示数据
  const { skills: displaySkills, pagination: currentPagination, layer: currentLayer } = getDisplayData();

  if (isLoading && foundationData.skills.length === 0 && domainData.skills.length === 0 && taskData.skills.length === 0) {
    return <div className={styles.loading}>加载中...</div>;
  }

  return (
    <>
      <div className={styles.tabHeader}>
        <div className={styles.layerTabs}>
          <button 
            className={`${styles.layerTab} ${activeLayer === 'all' && !isSearching ? styles.activeLayerTab : ''}`}
            onClick={() => { clearSearch(); handleLayerChange('all'); }}
          >
            全部 ({totalCount})
          </button>
          <button 
            className={`${styles.layerTab} ${activeLayer === 'foundation' && !isSearching ? styles.activeLayerTab : ''}`}
            onClick={() => { clearSearch(); handleLayerChange('foundation'); }}
          >
            基础层 ({foundationData.total})
          </button>
          <button 
            className={`${styles.layerTab} ${activeLayer === 'domain' && !isSearching ? styles.activeLayerTab : ''}`}
            onClick={() => { clearSearch(); handleLayerChange('domain'); }}
          >
            领域层 ({domainData.total})
          </button>
          <button 
            className={`${styles.layerTab} ${activeLayer === 'task' && !isSearching ? styles.activeLayerTab : ''}`}
            onClick={() => { clearSearch(); handleLayerChange('task'); }}
          >
            任务层 ({taskData.total})
          </button>
        </div>
        <div style={{ display: 'flex', gap: '12px', alignItems: 'center' }}>
          {/* 搜索框 */}
          <div style={{ position: 'relative', display: 'flex', alignItems: 'center' }}>
            <input
              type="text"
              placeholder="搜索 Skill（名称、内容）..."
              value={searchKeyword}
              onChange={(e) => setSearchKeyword(e.target.value)}
              style={{
                width: '240px',
                padding: '8px 32px 8px 12px',
                border: '1px solid #d9d9d9',
                borderRadius: '6px',
                fontSize: '14px',
                outline: 'none',
                transition: 'border-color 0.2s',
              }}
              onFocus={(e) => e.target.style.borderColor = '#1890ff'}
              onBlur={(e) => e.target.style.borderColor = '#d9d9d9'}
            />
            {searchKeyword && (
              <button
                onClick={clearSearch}
                style={{
                  position: 'absolute',
                  right: '8px',
                  background: 'none',
                  border: 'none',
                  cursor: 'pointer',
                  color: '#999',
                  fontSize: '16px',
                  padding: '0',
                  lineHeight: '1',
                }}
                title="清除搜索"
              >
                ×
              </button>
            )}
          </div>
          <button 
            className={styles.createButton}
            onClick={() => setShowCreateModal(true)}
          >
            + 新建 Skill
          </button>
        </div>
      </div>

      {message && (
        <div className={`${styles.message} ${styles[message.type]}`}>
          {message.text}
          <button onClick={() => setMessage(null)}>×</button>
        </div>
      )}

      {/* 搜索结果显示 */}
      {isSearching ? (
        <div className={styles.skillSection}>
          <h3 className={styles.skillSectionTitle}>
            搜索结果
            <span className={styles.skillSectionDesc}>
              找到 {searchTotal} 个匹配 "{searchKeyword}" 的 Skill
            </span>
          </h3>
          {searchResults.length > 0 ? (
            <div className={styles.skillList}>
              {searchResults.map(renderSkillCard)}
            </div>
          ) : (
            <div style={{ padding: '40px', textAlign: 'center', color: '#999' }}>
              未找到匹配的 Skill
            </div>
          )}
        </div>
      ) : activeLayer === 'all' ? (
        <>
          {renderLayerSection(
            '基础层 (Foundation)', 
            '最通用的基础知识，所有功能复用', 
            foundationData, 
            'foundation'
          )}
          {renderLayerSection(
            '领域层 (Domain)', 
            '专业领域知识，部分功能复用', 
            domainData, 
            'domain'
          )}
          {renderLayerSection(
            '任务层 (Task)', 
            '特定功能的专属工作流程', 
            taskData, 
            'task'
          )}
        </>
      ) : (
        <>
          {currentPagination?.loading ? (
            <div style={{ padding: '40px', textAlign: 'center', color: '#999' }}>加载中...</div>
          ) : (
            <>
              <div className={styles.skillList}>
                {displaySkills.map(renderSkillCard)}
              </div>
              {currentPagination && currentLayer && renderPagination(currentPagination, currentLayer)}
            </>
          )}
        </>
      )}

      {totalCount === 0 && !isLoading && (
        <div className={styles.empty}>
          <p>暂无 Skill</p>
          <button 
            className={styles.createButton}
            onClick={() => setShowCreateModal(true)}
          >
            创建第一个 Skill
          </button>
        </div>
      )}

      {(editingSkill || showCreateModal) && (
        <SkillEditor
          skill={editingSkill}
          onClose={handleEditorClose}
        />
      )}

      {viewingSkill && (
        <SkillEditor
          skill={viewingSkill}
          onClose={() => setViewingSkill(null)}
          readOnly
        />
      )}

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
    </>
  );
};

// 主组件
const AIConfigList = () => {
  const [searchParams, setSearchParams] = useSearchParams();
  const activeTab = searchParams.get('tab') || 'configs';

  const handleTabChange = (tab: string) => {
    setSearchParams({ tab });
  };

  return (
    <div className={styles.container}>
      <div className={styles.header}>
        <h1 className={styles.title}>AI 配置</h1>
      </div>

      <div className={styles.tabs}>
        <button 
          className={`${styles.tab} ${activeTab === 'configs' ? styles.activeTab : ''}`}
          onClick={() => handleTabChange('configs')}
        >
          模型配置
        </button>
        <button 
          className={`${styles.tab} ${activeTab === 'skills' ? styles.activeTab : ''}`}
          onClick={() => handleTabChange('skills')}
        >
          AI Skills
        </button>
      </div>

      <div className={styles.tabContent}>
        {activeTab === 'configs' ? <ConfigsTab /> : <SkillsTab />}
      </div>
    </div>
  );
};

export default AIConfigList;
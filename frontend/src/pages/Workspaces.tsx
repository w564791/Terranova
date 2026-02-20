import React, { useState, useEffect, useMemo, useRef, useCallback } from 'react';
import { useNavigate, Link } from 'react-router-dom';
import { type Workspace } from '../services/workspaces';
import { getProjects, createProject, type Project, type CreateProjectRequest } from '../services/projects';
import { useToast } from '../contexts/ToastContext';
import api from '../services/api';
import styles from './Workspaces.module.css';

console.log('Workspaces component loaded');

interface WorkspaceWithStatus extends Workspace {
  latestRunStatus?: string;
  latestRunId?: number;
  latestApplyTime?: string;
  // 后端返回的字段
  latest_run_status?: string;
  latest_run_id?: number;
  latest_apply_time?: string;
}

const Workspaces: React.FC = () => {
  console.log('Workspaces component rendering');
  const navigate = useNavigate();
  const { success, error: showError } = useToast();
  const [workspaces, setWorkspaces] = useState<WorkspaceWithStatus[]>([]);
  const [projects, setProjects] = useState<Project[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [searchTerm, setSearchTerm] = useState('');
  const [searchInput, setSearchInput] = useState('');
  const [projectSearchTerm, setProjectSearchTerm] = useState('');
  const [selectedProject, setSelectedProject] = useState<number | null>(null);
  const [selectedStatuses, setSelectedStatuses] = useState<string[]>([]);
  const [selectedTags, setSelectedTags] = useState<string[]>([]);
  const [showTagsDropdown, setShowTagsDropdown] = useState(false);
  const [showNewDropdown, setShowNewDropdown] = useState(false);
  const [showCreateProjectDialog, setShowCreateProjectDialog] = useState(false);
  const [newProjectName, setNewProjectName] = useState('');
  const [newProjectDisplayName, setNewProjectDisplayName] = useState('');
  const [newProjectDescription, setNewProjectDescription] = useState('');
  const [createProjectLoading, setCreateProjectLoading] = useState(false);
  const [sidebarCollapsed, setSidebarCollapsed] = useState(false);
  const [currentPage, setCurrentPage] = useState(1);
  const pageSize = 10;
  const [isInitialLoad, setIsInitialLoad] = useState(true);
  const loadingRef = useRef(false);
  const previousProjectRef = useRef<number | null>(null);

  // 加载项目列表
  const loadProjects = async () => {
    try {
      const projectList = await getProjects();
      console.log('Loaded projects:', projectList);
      setProjects(projectList);
    } catch (err) {
      console.error('Failed to load projects:', err);
    }
  };

  // 创建项目
  const handleCreateProject = async () => {
    if (!newProjectName.trim() || !newProjectDisplayName.trim()) {
      return;
    }

    try {
      setCreateProjectLoading(true);
      const data: CreateProjectRequest = {
        org_id: 1,
        name: newProjectName.trim(),
        display_name: newProjectDisplayName.trim(),
        description: newProjectDescription.trim() || undefined
      };
      await createProject(data);
      setShowCreateProjectDialog(false);
      setNewProjectName('');
      setNewProjectDisplayName('');
      setNewProjectDescription('');
      success('项目创建成功');
      loadProjects();
    } catch (err: any) {
      console.error('Failed to create project:', err);
      showError('创建项目失败: ' + (err.response?.data?.error || err.message || err));
    } finally {
      setCreateProjectLoading(false);
    }
  };

  const loadWorkspaces = useCallback(async (silent = false) => {
    // 防止重复加载
    if (loadingRef.current) {
      return;
    }
    
    console.log('Loading workspaces...', { silent, isInitialLoad });
    loadingRef.current = true;
    
    try {
      // 只在首次加载时显示 loading 状态
      if (!silent && isInitialLoad) {
        setLoading(true);
      }
      setError(null);
      
      const params = new URLSearchParams({
        page: '1',
        size: '100'
      });
      
      if (searchTerm) {
        params.append('search', searchTerm);
      }
      
      if (selectedProject !== null) {
        params.append('project_id', selectedProject.toString());
      }
      
      const response = await api.get(`/workspaces?${params.toString()}`);
      console.log('Workspace API response:', response);
      const responseData = response.data as any;
      const workspacesList = Array.isArray(responseData) ? responseData : (responseData?.items || []);
      
      // 后端现在直接返回状态信息，无需额外调用 tasks API
      const workspacesWithStatus = workspacesList.map((w: WorkspaceWithStatus) => ({
        ...w,
        // 映射后端返回的字段到前端使用的字段名
        latestRunStatus: w.latest_run_status || w.latestRunStatus,
        latestRunId: w.latest_run_id || w.latestRunId,
        latestApplyTime: w.latest_apply_time || w.latestApplyTime
      }));
      
      setWorkspaces(workspacesWithStatus);
      setIsInitialLoad(false);
    } catch (err: any) {
      console.error('Workspace API error:', err);
      const errorMessage = err.message === 'Failed to fetch' 
        ? '网络连接失败，请检查网络或稍后重试'
        : err.message || '加载工作空间列表失败';
      setError(errorMessage);
      setWorkspaces([]);
    } finally {
      setLoading(false);
      loadingRef.current = false;
    }
  }, [searchTerm, selectedProject, isInitialLoad]);

  useEffect(() => {
    loadProjects();
  }, []);

  // 防抖搜索
  useEffect(() => {
    const timer = setTimeout(() => {
      setSearchTerm(searchInput);
    }, 500);
    return () => clearTimeout(timer);
  }, [searchInput]);

  // 监听 selectedProject 变化，使用静默加载
  useEffect(() => {
    const isProjectChange = previousProjectRef.current !== selectedProject;
    previousProjectRef.current = selectedProject;
    
    // 如果是项目切换，使用静默加载；如果是首次加载，显示 loading
    loadWorkspaces(isProjectChange && !isInitialLoad);
  }, [searchTerm, selectedProject]);

  // 获取状态统计
  const statusStats = useMemo(() => {
    const stats = {
      attention: 0,
      error: 0,
      running: 0,
      success: 0
    };
    workspaces.forEach(w => {
      const status = w.latestRunStatus;
      if (status === 'requires_approval' || status === 'plan_completed') {
        stats.attention++;
      } else if (status === 'failed') {
        stats.error++;
      } else if (status === 'running') {
        stats.running++;
      } else if (status === 'applied' || status === 'success') {
        stats.success++;
      }
    });
    return stats;
  }, [workspaces]);

  // 过滤项目列表
  const filteredProjects = useMemo(() => {
    if (!projectSearchTerm) return projects;
    return projects.filter(p => 
      p.name.toLowerCase().includes(projectSearchTerm.toLowerCase()) ||
      p.display_name.toLowerCase().includes(projectSearchTerm.toLowerCase())
    );
  }, [projects, projectSearchTerm]);

  // 获取所有可用的tags
  const availableTags = useMemo(() => {
    const tagSet = new Set<string>();
    workspaces.forEach(workspace => {
      const tags = workspace.tags;
      if (tags && typeof tags === 'object') {
        Object.keys(tags).forEach(key => {
          const value = tags[key];
          // 支持 key:value 格式的tag
          tagSet.add(`${key}:${value}`);
        });
      }
    });
    return Array.from(tagSet).sort();
  }, [workspaces]);

  // 前端过滤
  const filteredWorkspaces = useMemo(() => {
    return workspaces.filter(workspace => {
      // 状态过滤
      let matchesStatus = selectedStatuses.length === 0;
      if (!matchesStatus) {
        const status = workspace.latestRunStatus;
        if (selectedStatuses.includes('attention') && (status === 'requires_approval' || status === 'plan_completed')) {
          matchesStatus = true;
        }
        if (selectedStatuses.includes('error') && status === 'failed') {
          matchesStatus = true;
        }
        if (selectedStatuses.includes('running') && status === 'running') {
          matchesStatus = true;
        }
        if (selectedStatuses.includes('success') && (status === 'applied' || status === 'success')) {
          matchesStatus = true;
        }
      }

      // Tag过滤
      let matchesTags = selectedTags.length === 0;
      const tags = workspace.tags;
      if (!matchesTags && tags && typeof tags === 'object') {
        matchesTags = selectedTags.some(selectedTag => {
          const [key, value] = selectedTag.split(':');
          return tags[key] === value;
        });
      }

      return matchesStatus && matchesTags;
    });
  }, [workspaces, selectedStatuses, selectedTags]);

  // 分页后的工作空间
  const paginatedWorkspaces = useMemo(() => {
    const startIndex = (currentPage - 1) * pageSize;
    return filteredWorkspaces.slice(startIndex, startIndex + pageSize);
  }, [filteredWorkspaces, currentPage, pageSize]);

  // 总页数
  const totalPages = Math.ceil(filteredWorkspaces.length / pageSize);

  // 当筛选条件变化时，重置到第一页
  useEffect(() => {
    setCurrentPage(1);
  }, [selectedStatuses, selectedTags, searchTerm, selectedProject]);

  // 切换状态选择
  const toggleStatus = (status: string) => {
    setSelectedStatuses(prev => 
      prev.includes(status) 
        ? prev.filter(s => s !== status)
        : [...prev, status]
    );
  };

  // 切换tag选择
  const toggleTag = (tag: string) => {
    setSelectedTags(prev => 
      prev.includes(tag) 
        ? prev.filter(t => t !== tag)
        : [...prev, tag]
    );
  };

  // 清除所有筛选
  const clearAllFilters = () => {
    setSearchTerm('');
    setSelectedStatuses([]);
    setSelectedTags([]);
  };

  const hasActiveFilters = searchTerm || selectedStatuses.length > 0 || selectedTags.length > 0;
  
  // 格式化相对时间
  const formatRelativeTime = (dateString: string | null | undefined) => {
    if (!dateString) return '-';
    if (dateString.startsWith('0001-01-01')) return '-';
    
    let normalizedDateString = dateString;
    if (dateString.endsWith('Z')) {
      normalizedDateString = dateString.slice(0, -1);
    }
    
    const date = new Date(normalizedDateString);
    const now = new Date();
    
    if (isNaN(date.getTime())) return '-';
    
    const diffMs = now.getTime() - date.getTime();
    const diffMins = Math.floor(diffMs / 60000);
    const diffHours = Math.floor(diffMs / 3600000);
    const diffDays = Math.floor(diffMs / 86400000);
    const diffMonths = Math.floor(diffDays / 30);
    const diffYears = Math.floor(diffDays / 365);

    if (diffMins < 5) return 'just now';
    if (diffMins < 60) return `${diffMins} mins ago`;
    if (diffHours < 24) return `${diffHours} hours ago`;
    if (diffDays < 30) return `${diffDays} days ago`;
    if (diffMonths < 12) return `${diffMonths} months ago`;
    return `${diffYears} years ago`;
  };

  // 获取选中的项目名称
  const getSelectedProjectName = () => {
    if (selectedProject === null) return '';
    const project = projects.find(p => p.id === selectedProject);
    return project?.display_name || project?.name || '';
  };

  if (loading) {
    return <div className={styles.loading}>加载中...</div>;
  }

  if (error) {
    return (
      <div className={styles.error}>
        <div className={styles.errorIcon}>
          <svg width="32" height="32" viewBox="0 0 24 24" fill="none" stroke="#f59e0b" strokeWidth="2">
            <path d="M10.29 3.86L1.82 18a2 2 0 0 0 1.71 3h16.94a2 2 0 0 0 1.71-3L13.71 3.86a2 2 0 0 0-3.42 0z"/>
            <line x1="12" y1="9" x2="12" y2="13"/>
            <line x1="12" y1="17" x2="12.01" y2="17"/>
          </svg>
        </div>
        <p className={styles.errorMessage}>{error}</p>
        <button onClick={() => loadWorkspaces(false)} className={styles.retryButton}>重新加载</button>
      </div>
    );
  }

  return (
    <div className={styles.container}>
      {/* 页面标题和 New 按钮 */}
      <div className={styles.pageHeader}>
        <h1 className={styles.pageTitle}>Projects & workspaces</h1>
        <div className={styles.newButtonContainer}>
          <button 
            className={`${styles.newButton} ${showNewDropdown ? styles.newButtonActive : ''}`}
            onClick={() => setShowNewDropdown(!showNewDropdown)}
          >
            <span>New</span>
            <span className={styles.dropdownArrow}>{showNewDropdown ? '∧' : '∨'}</span>
          </button>
          {showNewDropdown && (
            <>
              <div className={styles.dropdownOverlay} onClick={() => setShowNewDropdown(false)} />
              <div className={styles.newDropdown}>
                <button 
                  className={styles.dropdownItem}
                  onClick={() => {
                    setShowNewDropdown(false);
                    setShowCreateProjectDialog(true);
                  }}
                >
                  Project
                </button>
                <button 
                  className={styles.dropdownItem}
                  onClick={() => {
                    setShowNewDropdown(false);
                    navigate('/workspaces/create');
                  }}
                >
                  Workspace
                </button>
              </div>
            </>
          )}
        </div>
      </div>

      <div className={styles.mainLayout}>
        {/* 左侧 Projects 列表 */}
        {!sidebarCollapsed && (
        <div className={styles.projectsSidebar}>
          <div className={styles.projectsHeader}>
            <span className={styles.projectsTitle}>PROJECTS</span>
            <button className={styles.projectSearchBtn}>
              <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
                <circle cx="11" cy="11" r="8"/>
                <path d="M21 21l-4.35-4.35"/>
              </svg>
            </button>
          </div>
          
          <div className={styles.projectsList}>
            {/* All Workspaces */}
            <div 
              className={`${styles.projectItem} ${selectedProject === null ? styles.projectItemActive : ''}`}
              onClick={() => setSelectedProject(null)}
            >
              <svg className={styles.projectIcon} width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
                <path d="M22 19a2 2 0 0 1-2 2H4a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h5l2 3h9a2 2 0 0 1 2 2z"/>
              </svg>
              <span className={styles.projectName}>All Workspaces</span>
            </div>
            
            {/* Projects */}
            {filteredProjects.map(project => (
              <div 
                key={project.id}
                className={`${styles.projectItem} ${selectedProject === project.id ? styles.projectItemActive : ''}`}
                onClick={() => setSelectedProject(project.id)}
              >
                <svg className={styles.projectIcon} width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
                  <path d="M22 19a2 2 0 0 1-2 2H4a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h5l2 3h9a2 2 0 0 1 2 2z"/>
                </svg>
                <span className={styles.projectName}>{project.display_name || project.name}</span>
              </div>
            ))}
          </div>
          
          {/* 分页 */}
          <div className={styles.projectsPagination}>
            <button className={styles.paginationBtn} disabled>‹ Previous</button>
            <button className={styles.paginationBtn}>Next ›</button>
          </div>
        </div>
        )}

        {/* 右侧内容区域 */}
        <div className={styles.contentArea}>
          {/* 工作空间标题栏 */}
          <div className={styles.workspacesHeader}>
            <div className={styles.workspacesTitle}>
              <button 
                className={styles.collapseBtn}
                onClick={() => setSidebarCollapsed(!sidebarCollapsed)}
                title={sidebarCollapsed ? '展开项目栏' : '收起项目栏'}
              >
                <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
                  {sidebarCollapsed ? (
                    <path d="M9 18l6-6-6-6"/>
                  ) : (
                    <path d="M15 18l-6-6 6-6"/>
                  )}
                </svg>
              </button>
              <span>Workspaces</span>
              {selectedProject !== null && (
                <span className={styles.inProject}>
                  in <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" style={{verticalAlign: 'middle', marginRight: '4px'}}>
                    <path d="M22 19a2 2 0 0 1-2 2H4a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h5l2 3h9a2 2 0 0 1 2 2z"/>
                  </svg>
                  {getSelectedProjectName()}
                </span>
              )}
            </div>
            <div className={styles.searchBox}>
              <input
                type="text"
                placeholder="Search workspaces"
                value={searchInput}
                onChange={(e) => setSearchInput(e.target.value)}
                className={styles.searchInput}
              />
            </div>
          </div>

          {/* 状态统计和筛选器 */}
          <div className={styles.filtersBar}>
            <div className={styles.statusFilters}>
              <button 
                className={`${styles.statusBtn} ${styles.statusAttention} ${selectedStatuses.includes('attention') ? styles.statusBtnActive : ''}`}
                onClick={() => toggleStatus('attention')}
              >
                <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
                  <path d="M10.29 3.86L1.82 18a2 2 0 0 0 1.71 3h16.94a2 2 0 0 0 1.71-3L13.71 3.86a2 2 0 0 0-3.42 0z"/>
                  <line x1="12" y1="9" x2="12" y2="13"/>
                  <line x1="12" y1="17" x2="12.01" y2="17"/>
                </svg>
                <span>{statusStats.attention}</span>
              </button>
              <button 
                className={`${styles.statusBtn} ${styles.statusError} ${selectedStatuses.includes('error') ? styles.statusBtnActive : ''}`}
                onClick={() => toggleStatus('error')}
              >
                <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
                  <circle cx="12" cy="12" r="10"/>
                  <line x1="15" y1="9" x2="9" y2="15"/>
                  <line x1="9" y1="9" x2="15" y2="15"/>
                </svg>
                <span>{statusStats.error}</span>
              </button>
              <button 
                className={`${styles.statusBtn} ${styles.statusRunning} ${selectedStatuses.includes('running') ? styles.statusBtnActive : ''}`}
                onClick={() => toggleStatus('running')}
              >
                <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
                  <circle cx="12" cy="12" r="10"/>
                  <polyline points="12 6 12 12 16 14"/>
                </svg>
                <span>{statusStats.running}</span>
              </button>
              <button 
                className={`${styles.statusBtn} ${styles.statusSuccess} ${selectedStatuses.includes('success') ? styles.statusBtnActive : ''}`}
                onClick={() => toggleStatus('success')}
              >
                <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
                  <path d="M22 11.08V12a10 10 0 1 1-5.93-9.14"/>
                  <polyline points="22 4 12 14.01 9 11.01"/>
                </svg>
                <span>{statusStats.success}</span>
              </button>
            </div>
            
            <div className={styles.filterActions}>
              <div className={styles.filterDropdownContainer}>
                <button 
                  className={`${styles.filterBtn} ${selectedTags.length > 0 ? styles.filterBtnActive : ''}`}
                  onClick={() => setShowTagsDropdown(!showTagsDropdown)}
                >
                  Tags {selectedTags.length > 0 && `(${selectedTags.length})`} <span className={styles.dropdownIcon}>▾</span>
                </button>
                {showTagsDropdown && (
                  <>
                    <div className={styles.dropdownOverlay} onClick={() => setShowTagsDropdown(false)} />
                    <div className={styles.filterDropdown}>
                      {availableTags.length === 0 ? (
                        <div className={styles.filterDropdownEmpty}>No tags available</div>
                      ) : (
                        availableTags.map(tag => (
                          <label key={tag} className={styles.filterDropdownItem}>
                            <input
                              type="checkbox"
                              checked={selectedTags.includes(tag)}
                              onChange={() => toggleTag(tag)}
                            />
                            <span>{tag}</span>
                          </label>
                        ))
                      )}
                    </div>
                  </>
                )}
              </div>
              {hasActiveFilters && (
                <button className={styles.clearBtn} onClick={clearAllFilters}>
                  × Clear all
                </button>
              )}
            </div>
          </div>

          {/* 工作空间表格 */}
          <div className={styles.workspacesTable}>
            <div className={styles.tableHeader}>
              <div className={styles.colName}>Workspace Name ↓</div>
              <div className={styles.colStatus}>Run Status</div>
              <div className={styles.colMode}>Mode</div>
              <div className={styles.colTime}>Latest Change</div>
            </div>
            
            <div className={styles.tableBody}>
              {paginatedWorkspaces.length === 0 ? (
                <div className={styles.emptyState}>
                  <p>没有找到匹配的工作空间</p>
                </div>
              ) : (
                paginatedWorkspaces.map(workspace => (
                  <Link 
                    key={workspace.workspace_id}
                    to={`/workspaces/${workspace.workspace_id}`}
                    className={styles.tableRow}
                  >
                    <div className={styles.colName}>
                      <span className={styles.workspaceName}>{workspace.name}</span>
                      {workspace.description && (
                        <span className={styles.workspaceDesc}>{workspace.description}</span>
                      )}
                    </div>
                    <div className={styles.colStatus}>
                      {workspace.latestRunStatus && (
                        <span className={`${styles.statusBadge} ${styles[`status-${workspace.latestRunStatus}`]}`}>
                          {(workspace.latestRunStatus === 'applied' || workspace.latestRunStatus === 'success') && (
                            <><svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="3"><polyline points="20 6 9 17 4 12"/></svg> Applied</>
                          )}
                          {workspace.latestRunStatus === 'failed' && (
                            <><svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="3"><line x1="18" y1="6" x2="6" y2="18"/><line x1="6" y1="6" x2="18" y2="18"/></svg> Failed</>
                          )}
                          {workspace.latestRunStatus === 'running' && (
                            <><svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2"><circle cx="12" cy="12" r="10"/><polyline points="12 6 12 12 16 14"/></svg> Running</>
                          )}
                          {(workspace.latestRunStatus === 'requires_approval' || workspace.latestRunStatus === 'plan_completed' || workspace.latestRunStatus === 'apply_pending') && (
                            <><svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2"><rect x="6" y="4" width="4" height="16"/><rect x="14" y="4" width="4" height="16"/></svg> Pending</>
                          )}
                          {workspace.latestRunStatus === 'planned_and_finished' && (
                            <><svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2"><circle cx="12" cy="12" r="10"/><polyline points="20 6 9 17 4 12"/></svg> Planned (No Changes)</>
                          )}
                          {workspace.latestRunStatus === 'pending' && (
                            <><svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2"><circle cx="12" cy="12" r="10"/><line x1="12" y1="8" x2="12" y2="12"/><line x1="12" y1="16" x2="12.01" y2="16"/></svg> Pending</>
                          )}
                          {workspace.latestRunStatus === 'waiting' && (
                            <><svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2"><circle cx="12" cy="12" r="10"/><polyline points="12 6 12 12 16 14"/></svg> Waiting</>
                          )}
                          {workspace.latestRunStatus === 'cancelled' && (
                            <><svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2"><circle cx="12" cy="12" r="10"/><line x1="4.93" y1="4.93" x2="19.07" y2="19.07"/></svg> Cancelled</>
                          )}
                        </span>
                      )}
                    </div>
                    <div className={styles.colMode}>
                      <span className={styles.modeBadge}>{workspace.execution_mode}</span>
                    </div>
                    <div className={styles.colTime}>
                      {formatRelativeTime(workspace.latestApplyTime)}
                    </div>
                  </Link>
                ))
              )}
            </div>
          </div>

          {/* 分页 */}
          <div className={styles.tablePagination}>
            <span className={styles.paginationInfo}>
              {filteredWorkspaces.length === 0 ? '0' : `${(currentPage - 1) * pageSize + 1}–${Math.min(currentPage * pageSize, filteredWorkspaces.length)}`} of {filteredWorkspaces.length}
            </span>
            <div className={styles.paginationControls}>
              <button 
                className={styles.paginationBtn}
                disabled={currentPage === 1}
                onClick={() => setCurrentPage(p => Math.max(1, p - 1))}
              >
                ‹ Previous
              </button>
              <span className={styles.pageInfo}>{currentPage} / {totalPages || 1}</span>
              <button 
                className={styles.paginationBtn}
                disabled={currentPage >= totalPages}
                onClick={() => setCurrentPage(p => Math.min(totalPages, p + 1))}
              >
                Next ›
              </button>
            </div>
          </div>
        </div>
      </div>

      {/* 创建项目对话框 */}
      {showCreateProjectDialog && (
        <div className={styles.dialogOverlay}>
          <div className={styles.dialogContent}>
            <div className={styles.dialogHeader}>
              <h3 className={styles.dialogTitle}>创建项目</h3>
              <button 
                className={styles.dialogClose}
                onClick={() => setShowCreateProjectDialog(false)}
              >
                ×
              </button>
            </div>
            <div className={styles.dialogBody}>
              <div className={styles.formGroup}>
                <label className={styles.formLabel}>
                  项目标识 <span className={styles.required}>*</span>
                </label>
                <input
                  type="text"
                  value={newProjectName}
                  onChange={(e) => setNewProjectName(e.target.value)}
                  className={styles.formInput}
                  placeholder="例如：infrastructure"
                />
                <div className={styles.formHint}>小写字母、数字和连字符，创建后不可修改</div>
              </div>
              <div className={styles.formGroup}>
                <label className={styles.formLabel}>
                  显示名称 <span className={styles.required}>*</span>
                </label>
                <input
                  type="text"
                  value={newProjectDisplayName}
                  onChange={(e) => setNewProjectDisplayName(e.target.value)}
                  className={styles.formInput}
                  placeholder="例如：基础设施项目"
                />
              </div>
              <div className={styles.formGroup}>
                <label className={styles.formLabel}>描述</label>
                <textarea
                  value={newProjectDescription}
                  onChange={(e) => setNewProjectDescription(e.target.value)}
                  className={styles.formTextarea}
                  placeholder="项目的简要描述（可选）"
                  rows={3}
                />
              </div>
            </div>
            <div className={styles.dialogFooter}>
              <button 
                className={styles.cancelButton}
                onClick={() => setShowCreateProjectDialog(false)}
              >
                取消
              </button>
              <button 
                className={styles.primaryButton}
                onClick={handleCreateProject}
                disabled={createProjectLoading || !newProjectName.trim() || !newProjectDisplayName.trim()}
              >
                {createProjectLoading ? '创建中...' : '创建'}
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
};

export default Workspaces;

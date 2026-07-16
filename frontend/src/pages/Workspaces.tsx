import React, { useState, useEffect, useMemo, useRef, useCallback } from 'react';
import { useNavigate, Link } from 'react-router-dom';
import { type Workspace } from '../services/workspaces';
import { getProjects, createProject, type Project, type CreateProjectRequest } from '../services/projects';
import { useToast } from '../contexts/ToastContext';
import api from '../services/api';
import { getTaskStatusLabel, getTaskStatusCategory } from '../utils/taskStatus';
import styles from './Workspaces.module.css';

interface WorkspaceWithStatus extends Workspace {
  latestRunStatus?: string;
  latestRunId?: number;
  latestRunTaskType?: string;
  latestApplyTime?: string;
  latest_run_status?: string;
  latest_run_id?: number;
  latest_run_task_type?: string;
  latest_apply_time?: string;
}

const WORKSPACE_PAGE_SIZE = 10;
const PROJECT_PAGE_SIZE = 8;

function getInitials(name: string): string {
  const cleaned = (name || '').trim();
  if (!cleaned) return '??';
  const parts = cleaned.replace(/[-_./]+/g, ' ').split(/\s+/).filter(Boolean);
  if (parts.length >= 2) {
    return (parts[0][0] + parts[1][0]).toUpperCase();
  }
  return cleaned.slice(0, 2).toUpperCase();
}

function markClassForStatus(status: string | undefined): string {
  if (!status) return '';
  const cat = getTaskStatusCategory(status);
  switch (cat) {
    case 'attention':
      return styles.wsMarkAttention;
    case 'error':
      return styles.wsMarkError;
    case 'running':
      return styles.wsMarkRunning;
    case 'success':
      return styles.wsMarkSuccess;
    default:
      return '';
  }
}

function badgeClassForStatus(status: string | undefined): string {
  if (!status) return styles.badgeNeutral;
  const cat = getTaskStatusCategory(status);
  switch (cat) {
    case 'success':
      return styles.badgeSuccess;
    case 'attention':
      return styles.badgeAttention;
    case 'error':
      return styles.badgeError;
    case 'running':
      return styles.badgeRunning;
    case 'pending':
      return styles.badgePending;
    default:
      return styles.badgeNeutral;
  }
}

function tagEntries(tags: Record<string, unknown> | undefined | null): string[] {
  if (!tags || typeof tags !== 'object') return [];
  return Object.keys(tags)
    .map((key) => `${key}:${String(tags[key])}`)
    .sort();
}

const Workspaces: React.FC = () => {
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
  const [showCreateProjectDialog, setShowCreateProjectDialog] = useState(false);
  const [newProjectName, setNewProjectName] = useState('');
  const [newProjectDisplayName, setNewProjectDisplayName] = useState('');
  const [newProjectDescription, setNewProjectDescription] = useState('');
  const [createProjectLoading, setCreateProjectLoading] = useState(false);
  const [currentPage, setCurrentPage] = useState(1);
  const [projectPage, setProjectPage] = useState(1);
  const [sidebarCollapsed, setSidebarCollapsed] = useState(false);
  const [isInitialLoad, setIsInitialLoad] = useState(true);
  const loadingRef = useRef(false);
  const previousProjectRef = useRef<number | null>(null);

  const loadProjects = async () => {
    try {
      const projectList = await getProjects();
      setProjects(projectList);
    } catch (err) {
      console.error('Failed to load projects:', err);
    }
  };

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
        description: newProjectDescription.trim() || undefined,
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

  const loadWorkspaces = useCallback(
    async (silent = false) => {
      if (loadingRef.current) {
        return;
      }

      loadingRef.current = true;

      try {
        if (!silent && isInitialLoad) {
          setLoading(true);
        }
        setError(null);

        const params = new URLSearchParams({
          page: '1',
          size: '100',
        });

        if (searchTerm) {
          params.append('search', searchTerm);
        }

        if (selectedProject !== null) {
          params.append('project_id', selectedProject.toString());
        }

        const response = await api.get(`/workspaces?${params.toString()}`);
        const responseData = response.data as any;
        const workspacesList = Array.isArray(responseData)
          ? responseData
          : responseData?.items || [];

        const workspacesWithStatus = workspacesList.map((w: WorkspaceWithStatus) => ({
          ...w,
          latestRunStatus: w.latest_run_status || w.latestRunStatus,
          latestRunId: w.latest_run_id || w.latestRunId,
          latestRunTaskType: w.latest_run_task_type || w.latestRunTaskType,
          latestApplyTime: w.latest_apply_time || w.latestApplyTime,
        }));

        setWorkspaces(workspacesWithStatus);
        setIsInitialLoad(false);
      } catch (err: any) {
        console.error('Workspace API error:', err);
        const errorMessage =
          err.message === 'Failed to fetch'
            ? '网络连接失败，请检查网络或稍后重试'
            : err.message || '加载工作空间列表失败';
        setError(errorMessage);
        setWorkspaces([]);
      } finally {
        setLoading(false);
        loadingRef.current = false;
      }
    },
    [searchTerm, selectedProject, isInitialLoad]
  );

  useEffect(() => {
    loadProjects();
  }, []);

  // Debounced workspace search
  useEffect(() => {
    const timer = setTimeout(() => {
      setSearchTerm(searchInput);
    }, 500);
    return () => clearTimeout(timer);
  }, [searchInput]);

  // Reload workspaces when search/project changes
  useEffect(() => {
    const isProjectChange = previousProjectRef.current !== selectedProject;
    previousProjectRef.current = selectedProject;
    loadWorkspaces(isProjectChange && !isInitialLoad);
    // eslint-disable-next-line react-hooks/exhaustive-deps -- loadWorkspaces identity tracks deps; intentional on search/project only
  }, [searchTerm, selectedProject]);

  const statusStats = useMemo(() => {
    const stats = { attention: 0, error: 0, running: 0, success: 0 };
    workspaces.forEach((w) => {
      const status = w.latestRunStatus;
      if (
        status === 'requires_approval' ||
        status === 'plan_completed' ||
        status === 'decision_required' ||
        status === 'apply_pending'
      ) {
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

  // Filter projects by search
  const filteredProjects = useMemo(() => {
    if (!projectSearchTerm.trim()) return projects;
    const q = projectSearchTerm.toLowerCase();
    return projects.filter(
      (p) =>
        p.name.toLowerCase().includes(q) ||
        (p.display_name || '').toLowerCase().includes(q)
    );
  }, [projects, projectSearchTerm]);

  // Client-side project pagination (API returns full list)
  const projectTotalPages = Math.max(1, Math.ceil(filteredProjects.length / PROJECT_PAGE_SIZE));

  const paginatedProjects = useMemo(() => {
    const start = (projectPage - 1) * PROJECT_PAGE_SIZE;
    return filteredProjects.slice(start, start + PROJECT_PAGE_SIZE);
  }, [filteredProjects, projectPage]);

  // Reset / clamp project page when filter or data changes
  useEffect(() => {
    setProjectPage(1);
  }, [projectSearchTerm]);

  useEffect(() => {
    if (projectPage > projectTotalPages) {
      setProjectPage(projectTotalPages);
    }
  }, [projectPage, projectTotalPages]);

  const availableTags = useMemo(() => {
    const tagSet = new Set<string>();
    workspaces.forEach((workspace) => {
      tagEntries(workspace.tags).forEach((t) => tagSet.add(t));
    });
    return Array.from(tagSet).sort();
  }, [workspaces]);

  const filteredWorkspaces = useMemo(() => {
    return workspaces.filter((workspace) => {
      let matchesStatus = selectedStatuses.length === 0;
      if (!matchesStatus) {
        const status = workspace.latestRunStatus;
        if (
          selectedStatuses.includes('attention') &&
          (status === 'requires_approval' ||
            status === 'plan_completed' ||
            status === 'decision_required' ||
            status === 'apply_pending')
        ) {
          matchesStatus = true;
        }
        if (selectedStatuses.includes('error') && status === 'failed') {
          matchesStatus = true;
        }
        if (selectedStatuses.includes('running') && status === 'running') {
          matchesStatus = true;
        }
        if (
          selectedStatuses.includes('success') &&
          (status === 'applied' || status === 'success')
        ) {
          matchesStatus = true;
        }
      }

      let matchesTags = selectedTags.length === 0;
      if (!matchesTags) {
        const tags = workspace.tags;
        if (tags && typeof tags === 'object') {
          matchesTags = selectedTags.some((selectedTag) => {
            const [key, ...rest] = selectedTag.split(':');
            const value = rest.join(':');
            return String(tags[key]) === value;
          });
        }
      }

      return matchesStatus && matchesTags;
    });
  }, [workspaces, selectedStatuses, selectedTags]);

  const totalPages = Math.max(1, Math.ceil(filteredWorkspaces.length / WORKSPACE_PAGE_SIZE));

  const paginatedWorkspaces = useMemo(() => {
    const startIndex = (currentPage - 1) * WORKSPACE_PAGE_SIZE;
    return filteredWorkspaces.slice(startIndex, startIndex + WORKSPACE_PAGE_SIZE);
  }, [filteredWorkspaces, currentPage]);

  useEffect(() => {
    setCurrentPage(1);
  }, [selectedStatuses, selectedTags, searchTerm, selectedProject]);

  useEffect(() => {
    if (currentPage > totalPages) {
      setCurrentPage(totalPages);
    }
  }, [currentPage, totalPages]);

  const toggleStatus = (status: string) => {
    setSelectedStatuses((prev) =>
      prev.includes(status) ? prev.filter((s) => s !== status) : [...prev, status]
    );
  };

  const toggleTag = (tag: string) => {
    setSelectedTags((prev) =>
      prev.includes(tag) ? prev.filter((t) => t !== tag) : [...prev, tag]
    );
  };

  const clearAllFilters = () => {
    setSearchInput('');
    setSearchTerm('');
    setSelectedStatuses([]);
    setSelectedTags([]);
  };

  const hasActiveFilters =
    Boolean(searchTerm) || selectedStatuses.length > 0 || selectedTags.length > 0;

  const formatRelativeTime = (dateString: string | null | undefined) => {
    if (!dateString) return '—';
    if (dateString.startsWith('0001-01-01')) return '—';

    let normalizedDateString = dateString;
    if (dateString.endsWith('Z')) {
      normalizedDateString = dateString.slice(0, -1);
    }

    const date = new Date(normalizedDateString);
    const now = new Date();

    if (isNaN(date.getTime())) return '—';

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

  const totalWorkspaceCount = useMemo(() => {
    if (selectedProject === null) {
      // Prefer API counts when available; fall back to loaded list length
      const fromProjects = projects.reduce((sum, p) => sum + (p.workspace_count ?? 0), 0);
      return fromProjects > 0 ? fromProjects : workspaces.length;
    }
    const p = projects.find((x) => x.id === selectedProject);
    return p?.workspace_count ?? workspaces.length;
  }, [projects, selectedProject, workspaces.length]);

  const selectedProjectName = useMemo(() => {
    if (selectedProject === null) return null;
    const project = projects.find((p) => p.id === selectedProject);
    return project?.display_name || project?.name || null;
  }, [projects, selectedProject]);

  const rangeLabel = useMemo(() => {
    const total = filteredWorkspaces.length;
    if (total === 0) return '0 of 0';
    const start = (currentPage - 1) * WORKSPACE_PAGE_SIZE + 1;
    const end = Math.min(currentPage * WORKSPACE_PAGE_SIZE, total);
    return `${start}–${end} of ${total}`;
  }, [filteredWorkspaces.length, currentPage]);

  if (loading) {
    return <div className={styles.loading}>加载中...</div>;
  }

  if (error) {
    return (
      <div className={styles.error}>
        <div className={styles.errorIcon}>
          <svg width="32" height="32" viewBox="0 0 24 24" fill="none" stroke="var(--amber)" strokeWidth="2">
            <path d="M10.29 3.86L1.82 18a2 2 0 0 0 1.71 3h16.94a2 2 0 0 0 1.71-3L13.71 3.86a2 2 0 0 0-3.42 0z" />
            <line x1="12" y1="9" x2="12" y2="13" />
            <line x1="12" y1="17" x2="12.01" y2="17" />
          </svg>
        </div>
        <p className={styles.errorMessage}>{error}</p>
        <button onClick={() => loadWorkspaces(false)} className={styles.retryButton}>
          重新加载
        </button>
      </div>
    );
  }

  return (
    <div className={styles.page}>
      {/* Toolbar */}
      <div className={styles.toolbar}>
        <div className={styles.titleBlock}>
          <h1>Workspaces</h1>
          <p className={styles.subtitle}>
            {selectedProjectName
              ? `${totalWorkspaceCount} workspace${totalWorkspaceCount === 1 ? '' : 's'} in ${selectedProjectName}`
              : `${totalWorkspaceCount} workspace${totalWorkspaceCount === 1 ? '' : 's'} · ${projects.length} project${projects.length === 1 ? '' : 's'}`}
          </p>
        </div>
        <div className={styles.actions}>
          <button
            type="button"
            className={styles.btnGhost}
            onClick={() => setShowCreateProjectDialog(true)}
          >
            New project
          </button>
          <button
            type="button"
            className={styles.btnPrimary}
            onClick={() => navigate('/workspaces/create')}
          >
            <span aria-hidden>+</span> New workspace
          </button>
        </div>
      </div>

      <div className={styles.layout}>
        {/* Projects sidebar */}
        {!sidebarCollapsed && (
          <aside className={styles.sidebar}>
            <div className={styles.sidebarHeader}>
              <div className={styles.sidebarLabel}>Projects</div>
              <div className={styles.sidebarSearch}>
                <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
                  <circle cx="11" cy="11" r="8" />
                  <path d="M21 21l-4.35-4.35" />
                </svg>
                <input
                  type="text"
                  placeholder="Filter projects"
                  value={projectSearchTerm}
                  onChange={(e) => setProjectSearchTerm(e.target.value)}
                  aria-label="Filter projects"
                />
              </div>
            </div>

            <div className={styles.projectList}>
              <button
                type="button"
                className={`${styles.projectItem} ${selectedProject === null ? styles.projectItemActive : ''}`}
                onClick={() => setSelectedProject(null)}
              >
                <span className={styles.projectMark}>All</span>
                <span className={styles.projectName}>All workspaces</span>
                <span className={styles.projectCount}>
                  {projects.reduce((s, p) => s + (p.workspace_count ?? 0), 0) || workspaces.length}
                </span>
              </button>

              {paginatedProjects.length === 0 ? (
                <div className={styles.projectEmpty}>
                  {projectSearchTerm.trim() ? 'No matching projects' : 'No projects yet'}
                </div>
              ) : (
                paginatedProjects.map((project) => (
                  <button
                    type="button"
                    key={project.id}
                    className={`${styles.projectItem} ${selectedProject === project.id ? styles.projectItemActive : ''}`}
                    onClick={() => setSelectedProject(project.id)}
                    title={project.display_name || project.name}
                  >
                    <span className={styles.projectMark}>
                      {getInitials(project.display_name || project.name)}
                    </span>
                    <span className={styles.projectName}>
                      {project.display_name || project.name}
                    </span>
                    <span className={styles.projectCount}>{project.workspace_count ?? 0}</span>
                  </button>
                ))
              )}
            </div>

            {filteredProjects.length > PROJECT_PAGE_SIZE && (
              <div className={styles.sidebarPager}>
                <span className={styles.pagerInfo}>
                  {projectPage} / {projectTotalPages}
                </span>
                <div className={styles.pagerBtns}>
                  <button
                    type="button"
                    className={styles.pagerBtn}
                    disabled={projectPage <= 1}
                    onClick={() => setProjectPage((p) => Math.max(1, p - 1))}
                    aria-label="Previous projects page"
                  >
                    ‹
                  </button>
                  <button
                    type="button"
                    className={styles.pagerBtn}
                    disabled={projectPage >= projectTotalPages}
                    onClick={() => setProjectPage((p) => Math.min(projectTotalPages, p + 1))}
                    aria-label="Next projects page"
                  >
                    ›
                  </button>
                </div>
              </div>
            )}
          </aside>
        )}

        {/* Main panel */}
        <section className={styles.main}>
          <div className={styles.panelToolbar}>
            <div className={styles.panelTitle}>
              <button
                type="button"
                className={styles.collapseBtn}
                onClick={() => setSidebarCollapsed(!sidebarCollapsed)}
                title={sidebarCollapsed ? '展开项目栏' : '收起项目栏'}
                aria-label={sidebarCollapsed ? '展开项目栏' : '收起项目栏'}
              >
                <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
                  {sidebarCollapsed ? (
                    <path d="M9 18l6-6-6-6" />
                  ) : (
                    <path d="M15 18l-6-6 6-6" />
                  )}
                </svg>
              </button>
              <span>Workspaces</span>
              {selectedProjectName && (
                <span className={styles.inProject}>in {selectedProjectName}</span>
              )}
            </div>
          </div>

          <div className={styles.stats}>
            <button
              type="button"
              className={`${styles.stat} ${styles.statAttention} ${selectedStatuses.includes('attention') ? styles.statActive : ''}`}
              onClick={() => toggleStatus('attention')}
            >
              <div className={styles.statKey}>
                <span className={`${styles.dot} ${styles.dotAttention}`} />
                Needs attention
              </div>
              <div className={styles.statValue}>{statusStats.attention}</div>
            </button>
            <button
              type="button"
              className={`${styles.stat} ${styles.statError} ${selectedStatuses.includes('error') ? styles.statActive : ''}`}
              onClick={() => toggleStatus('error')}
            >
              <div className={styles.statKey}>
                <span className={`${styles.dot} ${styles.dotError}`} />
                Failed
              </div>
              <div className={styles.statValue}>{statusStats.error}</div>
            </button>
            <button
              type="button"
              className={`${styles.stat} ${styles.statRunning} ${selectedStatuses.includes('running') ? styles.statActive : ''}`}
              onClick={() => toggleStatus('running')}
            >
              <div className={styles.statKey}>
                <span className={`${styles.dot} ${styles.dotRunning}`} />
                Running
              </div>
              <div className={styles.statValue}>{statusStats.running}</div>
            </button>
            <button
              type="button"
              className={`${styles.stat} ${styles.statSuccess} ${selectedStatuses.includes('success') ? styles.statActive : ''}`}
              onClick={() => toggleStatus('success')}
            >
              <div className={styles.statKey}>
                <span className={`${styles.dot} ${styles.dotSuccess}`} />
                Healthy
              </div>
              <div className={styles.statValue}>{statusStats.success}</div>
            </button>
          </div>

          <div className={styles.tools}>
            <div className={styles.searchBox}>
              <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
                <circle cx="11" cy="11" r="8" />
                <path d="M21 21l-4.35-4.35" />
              </svg>
              <input
                type="text"
                placeholder="Search workspaces…"
                value={searchInput}
                onChange={(e) => setSearchInput(e.target.value)}
                aria-label="Search workspaces"
              />
            </div>

            <div className={styles.filterWrap}>
              <button
                type="button"
                className={`${styles.filterBtn} ${selectedTags.length > 0 ? styles.filterBtnActive : ''}`}
                onClick={() => setShowTagsDropdown(!showTagsDropdown)}
              >
                Tags{selectedTags.length > 0 ? ` (${selectedTags.length})` : ''} ▾
              </button>
              {showTagsDropdown && (
                <>
                  <div
                    className={styles.dropdownOverlay}
                    onClick={() => setShowTagsDropdown(false)}
                  />
                  <div className={styles.filterDropdown}>
                    {availableTags.length === 0 ? (
                      <div className={styles.filterDropdownEmpty}>No tags available</div>
                    ) : (
                      availableTags.map((tag) => (
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
              <button type="button" className={styles.clearBtn} onClick={clearAllFilters}>
                × Clear filters
              </button>
            )}
          </div>

          <div className={styles.tableHeader}>
            <div>Workspace</div>
            <div>Status</div>
            <div>Mode</div>
            <div>Updated</div>
          </div>

          <div className={styles.tableBody}>
            {paginatedWorkspaces.length === 0 ? (
              <div className={styles.emptyState}>
                <p>没有找到匹配的工作空间</p>
                {hasActiveFilters && (
                  <button type="button" className={styles.clearBtn} onClick={clearAllFilters}>
                    Clear filters
                  </button>
                )}
              </div>
            ) : (
              paginatedWorkspaces.map((workspace) => {
                const status = workspace.latestRunStatus;
                const tags = tagEntries(workspace.tags).slice(0, 3);
                const id = workspace.workspace_id ?? String(workspace.id);

                return (
                  <Link
                    key={id}
                    to={`/workspaces/${id}`}
                    className={styles.row}
                  >
                    <div className={styles.wsCell}>
                      <div className={`${styles.wsMark} ${markClassForStatus(status)}`}>
                        {getInitials(workspace.name)}
                      </div>
                      <div className={styles.wsText}>
                        <div className={styles.wsName}>{workspace.name}</div>
                        {workspace.description && (
                          <div className={styles.wsDesc}>{workspace.description}</div>
                        )}
                        {tags.length > 0 && (
                          <div className={styles.tags}>
                            {tags.map((t) => (
                              <span key={t} className={styles.tag}>
                                {t}
                              </span>
                            ))}
                          </div>
                        )}
                      </div>
                    </div>
                    <div>
                      {status ? (
                        <span className={`${styles.badge} ${badgeClassForStatus(status)}`}>
                          <span className={styles.badgePulse} />
                          {getTaskStatusLabel({
                            status,
                            task_type: workspace.latestRunTaskType,
                          })}
                        </span>
                      ) : (
                        <span className={`${styles.badge} ${styles.badgeNeutral}`}>—</span>
                      )}
                    </div>
                    <div>
                      <span className={styles.modeCode}>{workspace.execution_mode || '—'}</span>
                    </div>
                    <div className={styles.time}>
                      {formatRelativeTime(workspace.latestApplyTime)}
                    </div>
                  </Link>
                );
              })
            )}
          </div>

          <div className={styles.tableFooter}>
            <span>{rangeLabel}</span>
            <div className={styles.tablePager}>
              <button
                type="button"
                className={styles.tablePagerBtn}
                disabled={currentPage <= 1}
                onClick={() => setCurrentPage((p) => Math.max(1, p - 1))}
                aria-label="Previous page"
              >
                ‹
              </button>
              <span className={styles.pageLabel}>
                {currentPage} / {totalPages}
              </span>
              <button
                type="button"
                className={styles.tablePagerBtn}
                disabled={currentPage >= totalPages || filteredWorkspaces.length === 0}
                onClick={() => setCurrentPage((p) => Math.min(totalPages, p + 1))}
                aria-label="Next page"
              >
                ›
              </button>
            </div>
          </div>
        </section>
      </div>

      {/* Create project dialog */}
      {showCreateProjectDialog && (
        <div className={styles.dialogOverlay}>
          <div className={styles.dialogContent} role="dialog" aria-labelledby="create-project-title">
            <div className={styles.dialogHeader}>
              <h3 id="create-project-title" className={styles.dialogTitle}>
                创建项目
              </h3>
              <button
                type="button"
                className={styles.dialogClose}
                onClick={() => setShowCreateProjectDialog(false)}
                aria-label="Close"
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
                type="button"
                className={styles.cancelButton}
                onClick={() => setShowCreateProjectDialog(false)}
              >
                取消
              </button>
              <button
                type="button"
                className={styles.btnPrimary}
                onClick={handleCreateProject}
                disabled={
                  createProjectLoading ||
                  !newProjectName.trim() ||
                  !newProjectDisplayName.trim()
                }
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

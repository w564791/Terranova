import React, { useEffect, useState, useRef, useMemo } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import { Modal, Form, Input, Alert, Tag } from 'antd';
import { WarningOutlined } from '@ant-design/icons';
import { useToast } from '../contexts/ToastContext';
import { extractErrorMessage } from '../utils/errorHandler';
import api from '../services/api';
import { stateAPI } from '../services/state';
import StateResourceViewer from '../components/StateResourceViewer';
import WorkspaceSidebar from '../components/WorkspaceSidebar';
import type { StateContent } from '../utils/stateParser';
import styles from './StatePreview.module.css';

const { TextArea } = Input;

interface StateData {
  version: number;
  terraform_version: string;
  serial: number;
  lineage: string;
  resources: any[];
  outputs: any;
}

interface Match {
  index: number;
  start: number;
  end: number;
  line: number;
}

type RetrieveStatus = 'idle' | 'loading' | 'success' | 'error' | 'no_permission';
type ViewMode = 'json' | 'resources';

const StatePreview: React.FC = () => {
  const { workspaceId, version } = useParams<{ workspaceId: string; version: string }>();
  const navigate = useNavigate();
  const { showToast } = useToast();
  const [workspace, setWorkspace] = useState<any>(null);
  const [stateData, setStateData] = useState<StateData | null>(null);
  const [stateMetadata, setStateMetadata] = useState<any>(null);
  const [loading, setLoading] = useState(true);
  // State 内容获取状态
  const [retrieveStatus, setRetrieveStatus] = useState<RetrieveStatus>('idle');
  const [retrieveError, setRetrieveError] = useState<string>('');
  const [overviewResourceCount, setOverviewResourceCount] = useState<number>(0);
  const [searchTerm, setSearchTerm] = useState('');
  const [caseSensitive, setCaseSensitive] = useState(false);
  const [currentMatchIndex, setCurrentMatchIndex] = useState(0);
  const [matches, setMatches] = useState<Match[]>([]);
  const [showNewRunDialog, setShowNewRunDialog] = useState(false);
  const [viewMode, setViewMode] = useState<ViewMode>('resources');
  
  // 回滚相关状态
  const [rollbackModalVisible, setRollbackModalVisible] = useState(false);
  const [rollbackReason, setRollbackReason] = useState('');
  const [rollbackLoading, setRollbackLoading] = useState(false);
  const [rollbackForce, setRollbackForce] = useState(false);
  const [currentVersion, setCurrentVersion] = useState(0);
  const jsonViewerRef = useRef<HTMLDivElement>(null);
  const matchRefs = useRef<Map<number, HTMLSpanElement>>(new Map());

  useEffect(() => {
    fetchWorkspace();
    fetchStateMetadata();
    fetchCurrentVersion();
    // 不再自动获取 state 内容，需要用户点击 Retrieve 按钮
    // fetchStateData();
    setLoading(false);
    setRetrieveStatus('idle');
    setStateData(null);
  }, [workspaceId, version]);
  
  const fetchCurrentVersion = async () => {
    try {
      const response = await stateAPI.getStateVersions(workspaceId!, 1, 0);
      setCurrentVersion(response.current_version || 0);
    } catch (error) {
      console.error('Failed to fetch current version:', error);
    }
  };
  
  const fetchStateMetadata = async () => {
    try {
      const response: any = await api.get(`/workspaces/${workspaceId}/state-versions/${version}/metadata`);
      // API 返回格式: { code: 200, data: { ... }, timestamp: ... }
      // api 拦截器可能已经返回 response.data，所以需要检查
      const metadata = response?.data?.data || response?.data || response;
      setStateMetadata(metadata);
    } catch (error) {
      console.error('Failed to fetch state metadata:', error);
    }
  };

  // Search for matches when search term changes
  useEffect(() => {
    if (!stateData || !searchTerm.trim()) {
      setMatches([]);
      setCurrentMatchIndex(0);
      return;
    }

    const jsonStr = JSON.stringify(stateData, null, 2);
    const searchPattern = caseSensitive ? searchTerm : searchTerm.toLowerCase();
    const searchIn = caseSensitive ? jsonStr : jsonStr.toLowerCase();
    
    const foundMatches: Match[] = [];
    let index = 0;
    let matchIndex = 0;
    
    while ((index = searchIn.indexOf(searchPattern, index)) !== -1) {
      // Calculate line number
      const beforeMatch = jsonStr.substring(0, index);
      const line = beforeMatch.split('\n').length;
      
      foundMatches.push({
        index: matchIndex++,
        start: index,
        end: index + searchTerm.length,
        line
      });
      index += searchTerm.length;
    }
    
    setMatches(foundMatches);
    setCurrentMatchIndex(foundMatches.length > 0 ? 0 : -1);
  }, [searchTerm, caseSensitive, stateData]);

  // Scroll to current match
  useEffect(() => {
    if (matches.length > 0 && currentMatchIndex >= 0) {
      const matchElement = matchRefs.current.get(currentMatchIndex);
      if (matchElement) {
        matchElement.scrollIntoView({
          behavior: 'smooth',
          block: 'center'
        });
      }
    }
  }, [currentMatchIndex, matches]);

  const fetchWorkspace = async () => {
    try {
      const data: any = await api.get(`/workspaces/${workspaceId}`);
      setWorkspace(data.data || data);
      
      // 获取overview数据中的resource_count（与WorkspaceDetail.tsx保持一致）
      const overviewResponse = await api.get(`/workspaces/${workspaceId}/overview`);
      const overviewData = overviewResponse.data || overviewResponse;
      setOverviewResourceCount(overviewData.resource_count || 0);
    } catch (err: any) {
      console.error('Failed to fetch workspace:', err);
    }
  };

  // 用户点击 "Retrieve State" 按钮后才获取内容
  const handleRetrieveState = async () => {
    setRetrieveStatus('loading');
    setRetrieveError('');
    try {
      const result = await stateAPI.retrieveStateContent(workspaceId!, parseInt(version!));
      setStateData(result.data.content as StateData);
      setRetrieveStatus('success');
    } catch (err: any) {
      // 检查是否是权限不足错误
      if (err?.response?.status === 403 || err?.status === 403) {
        setRetrieveStatus('no_permission');
        setRetrieveError('您没有 WORKSPACE_STATE_SENSITIVE 权限，无法查看 State 文件内容。');
      } else {
        setRetrieveStatus('error');
        setRetrieveError(extractErrorMessage(err));
      }
    }
  };

  // 打开回滚对话框
  const handleOpenRollback = () => {
    setRollbackReason('');
    setRollbackForce(false);
    setRollbackModalVisible(true);
  };

  // 确认回滚
  const handleConfirmRollback = async () => {
    if (!rollbackReason.trim()) {
      showToast('请填写回滚原因', 'warning');
      return;
    }

    setRollbackLoading(true);
    try {
      const response = await stateAPI.rollbackState(workspaceId!, {
        target_version: parseInt(version!),
        reason: rollbackReason,
        force: rollbackForce,
      });

      let successMsg = `回滚成功！新版本: #${response.new_version}`;
      if (response.warnings && response.warnings.length > 0) {
        successMsg += ` (${response.warnings.length} 条警告)`;
      }
      showToast(successMsg, 'success');

      setRollbackModalVisible(false);
      // 跳转到新版本
      navigate(`/workspaces/${workspaceId}/states/${response.new_version}`);
    } catch (error: any) {
      const errMsg = typeof error === 'string' ? error : (error?.message || '未知错误');
      showToast('回滚失败: ' + errMsg, 'error');
    } finally {
      setRollbackLoading(false);
    }
  };

  const handleDownload = async () => {
    try {
      const token = localStorage.getItem('token');
      const response = await fetch(
        `http://localhost:8080/api/v1/workspaces/${workspaceId}/state-versions/${version}`,
        {
          headers: {
            'Authorization': `Bearer ${token}`
          }
        }
      );
      
      if (!response.ok) {
        throw new Error('下载失败');
      }
      
      const blob = await response.blob();
      const url = window.URL.createObjectURL(blob);
      const link = document.createElement('a');
      link.href = url;
      link.setAttribute('download', `${workspace?.name || 'state'}-v${version}.tfstate`);
      document.body.appendChild(link);
      link.click();
      link.remove();
      window.URL.revokeObjectURL(url);
      
      showToast('State文件下载成功', 'success');
    } catch (error) {
      const message = extractErrorMessage(error);
      showToast(message, 'error');
    }
  };

  const handlePreviousMatch = () => {
    if (matches.length === 0) return;
    const newIndex = currentMatchIndex > 0 ? currentMatchIndex - 1 : matches.length - 1;
    setCurrentMatchIndex(newIndex);
  };

  const handleNextMatch = () => {
    if (matches.length === 0) return;
    const newIndex = currentMatchIndex < matches.length - 1 ? currentMatchIndex + 1 : 0;
    setCurrentMatchIndex(newIndex);
  };

  const formatRelativeTime = (dateString: string | null) => {
    if (!dateString) return '从未';
    
    // 处理无效日期
    if (dateString.startsWith('0001-01-01')) return '从未';
    
    // WORKAROUND: 后端存储的是本地时间但添加了Z后缀
    // 移除Z后缀，将其作为本地时间解析
    let normalizedDateString = dateString;
    if (dateString.endsWith('Z')) {
      normalizedDateString = dateString.slice(0, -1);
    }
    
    const date = new Date(normalizedDateString);
    const now = new Date();
    
    // 验证日期是否有效
    if (isNaN(date.getTime())) return '无效日期';
    
    const diffMs = now.getTime() - date.getTime();
    const diffMins = Math.floor(diffMs / 60000);
    const diffHours = Math.floor(diffMs / 3600000);
    const diffDays = Math.floor(diffMs / 86400000);

    // 5分钟以内显示"刚刚"
    if (diffMins < 5) return '刚刚';
    // 1小时以内显示"X分钟前"
    if (diffMins < 60) return `${diffMins}分钟前`;
    // 24小时以内显示"X小时前"
    if (diffHours < 24) return `${diffHours}小时前`;
    // 超过1天显示具体日期时间（精确到分钟）
    return date.toLocaleString('zh-CN', {
      year: 'numeric',
      month: '2-digit',
      day: '2-digit',
      hour: '2-digit',
      minute: '2-digit'
    });
  };

  // Syntax highlighting function
  const highlightJSON = (json: string): React.ReactNode[] => {
    const tokens: React.ReactNode[] = [];
    let currentIndex = 0;
    
    interface Token {
      start: number;
      end: number;
      text: string;
      className: string;
      type: string;
    }
    
    const allTokens: Token[] = [];
    
    // First, find all keys (property names followed by colon)
    const keyRegex = /"([^"]+)"(?=\s*:)/g;
    let match;
    while ((match = keyRegex.exec(json)) !== null) {
      allTokens.push({
        start: match.index,
        end: match.index + match[0].length,
        text: match[0],
        className: styles.jsonKey,
        type: 'key'
      });
    }
    
    // Then find all other patterns (excluding keys)
    const valuePatterns = [
      { type: 'string', regex: /"(?:[^"\\]|\\.)*"/g, className: styles.jsonString },
      { type: 'number', regex: /\b-?\d+\.?\d*\b/g, className: styles.jsonNumber },
      { type: 'boolean', regex: /\b(true|false)\b/g, className: styles.jsonBoolean },
      { type: 'null', regex: /\bnull\b/g, className: styles.jsonNull }
    ];
    
    valuePatterns.forEach(pattern => {
      const regex = new RegExp(pattern.regex.source, 'g');
      let match;
      while ((match = regex.exec(json)) !== null) {
        allTokens.push({
          start: match.index,
          end: match.index + match[0].length,
          text: match[0],
          className: pattern.className,
          type: pattern.type
        });
      }
    });

    // Sort tokens by start position
    allTokens.sort((a, b) => a.start - b.start);

    // Remove overlapping tokens (keys take priority)
    const filteredTokens: Token[] = [];
    let lastEnd = 0;
    allTokens.forEach(token => {
      if (token.start >= lastEnd) {
        filteredTokens.push(token);
        lastEnd = token.end;
      }
    });

    // Build the highlighted output
    let tokenIndex = 0;
    filteredTokens.forEach((token) => {
      // Add plain text before this token
      if (token.start > currentIndex) {
        const plainText = json.substring(currentIndex, token.start);
        tokens.push(
          <span key={`plain-${tokenIndex++}`}>
            {highlightSearchMatches(plainText, currentIndex)}
          </span>
        );
      }

      // Add the highlighted token
      tokens.push(
        <span key={`token-${tokenIndex++}`} className={token.className}>
          {highlightSearchMatches(token.text, token.start)}
        </span>
      );

      currentIndex = token.end;
    });

    // Add remaining plain text
    if (currentIndex < json.length) {
      const plainText = json.substring(currentIndex);
      tokens.push(
        <span key={`plain-${tokenIndex++}`}>
          {highlightSearchMatches(plainText, currentIndex)}
        </span>
      );
    }

    return tokens;
  };

  // Highlight search matches within a text segment
  const highlightSearchMatches = (text: string, offset: number): React.ReactNode[] => {
    if (!searchTerm.trim() || matches.length === 0) {
      return [text];
    }

    const result: React.ReactNode[] = [];
    let lastIndex = 0;

    matches.forEach((match) => {
      const relativeStart = match.start - offset;
      const relativeEnd = match.end - offset;

      if (relativeStart >= 0 && relativeStart < text.length) {
        // Add text before match
        if (relativeStart > lastIndex) {
          result.push(text.substring(lastIndex, relativeStart));
        }

        // Add highlighted match
        const matchText = text.substring(
          Math.max(0, relativeStart),
          Math.min(text.length, relativeEnd)
        );
        
        const isCurrentMatch = match.index === currentMatchIndex;
        result.push(
          <span
            key={`match-${match.index}`}
            ref={(el) => {
              if (el) matchRefs.current.set(match.index, el);
            }}
            className={isCurrentMatch ? styles.currentMatch : styles.searchMatch}
          >
            {matchText}
          </span>
        );

        lastIndex = Math.min(text.length, relativeEnd);
      }
    });

    // Add remaining text
    if (lastIndex < text.length) {
      result.push(text.substring(lastIndex));
    }

    return result.length > 0 ? result : [text];
  };

  const highlightedJSON = useMemo(() => {
    if (!stateData) return null;
    const jsonStr = JSON.stringify(stateData, null, 2);
    return highlightJSON(jsonStr);
  }, [stateData, searchTerm, matches, currentMatchIndex]);

  if (loading) {
    return (
      <div className={styles.container}>
        <div className={styles.loading}>加载中...</div>
      </div>
    );
  }

  return (
    <div className={styles.stateLayout}>
      {/* 左侧导航栏 - 使用共享组件 */}
      <WorkspaceSidebar
        workspaceId={workspaceId!}
        workspaceName={workspace?.name || `Workspace #${workspaceId}`}
        activeTab="states"
      />

      {/* 右侧主内容区 */}
      <main className={styles.mainContent}>
        {/* 全局头部 */}
        {workspace && (
          <div className={styles.globalHeader}>
            <div className={styles.globalHeaderLeft}>
              <h1 className={styles.globalTitle}>{workspace.name}</h1>
              <div className={styles.globalMeta}>
                <span className={styles.metaItem}>ID: {workspace.workspace_id}</span>
                <span className={styles.metaItem}>
                  {workspace.is_locked ? 'Locked' : 'Unlocked'}
                </span>
                <span className={styles.metaItem}>
                  Resources {overviewResourceCount}
                </span>
                <span className={styles.metaItem}>
                  Terraform {workspace.terraform_version || 'latest'}
                </span>
                <span className={styles.metaItem}>
                  Updated {formatRelativeTime(workspace.updated_at)}
                </span>
              </div>
            </div>
            <div className={styles.globalHeaderRight}>
              <button 
                className={styles.lockButton}
                onClick={() => navigate(`/workspaces/${workspaceId}`)}
              >
                {workspace.is_locked ? 'Unlock' : 'Lock'}
              </button>
              <button 
                className={styles.newRunButton}
                onClick={() => setShowNewRunDialog(true)}
              >
                + New run
              </button>
            </div>
          </div>
        )}

        {/* State预览内容 */}
        <div className={styles.stateContent}>
          {/* 返回按钮和操作栏 */}
          <div className={styles.stateHeader}>
            <button 
              onClick={() => navigate(`/workspaces/${workspaceId}?tab=states`)}
              className={styles.backToListButton}
            >
              ← Back to States
            </button>
            <div className={styles.stateActions}>
              {stateMetadata?.task_id && (
                <button
                  onClick={() => navigate(`/workspaces/${workspaceId}/tasks/${stateMetadata.task_id}`)}
                  className={styles.viewTaskButton}
                >
                  View Task #{stateMetadata.task_id} →
                </button>
              )}
              <button onClick={handleDownload} className={styles.downloadButton}>
                Download
              </button>
              {parseInt(version!) !== currentVersion && (
                <button onClick={handleOpenRollback} className={styles.rollbackButton}>
                  Rollback to this version
                </button>
              )}
            </div>
          </div>

          {/* State信息卡片 */}
          <div className={styles.stateInfoCard}>
            <h2 className={styles.stateTitle}>
              State Version {version}
              {parseInt(version!) === currentVersion && (
                <Tag color="green" style={{ marginLeft: 12 }}>Current</Tag>
              )}
              {stateMetadata?.is_imported && (
                <Tag color="blue" style={{ marginLeft: 8 }}>Imported</Tag>
              )}
              {stateMetadata?.is_rollback && (
                <Tag color="orange" style={{ marginLeft: 8 }}>
                  Rollback from #{stateMetadata.rollback_from_version}
                </Tag>
              )}
            </h2>
            <div className={styles.stateMetadata}>
              <div className={styles.metadataItem}>
                <span className={styles.metadataLabel}>Terraform Version:</span>
                <span className={styles.metadataValue}>{stateMetadata?.terraform_version || stateData?.terraform_version || 'N/A'}</span>
              </div>
              <div className={styles.metadataItem}>
                <span className={styles.metadataLabel}>Serial:</span>
                <span className={styles.metadataValue}>{stateMetadata?.serial || stateData?.serial || 0}</span>
              </div>
              <div className={styles.metadataItem}>
                <span className={styles.metadataLabel}>Resources:</span>
                <span className={styles.metadataValue}>{stateMetadata?.resources_count || stateData?.resources?.length || 0}</span>
              </div>
              {stateMetadata?.task_id && (
                <div className={styles.metadataItem}>
                  <span className={styles.metadataLabel}>Created by Task:</span>
                  <span className={styles.metadataValue}>#{stateMetadata.task_id}</span>
                </div>
              )}
            </div>
          </div>

          {/* 视图切换和搜索过滤器 */}
          <div className={styles.filterSection}>
            {/* 视图切换按钮 */}
            <div className={styles.viewToggle}>
              <button
                className={`${styles.viewToggleButton} ${viewMode === 'resources' ? styles.viewToggleActive : ''}`}
                onClick={() => setViewMode('resources')}
              >
                Resources
              </button>
              <button
                className={`${styles.viewToggleButton} ${viewMode === 'json' ? styles.viewToggleActive : ''}`}
                onClick={() => setViewMode('json')}
              >
                JSON
              </button>
            </div>

            {/* JSON 视图的搜索功能 */}
            {viewMode === 'json' && (
              <>
                <input
                  type="text"
                  placeholder="Search in JSON..."
                  value={searchTerm}
                  onChange={(e) => setSearchTerm(e.target.value)}
                  className={styles.filterInput}
                />
                
                {matches.length > 0 && (
                  <>
                    <div className={styles.matchCounter}>
                      {currentMatchIndex + 1} / {matches.length}
                    </div>
                    <button 
                      onClick={handlePreviousMatch}
                      className={styles.navButton}
                      title="Previous match"
                    >
                      ← Prev
                    </button>
                    <button 
                      onClick={handleNextMatch}
                      className={styles.navButton}
                      title="Next match"
                    >
                      Next →
                    </button>
                  </>
                )}
                
                <label className={styles.caseSensitiveLabel}>
                  <input
                    type="checkbox"
                    checked={caseSensitive}
                    onChange={(e) => setCaseSensitive(e.target.checked)}
                    className={styles.caseSensitiveCheckbox}
                  />
                  Case sensitive
                </label>
                
                <div className={styles.filterActions}>
                  <button className={styles.expandButton}>⊕ Expand</button>
                  <button className={styles.fullScreenButton}>⛶ Full screen</button>
                </div>
              </>
            )}
          </div>

          {/* JSON查看器 */}
          <div className={styles.jsonViewer} ref={jsonViewerRef}>
            {/* 初始状态：显示居中的 Retrieve 按钮 */}
            {retrieveStatus === 'idle' && (
              <div className={styles.retrievePrompt}>
                <div className={styles.retrieveIcon}>Locked</div>
                <p className={styles.retrieveText}>State 内容包含敏感数据，需要显式请求查看</p>
                <button 
                  className={styles.retrieveButton}
                  onClick={handleRetrieveState}
                >
                  Retrieve State
                </button>
              </div>
            )}

            {/* 加载中 */}
            {retrieveStatus === 'loading' && (
              <div className={styles.retrievePrompt}>
                <div className={styles.loadingSpinner}></div>
                <p className={styles.retrieveText}>正在获取 State 内容...</p>
              </div>
            )}

            {/* 无权限 */}
            {retrieveStatus === 'no_permission' && (
              <div className={styles.retrievePrompt}>
                <div className={styles.retrieveIcon}>Warning</div>
                <Alert
                  type="warning"
                  message="无权限查看 State 内容"
                  description={retrieveError}
                  style={{ maxWidth: 400 }}
                />
              </div>
            )}

            {/* 错误 */}
            {retrieveStatus === 'error' && (
              <div className={styles.retrievePrompt}>
                <Alert
                  type="error"
                  message="获取 State 失败"
                  description={retrieveError}
                  style={{ maxWidth: 400 }}
                />
                <button 
                  className={styles.retryButton}
                  onClick={handleRetrieveState}
                  style={{ marginTop: 16 }}
                >
                  重试
                </button>
              </div>
            )}

            {/* 成功获取：显示 State 内容 */}
            {retrieveStatus === 'success' && stateData && (
              <>
                {viewMode === 'json' ? (
                  <pre className={styles.jsonContent}>
                    <code>{highlightedJSON}</code>
                  </pre>
                ) : (
                  <div className={styles.resourceViewContainer}>
                    <StateResourceViewer
                      stateContent={stateData as StateContent}
                      showSensitive={true}
                    />
                  </div>
                )}
              </>
            )}
          </div>
        </div>
      </main>

      {/* 回滚确认对话框 */}
      <Modal
        title="确认回滚 State"
        open={rollbackModalVisible}
        onOk={handleConfirmRollback}
        onCancel={() => setRollbackModalVisible(false)}
        confirmLoading={rollbackLoading}
        okText="确认回滚"
        cancelText="取消"
        okButtonProps={{ danger: true }}
        width={600}
      >
        <Alert
          type="warning"
          message="回滚操作将创建新的 State 版本"
          style={{ marginBottom: 16 }}
          icon={<WarningOutlined />}
        />

        <div className={styles.rollbackInfo}>
          <p>
            <strong>当前版本：</strong>#{currentVersion}
          </p>
          <p>
            <strong>回滚到版本：</strong>#{version}
          </p>
          <p>
            <strong>新版本号：</strong>#{currentVersion + 1} (标记为从 #{version} 回滚)
          </p>
        </div>

        <Form.Item label="回滚原因" required>
          <TextArea
            rows={4}
            placeholder="请说明回滚原因，例如：修复错误的 state 上传、恢复到稳定版本等"
            value={rollbackReason}
            onChange={(e) => setRollbackReason(e.target.value)}
            maxLength={500}
            showCount
          />
        </Form.Item>

        <Form.Item>
          <label style={{ display: 'flex', alignItems: 'center', gap: 8, cursor: 'pointer' }}>
            <input
              type="checkbox"
              checked={rollbackForce}
              onChange={(e) => setRollbackForce(e.target.checked)}
            />
            <span>强制回滚（跳过 lineage/serial 校验）</span>
          </label>
        </Form.Item>

        {rollbackForce && (
          <Alert
            type="error"
            message=" 危险操作警告"
            description={
              <div>
                <p><strong>强制回滚将跳过所有安全校验，可能导致：</strong></p>
                <ul style={{ margin: '8px 0', paddingLeft: 20 }}>
                  <li>覆盖其他用户的更改</li>
                  <li>State 不一致</li>
                  <li>资源管理混乱</li>
                </ul>
              </div>
            }
            style={{ marginBottom: 16 }}
          />
        )}

        <Alert
          type="info"
          message="提示"
          description={rollbackForce 
            ? "强制回滚完成后 workspace 会保持锁定状态，需要手动解锁。"
            : "回滚会先校验 lineage/serial，如果校验失败请勾选强制回滚。回滚完成后 workspace 会自动解锁。"
          }
          style={{ marginTop: 16 }}
        />
      </Modal>
    </div>
  );
};

export default StatePreview;

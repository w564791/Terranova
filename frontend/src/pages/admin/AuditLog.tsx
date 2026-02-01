import React, { useState, useEffect } from 'react';
import { iamService } from '../../services/iam';
import { useToast } from '../../contexts/ToastContext';
import ConfirmDialog from '../../components/ConfirmDialog';
import styles from './AuditLog.module.css';

const AuditLog: React.FC = () => {
  const { success: showSuccess, error: showError } = useToast();
  const [logs, setLogs] = useState<any[]>([]);
  const [loading, setLoading] = useState(false);
  const [logType, setLogType] = useState<'access' | 'denied'>('access');
  const [startTime, setStartTime] = useState('');
  const [endTime, setEndTime] = useState('');
  const [limit, setLimit] = useState(100);
  const [includeBody, setIncludeBody] = useState(false);
  const [includeHeaders, setIncludeHeaders] = useState(false);
  const [expandedLog, setExpandedLog] = useState<number | null>(null);
  const [auditEnabled, setAuditEnabled] = useState(true); // 审计日志总开关
  
  // 新增：高级搜索字段
  const [method, setMethod] = useState('');
  const [httpCodeOperator, setHttpCodeOperator] = useState('');
  const [httpCodeValue, setHttpCodeValue] = useState('');

  // 确认对话框状态
  const [confirmDialog, setConfirmDialog] = useState<{
    isOpen: boolean;
    title: string;
    message: string;
    type: 'info' | 'warning' | 'danger';
    onConfirm: () => void;
  }>({
    isOpen: false,
    title: '',
    message: '',
    type: 'info',
    onConfirm: () => {}
  });

  // 时区显示设置
  const [timeZone, setTimeZone] = useState<'local' | 'utc'>('local');

  // 列显示设置
  const [showColumnSettings, setShowColumnSettings] = useState(false);
  const [visibleColumns, setVisibleColumns] = useState({
    time: true,
    username: true,
    path: true,
    resourceType: true,
    action: true,
    httpCode: true,
    result: true,
    denyReason: true,
    ipAddress: true,
    duration: true,
  });

  const toggleColumn = (column: keyof typeof visibleColumns) => {
    setVisibleColumns(prev => ({
      ...prev,
      [column]: !prev[column]
    }));
  };

  const resetColumns = () => {
    setVisibleColumns({
      time: true,
      username: true,
      path: true,
      resourceType: true,
      action: true,
      httpCode: true,
      result: true,
      denyReason: true,
      ipAddress: true,
      duration: true,
    });
  };

  useEffect(() => {
    // 设置默认时间范围（最近7天到现在）- 使用本地时间
    const end = new Date();
    const start = new Date();
    start.setDate(start.getDate() - 7);
    
    // 转换为本地时间格式 (YYYY-MM-DDTHH:mm)
    const formatLocalDateTime = (date: Date) => {
      const year = date.getFullYear();
      const month = String(date.getMonth() + 1).padStart(2, '0');
      const day = String(date.getDate()).padStart(2, '0');
      const hours = String(date.getHours()).padStart(2, '0');
      const minutes = String(date.getMinutes()).padStart(2, '0');
      return `${year}-${month}-${day}T${hours}:${minutes}`;
    };
    
    setStartTime(formatLocalDateTime(start));
    setEndTime(formatLocalDateTime(end));

    // 加载审计配置
    loadAuditConfig();
  }, []);

  useEffect(() => {
    if (startTime && endTime && auditEnabled) {
      loadLogs();
    }
  }, [logType, startTime, endTime, limit, auditEnabled]);

  const loadLogs = async () => {
    setLoading(true);
    try {
      const params: any = {
        start_time: new Date(startTime).toISOString(),
        end_time: new Date(endTime).toISOString(),
        limit,
      };

      // 添加高级搜索参数（仅对访问历史有效）
      if (logType === 'access') {
        if (method) {
          params.method = method;
        }
        if (httpCodeOperator && httpCodeValue) {
          params.http_code_operator = httpCodeOperator;
          params.http_code_value = parseInt(httpCodeValue);
        }
      }

      let data;
      if (logType === 'access') {
        data = await iamService.queryAccessHistory(params);
      } else {
        data = await iamService.queryDeniedAccess(params);
      }
      
      setLogs(data.logs || []);
    } catch (error: any) {
      console.error('加载日志失败:', error);
      showError('加载日志失败: ' + (error.response?.data?.error || error.message));
    } finally {
      setLoading(false);
    }
  };

  const loadAuditConfig = async () => {
    try {
      const config = await iamService.getAuditConfig();
      console.log('Loaded audit config:', config);
      setAuditEnabled(config.enabled);
      setIncludeBody(config.include_body);
      setIncludeHeaders(config.include_headers);
    } catch (error: any) {
      console.error('加载审计配置失败:', error);
    }
  };

  const handleToggleAudit = async () => {
    const newState = !auditEnabled;
    
    // 禁用审计日志需要二次确认
    if (!newState) {
      setConfirmDialog({
        isOpen: true,
        title: '禁用审计日志',
        message: '禁用后将不再记录系统操作日志，这可能影响安全审计和问题追踪。\n\n建议仅在必要时禁用。',
        type: 'warning',
        onConfirm: async () => {
          setConfirmDialog({ ...confirmDialog, isOpen: false });
          try {
            await iamService.updateAuditConfig({
              enabled: newState,
              include_body: includeBody,
              include_headers: includeHeaders,
            });
            setAuditEnabled(newState);
            showSuccess('审计日志已禁用');
          } catch (error: any) {
            showError('更新配置失败: ' + (error.response?.data?.error || error.message));
          }
        }
      });
      return;
    }
    
    // 启用审计日志不需要确认
    try {
      await iamService.updateAuditConfig({
        enabled: newState,
        include_body: includeBody,
        include_headers: includeHeaders,
      });
      setAuditEnabled(newState);
      showSuccess('审计日志已启用');
    } catch (error: any) {
      showError('更新配置失败: ' + (error.response?.data?.error || error.message));
    }
  };

  const handleToggleBody = async (checked: boolean) => {
    // 启用请求体记录需要二次确认并警告
    if (checked) {
      setConfirmDialog({
        isOpen: true,
        title: '启用请求体记录',
        message: ' 警告：启用后会记录所有API请求的请求体内容，这将导致：\n\n• 审计日志数据量大幅增加\n• 可能包含敏感信息（密码、令牌等）\n• 数据库存储空间快速增长\n\n建议仅在调试或特殊审计需求时短期启用。',
        type: 'danger',
        onConfirm: async () => {
          setConfirmDialog({ ...confirmDialog, isOpen: false });
          try {
            await iamService.updateAuditConfig({
              enabled: auditEnabled,
              include_body: checked,
              include_headers: includeHeaders,
            });
            setIncludeBody(checked);
            showSuccess('请求体记录已启用');
          } catch (error: any) {
            showError('更新配置失败: ' + (error.response?.data?.error || error.message));
          }
        }
      });
      return;
    }
    
    // 禁用不需要确认
    try {
      await iamService.updateAuditConfig({
        enabled: auditEnabled,
        include_body: checked,
        include_headers: includeHeaders,
      });
      setIncludeBody(checked);
      showSuccess('请求体记录已禁用');
    } catch (error: any) {
      showError('更新配置失败: ' + (error.response?.data?.error || error.message));
    }
  };

  const handleToggleHeaders = async (checked: boolean) => {
    // 启用请求头记录需要二次确认并警告
    if (checked) {
      setConfirmDialog({
        isOpen: true,
        title: '启用请求头记录',
        message: ' 警告：启用后会记录所有API请求的请求头信息，这将导致：\n\n• 审计日志数据量显著增加\n• 可能包含敏感信息（认证令牌、Cookie等）\n• 数据库存储空间增长\n\n建议仅在调试或特殊审计需求时短期启用。',
        type: 'danger',
        onConfirm: async () => {
          setConfirmDialog({ ...confirmDialog, isOpen: false });
          try {
            await iamService.updateAuditConfig({
              enabled: auditEnabled,
              include_body: includeBody,
              include_headers: checked,
            });
            setIncludeHeaders(checked);
            showSuccess('请求头记录已启用');
          } catch (error: any) {
            showError('更新配置失败: ' + (error.response?.data?.error || error.message));
          }
        }
      });
      return;
    }
    
    // 禁用不需要确认
    try {
      await iamService.updateAuditConfig({
        enabled: auditEnabled,
        include_body: includeBody,
        include_headers: checked,
      });
      setIncludeHeaders(checked);
      showSuccess('请求头记录已禁用');
    } catch (error: any) {
      showError('更新配置失败: ' + (error.response?.data?.error || error.message));
    }
  };

  const handleSearch = () => {
    loadLogs();
  };

  const exportLogs = () => {
    const dataStr = JSON.stringify(logs, null, 2);
    const dataBlob = new Blob([dataStr], { type: 'application/json' });
    const url = URL.createObjectURL(dataBlob);
    const link = document.createElement('a');
    link.href = url;
    link.download = `audit-logs-${logType}-${Date.now()}.json`;
    link.click();
    URL.revokeObjectURL(url);
    showSuccess('日志已导出');
  };

  return (
    <div className={styles.container}>
      <div className={styles.header}>
        <h1>审计日志</h1>
        <div className={styles.headerActions}>
          <button
            onClick={handleToggleAudit}
            className={auditEnabled ? styles.disableButton : styles.enableButton}
          >
            {auditEnabled ? '禁用审计日志' : '启用审计日志'}
          </button>
        </div>
      </div>

      {!auditEnabled && (
        <div className={styles.disabledState}>
          <h3>审计日志未启用</h3>
          <p>审计日志功能当前处于禁用状态</p>
          <p>点击右上角的"启用审计日志"按钮以开始记录系统操作日志</p>
        </div>
      )}

      {auditEnabled && (
        <>
          <div className={styles.configSection}>
            <div className={styles.configTitle}>审计配置</div>
            <div className={styles.configOptions}>
              <label className={styles.configLabel}>
                <input
                  type="checkbox"
                  checked={includeBody}
                  onChange={(e) => handleToggleBody(e.target.checked)}
                  className={styles.checkbox}
                />
                <span>记录请求体</span>
                <span className={styles.configHint}>启用后将记录所有API请求的请求体内容</span>
              </label>
              <label className={styles.configLabel}>
                <input
                  type="checkbox"
                  checked={includeHeaders}
                  onChange={(e) => handleToggleHeaders(e.target.checked)}
                  className={styles.checkbox}
                />
                <span>记录请求头</span>
                <span className={styles.configHint}>启用后将记录所有API请求的请求头信息</span>
              </label>
            </div>
          </div>

          <div className={styles.querySection}>
            <div className={styles.querySectionHeader}>
              <div className={styles.headerLeft}>
                <h3>查询审计日志</h3>
                <div className={styles.timezoneTabs}>
                  <button
                    className={`${styles.timezoneTab} ${timeZone === 'local' ? styles.timezoneTabActive : ''}`}
                    onClick={() => setTimeZone('local')}
                  >
                    本地时间 (UTC+8)
                  </button>
                  <button
                    className={`${styles.timezoneTab} ${timeZone === 'utc' ? styles.timezoneTabActive : ''}`}
                    onClick={() => setTimeZone('utc')}
                  >
                    UTC时间
                  </button>
                </div>
              </div>
              <div className={styles.headerButtons}>
                <button 
                  onClick={() => setShowColumnSettings(!showColumnSettings)}
                  className={styles.columnSettingsButton}
                >
                  {showColumnSettings ? '隐藏列设置' : '列设置'}
                </button>
                <button 
                  onClick={exportLogs} 
                  className={styles.exportButton}
                  disabled={logs.length === 0}
                >
                  导出日志
                </button>
              </div>
            </div>

            {/* 列显示设置面板 */}
            {showColumnSettings && (
              <div className={styles.columnSettings}>
                <div className={styles.columnSettingsHeader}>
                  <span>选择要显示的列</span>
                  <button onClick={resetColumns} className={styles.resetButton}>
                    重置
                  </button>
                </div>
                <div className={styles.columnCheckboxes}>
                  <label className={styles.columnCheckbox}>
                    <input
                      type="checkbox"
                      checked={visibleColumns.time}
                      onChange={() => toggleColumn('time')}
                    />
                    <span>时间</span>
                  </label>
                  <label className={styles.columnCheckbox}>
                    <input
                      type="checkbox"
                      checked={visibleColumns.username}
                      onChange={() => toggleColumn('username')}
                    />
                    <span>用户名</span>
                  </label>
                  <label className={styles.columnCheckbox}>
                    <input
                      type="checkbox"
                      checked={visibleColumns.path}
                      onChange={() => toggleColumn('path')}
                    />
                    <span>请求路径</span>
                  </label>
                  <label className={styles.columnCheckbox}>
                    <input
                      type="checkbox"
                      checked={visibleColumns.resourceType}
                      onChange={() => toggleColumn('resourceType')}
                    />
                    <span>资源类型</span>
                  </label>
                  <label className={styles.columnCheckbox}>
                    <input
                      type="checkbox"
                      checked={visibleColumns.action}
                      onChange={() => toggleColumn('action')}
                    />
                    <span>操作</span>
                  </label>
                  <label className={styles.columnCheckbox}>
                    <input
                      type="checkbox"
                      checked={visibleColumns.httpCode}
                      onChange={() => toggleColumn('httpCode')}
                    />
                    <span>HTTP状态码</span>
                  </label>
                  <label className={styles.columnCheckbox}>
                    <input
                      type="checkbox"
                      checked={visibleColumns.result}
                      onChange={() => toggleColumn('result')}
                    />
                    <span>结果</span>
                  </label>
                  {logType === 'denied' && (
                    <label className={styles.columnCheckbox}>
                      <input
                        type="checkbox"
                        checked={visibleColumns.denyReason}
                        onChange={() => toggleColumn('denyReason')}
                      />
                      <span>拒绝原因</span>
                    </label>
                  )}
                  <label className={styles.columnCheckbox}>
                    <input
                      type="checkbox"
                      checked={visibleColumns.ipAddress}
                      onChange={() => toggleColumn('ipAddress')}
                    />
                    <span>IP地址</span>
                  </label>
                  <label className={styles.columnCheckbox}>
                    <input
                      type="checkbox"
                      checked={visibleColumns.duration}
                      onChange={() => toggleColumn('duration')}
                    />
                    <span>耗时</span>
                  </label>
                </div>
              </div>
            )}
          </div>

          <div className={styles.filters}>
        <div className={styles.filterGroup}>
          <label>日志类型:</label>
          <select
            value={logType}
            onChange={(e) => setLogType(e.target.value as 'access' | 'denied')}
            className={styles.select}
          >
            <option value="access">访问历史</option>
            <option value="denied">被拒绝的访问</option>
          </select>
        </div>

        <div className={styles.filterGroup}>
          <label>开始时间:</label>
          <input
            type="datetime-local"
            value={startTime}
            onChange={(e) => setStartTime(e.target.value)}
            className={styles.input}
          />
        </div>

        <div className={styles.filterGroup}>
          <label>结束时间:</label>
          <input
            type="datetime-local"
            value={endTime}
            onChange={(e) => setEndTime(e.target.value)}
            className={styles.input}
          />
        </div>

        {/* 高级搜索 - 仅对访问历史显示 */}
        {logType === 'access' && (
          <>
            <div className={styles.filterGroup}>
              <label>请求Method:</label>
              <select
                value={method}
                onChange={(e) => setMethod(e.target.value)}
                className={styles.select}
              >
                <option value="">全部</option>
                <option value="GET">GET</option>
                <option value="POST">POST</option>
                <option value="PUT">PUT</option>
                <option value="DELETE">DELETE</option>
                <option value="PATCH">PATCH</option>
              </select>
            </div>

            <div className={styles.filterGroup}>
              <label>HTTP状态码:</label>
              <div className={styles.httpCodeFilter}>
                <select
                  value={httpCodeOperator}
                  onChange={(e) => setHttpCodeOperator(e.target.value)}
                  className={styles.operatorSelect}
                >
                  <option value="">不筛选</option>
                  <option value="=">=</option>
                  <option value="!=">!=</option>
                  <option value=">">&gt;</option>
                  <option value="<">&lt;</option>
                  <option value=">=">&gt;=</option>
                  <option value="<=">&lt;=</option>
                </select>
                <input
                  type="number"
                  value={httpCodeValue}
                  onChange={(e) => setHttpCodeValue(e.target.value)}
                  placeholder="状态码"
                  className={styles.httpCodeInput}
                  disabled={!httpCodeOperator}
                  min="100"
                  max="599"
                />
              </div>
            </div>
          </>
        )}

        <div className={styles.filterGroup}>
          <label>限制数量:</label>
          <select
            value={limit}
            onChange={(e) => setLimit(Number(e.target.value))}
            className={styles.select}
          >
            <option value={50}>50</option>
            <option value={100}>100</option>
            <option value={200}>200</option>
            <option value={500}>500</option>
          </select>
        </div>

        <button onClick={handleSearch} className={styles.searchButton}>
          查询
        </button>
      </div>

      {loading && <div className={styles.loading}>加载中...</div>}

      {!loading && logs.length === 0 && (
        <div className={styles.emptyState}>
          <h3>暂无日志记录</h3>
          <p>当前时间范围内没有找到审计日志</p>
          <div className={styles.emptyHint}>
            <p>提示：</p>
            <ul>
              <li>审计日志会在系统运行时自动记录</li>
              <li>尝试调整时间范围或切换日志类型</li>
              <li>执行一些操作（如权限授予、资源访问）后会产生日志</li>
            </ul>
          </div>
        </div>
      )}

      {!loading && logs.length > 0 && (
        <div className={styles.statsBar}>
          <span>共找到 <strong>{logs.length}</strong> 条记录</span>
          {logType === 'denied' && (
            <span className={styles.warningBadge}>
              安全警告：发现被拒绝的访问记录
            </span>
          )}
        </div>
      )}

      {!loading && logs.length > 0 && (
        <div className={styles.tableContainer}>
          <table className={styles.table}>
            <thead>
              <tr>
                {visibleColumns.time && <th>时间</th>}
                {visibleColumns.username && <th>用户名</th>}
                {visibleColumns.path && <th>请求路径</th>}
                {visibleColumns.resourceType && <th>资源类型</th>}
                {visibleColumns.action && <th>操作</th>}
                {visibleColumns.httpCode && <th>HTTP状态码</th>}
                {visibleColumns.result && <th>结果</th>}
                {logType === 'denied' && visibleColumns.denyReason && <th>拒绝原因</th>}
                {visibleColumns.ipAddress && <th>IP地址</th>}
                {visibleColumns.duration && <th>耗时(ms)</th>}
                <th>操作</th>
              </tr>
            </thead>
            <tbody>
              {logs.map((log, index) => (
                <React.Fragment key={index}>
                  <tr>
                    {visibleColumns.time && (
                      <td className={styles.timeCell}>
                        {(() => {
                          try {
                            let date: Date;
                            // 如果时间字符串已经包含时区信息，直接解析
                            if (log.accessed_at.includes('+') || log.accessed_at.includes('Z')) {
                              date = new Date(log.accessed_at);
                            } else {
                              // 否则添加Z表示UTC时间
                              date = new Date(log.accessed_at + 'Z');
                            }
                            
                            if (timeZone === 'utc') {
                              // 显示UTC时间（数据库存储的是本地时间，需要减去8小时）
                              const utcDate = new Date(date.getTime() - 8 * 60 * 60 * 1000);
                              const utcStr = utcDate.toISOString().replace('T', ' ').substring(0, 19);
                              return `${utcStr} UTC`;
                            } else {
                              // 显示本地时间（数据库存储的就是本地时间，直接显示）
                              const year = date.getFullYear();
                              const month = String(date.getMonth() + 1).padStart(2, '0');
                              const day = String(date.getDate()).padStart(2, '0');
                              const hours = String(date.getHours()).padStart(2, '0');
                              const minutes = String(date.getMinutes()).padStart(2, '0');
                              const seconds = String(date.getSeconds()).padStart(2, '0');
                              return `${year}/${month}/${day} ${hours}:${minutes}:${seconds}`;
                            }
                          } catch (e) {
                            return log.accessed_at;
                          }
                        })()}
                      </td>
                    )}
                    {visibleColumns.username && <td>{log.user_id}</td>}
                    {visibleColumns.path && (
                      <td className={styles.pathCell}>{log.request_path || '-'}</td>
                    )}
                    {visibleColumns.resourceType && (
                      <td className={styles.resourceCell}>{log.resource_type}</td>
                    )}
                    {visibleColumns.action && (
                      <td className={styles.actionCell}>{log.action}</td>
                    )}
                    {visibleColumns.httpCode && (
                      <td className={styles.httpCodeCell}>
                        <span className={log.http_code >= 200 && log.http_code < 300 ? styles.httpSuccess : styles.httpError}>
                          {log.http_code || '-'}
                        </span>
                      </td>
                    )}
                    {visibleColumns.result && (
                      <td>
                        <span className={log.is_allowed ? styles.statusAllowed : styles.statusDenied}>
                          {log.is_allowed ? '允许' : '拒绝'}
                        </span>
                      </td>
                    )}
                    {logType === 'denied' && visibleColumns.denyReason && (
                      <td className={styles.reasonCell}>{log.deny_reason || '-'}</td>
                    )}
                    {visibleColumns.ipAddress && (
                      <td className={styles.ipCell}>{log.ip_address || '-'}</td>
                    )}
                    {visibleColumns.duration && (
                      <td className={styles.durationCell}>{log.duration_ms || '-'}</td>
                    )}
                    <td>
                      <button
                        onClick={() => setExpandedLog(expandedLog === index ? null : index)}
                        className={styles.detailButton}
                      >
                        {expandedLog === index ? '收起' : '详情'}
                      </button>
                    </td>
                  </tr>
                  {/* 展开的详情行 */}
                  {expandedLog === index && (
                    <tr className={styles.detailRow}>
                      <td colSpan={
                        Object.values(visibleColumns).filter(Boolean).length + 
                        (logType === 'denied' && visibleColumns.denyReason ? 1 : 0) + 1
                      }>
                        <div className={styles.logDetail}>
                          <h4>请求详情</h4>
                          {log.request_body && includeBody && (
                            <div className={styles.detailSection}>
                              <strong>请求体:</strong>
                              <pre>{log.request_body}</pre>
                            </div>
                          )}
                          {log.request_headers && includeHeaders && (
                            <div className={styles.detailSection}>
                              <strong>请求头:</strong>
                              <pre>{typeof log.request_headers === 'string' 
                                ? JSON.stringify(JSON.parse(log.request_headers), null, 2)
                                : JSON.stringify(log.request_headers, null, 2)
                              }</pre>
                            </div>
                          )}
                          {log.user_agent && (
                            <div className={styles.detailSection}>
                              <strong>User Agent:</strong>
                              <p>{log.user_agent}</p>
                            </div>
                          )}
                        </div>
                      </td>
                    </tr>
                  )}
                </React.Fragment>
              ))}
            </tbody>
          </table>
        </div>
      )}

          <div className={styles.info}>
            <h3>审计日志说明</h3>
            <ul>
              <li><strong>访问历史</strong>: 记录所有资源访问请求，包括成功和失败的访问</li>
              <li><strong>被拒绝的访问</strong>: 仅显示被权限系统拒绝的访问请求，用于安全审计</li>
              <li><strong>时间范围</strong>: 默认查询最近7天的日志，可自定义时间范围</li>
              <li><strong>导出功能</strong>: 支持将日志导出为JSON格式，便于进一步分析</li>
              <li><strong>请求体/请求头</strong>: 启用后可在详情中查看完整的请求信息</li>
            </ul>
          </div>
        </>
      )}

      {/* 确认对话框 */}
      <ConfirmDialog
        isOpen={confirmDialog.isOpen}
        title={confirmDialog.title}
        message={confirmDialog.message}
        type={confirmDialog.type}
        confirmText="确认"
        cancelText="取消"
        onConfirm={confirmDialog.onConfirm}
        onCancel={() => setConfirmDialog({ ...confirmDialog, isOpen: false })}
      />
    </div>
  );
};

export default AuditLog;

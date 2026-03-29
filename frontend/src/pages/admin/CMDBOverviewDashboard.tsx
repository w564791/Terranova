import React, { useState, useEffect, useMemo } from 'react';
import { Spin, Pagination } from 'antd';
import { cmdbService, CMDBOverview, CMDBRecentSync, CMDBSearchAnalytics } from '../../services/cmdb';
import styles from './CMDBOverviewDashboard.module.css';

const CMDBOverviewDashboard: React.FC = () => {
  const [data, setData] = useState<CMDBOverview | null>(null);
  const [loading, setLoading] = useState(true);

  // 同步历史分页状态
  const [syncs, setSyncs] = useState<CMDBRecentSync[]>([]);
  const [syncTotal, setSyncTotal] = useState(0);
  const [syncPage, setSyncPage] = useState(1);
  const [syncLoading, setSyncLoading] = useState(false);
  const syncSize = 10;

  // 搜索分析状态
  const [searchAnalytics, setSearchAnalytics] = useState<CMDBSearchAnalytics | null>(null);
  const [searchAnalyticsLoading, setSearchAnalyticsLoading] = useState(false);
  const [searchPeriod, setSearchPeriod] = useState<'24h' | '7d' | '30d'>('7d');
  const [searchSource, setSearchSource] = useState<'all' | 'manual' | 'agent'>('all');

  useEffect(() => {
    loadData();
  }, []);

  useEffect(() => {
    loadSyncHistory(syncPage);
  }, [syncPage]);

  const loadData = async () => {
    try {
      setLoading(true);
      const overview = await cmdbService.getCMDBOverview();
      setData(overview);
    } catch (err) {
      console.error('Failed to load CMDB overview:', err);
    } finally {
      setLoading(false);
    }
  };

  const loadSyncHistory = async (page: number) => {
    try {
      setSyncLoading(true);
      const result = await cmdbService.getSyncHistory(page, syncSize);
      setSyncs(result.syncs);
      setSyncTotal(result.total);
    } catch (err) {
      console.error('Failed to load sync history:', err);
    } finally {
      setSyncLoading(false);
    }
  };

  const loadSearchAnalytics = async (period: string, source: string) => {
    try {
      setSearchAnalyticsLoading(true);
      const data = await cmdbService.getSearchAnalytics(period, source);
      setSearchAnalytics(data);
    } catch (err) {
      console.error('Failed to load search analytics:', err);
    } finally {
      setSearchAnalyticsLoading(false);
    }
  };

  useEffect(() => {
    loadSearchAnalytics(searchPeriod, searchSource);
  }, [searchPeriod, searchSource]);

  // 词云数据：随机打乱 + 字体映射
  const wordCloudItems = useMemo(() => {
    if (!searchAnalytics?.top_queries?.length) return [];
    const queries = [...searchAnalytics.top_queries];
    const maxCount = Math.max(...queries.map(q => q.count));
    const minCount = Math.min(...queries.map(q => q.count));
    const colors = ['#1677ff','#597ef7','#85a5ff','#9333ea','#f759ab','#fa8c16','#52c41a','#13c2c2','#ff4d4f','#faad14'];
    const rotations = [-5, 0, 0, 0, 5];
    // 随机打乱
    for (let i = queries.length - 1; i > 0; i--) {
      const j = Math.floor(Math.random() * (i + 1));
      [queries[i], queries[j]] = [queries[j], queries[i]];
    }
    return queries.map((q, i) => ({
      text: q.query,
      count: q.count,
      avgResults: q.avg_results,
      fontSize: maxCount === minCount ? 20 : 14 + (q.count - minCount) / (maxCount - minCount) * 34,
      color: colors[i % colors.length],
      rotation: rotations[i % rotations.length],
    }));
  }, [searchAnalytics?.top_queries]);

  if (loading) return <Spin style={{ display: 'block', margin: '80px auto' }} />;
  if (!data) return <div className={styles.emptyHint}>暂无数据</div>;

  const coverageColor = (pct: number) =>
    pct >= 90 ? '#52c41a' : pct >= 60 ? '#fa8c16' : '#ff4d4f';

  const formatTime = (ts?: string) => {
    if (!ts) return '-';
    const d = new Date(ts);
    return `${d.getMonth() + 1}/${d.getDate()} ${d.getHours().toString().padStart(2, '0')}:${d.getMinutes().toString().padStart(2, '0')}`;
  };

  return (
    <div className={styles.container}>
      {/* Row 1: Data Sources */}
      <div className={`${styles.row} ${styles.row3}`}>
        <div className={styles.card}>
          <div className={styles.cardLabel}>Workspaces</div>
          <div className={styles.cardValue}>{data.sources.workspace_count}</div>
          <div className={styles.cardHint}>有资源的 workspace 数量</div>
        </div>
        <div className={styles.card}>
          <div className={styles.cardLabel}>外部数据源</div>
          <div className={styles.cardValue}>{data.sources.external_source_count}</div>
          <div className={styles.cardHint}>
            {data.sources.external_source_healthy} 正常
            {data.sources.external_source_error > 0 && (
              <span style={{ color: '#ff4d4f' }}> / {data.sources.external_source_error} 异常</span>
            )}
          </div>
        </div>
        <div className={styles.card}>
          <div className={styles.cardLabel}>资源总数</div>
          <div className={styles.cardValue}>{data.resources.total}</div>
          <div className={styles.cardHint}>
            Workspace: {data.resources.from_workspace} / 外部: {data.resources.from_external}
          </div>
        </div>
      </div>

      {/* Row 2: Embedding + Summary Coverage */}
      <div className={`${styles.row} ${styles.row2}`}>
        <div className={styles.card}>
          <div className={styles.cardLabel}>Embedding 覆盖率</div>
          <div className={styles.cardValue}>{data.embedding.coverage_pct.toFixed(1)}%</div>
          <div className={styles.cardHint}>
            {data.embedding.completed}/{data.embedding.total}
          </div>
          <div className={styles.coverageBar}>
            <div
              className={styles.coverageFill}
              style={{
                width: `${data.embedding.coverage_pct}%`,
                background: coverageColor(data.embedding.coverage_pct),
              }}
            />
          </div>
        </div>
        <div className={styles.card}>
          <div className={styles.cardLabel}>Summary 覆盖率</div>
          <div className={styles.cardValue}>{data.summary.coverage_pct.toFixed(1)}%</div>
          <div className={styles.cardHint}>
            {data.summary.completed}/{data.summary.total}
          </div>
          <div className={styles.coverageBar}>
            <div
              className={styles.coverageFill}
              style={{
                width: `${data.summary.coverage_pct}%`,
                background: coverageColor(data.summary.coverage_pct),
              }}
            />
          </div>
        </div>
      </div>

      {/* Row: Search Analytics */}
      <div className={styles.card}>
        <div className={styles.searchAnalyticsHeader}>
          <div className={styles.cardLabel}>搜索召回质量</div>
          <div className={styles.filterButtons}>
            <div className={styles.periodButtons}>
              {(['24h', '7d', '30d'] as const).map(p => (
                <button
                  key={p}
                  className={`${styles.periodButton} ${searchPeriod === p ? styles.periodButtonActive : ''}`}
                  onClick={() => setSearchPeriod(p)}
                >
                  {p}
                </button>
              ))}
            </div>
            <div className={styles.periodButtons}>
              {([['all', '全部'], ['manual', '用户'], ['agent', 'Agent']] as const).map(([val, label]) => (
                <button
                  key={val}
                  className={`${styles.periodButton} ${searchSource === val ? styles.periodButtonActive : ''}`}
                  onClick={() => setSearchSource(val)}
                >
                  {label}
                </button>
              ))}
            </div>
          </div>
        </div>

        {searchAnalyticsLoading ? (
          <Spin style={{ display: 'block', margin: '24px auto' }} />
        ) : !searchAnalytics || searchAnalytics.usage.total_searches === 0 ? (
          <div className={styles.emptyHint}>暂无搜索记录</div>
        ) : (
          <>
            {/* 指标卡片行 */}
            <div className={styles.searchMetricsRow}>
              <div className={styles.searchMetric}>
                <div className={styles.searchMetricValue}>{searchAnalytics.usage.total_searches}</div>
                <div className={styles.searchMetricLabel}>搜索次数</div>
              </div>
              <div className={styles.searchMetric}>
                <div className={styles.searchMetricValue} style={{ color: searchAnalytics.usage.zero_result_rate > 20 ? '#ff4d4f' : undefined }}>
                  {searchAnalytics.usage.zero_result_rate.toFixed(1)}%
                </div>
                <div className={styles.searchMetricLabel}>零结果率</div>
              </div>
              <div className={styles.searchMetric}>
                <div className={styles.searchMetricValue}>{searchAnalytics.usage.avg_result_count.toFixed(1)}</div>
                <div className={styles.searchMetricLabel}>平均结果数</div>
              </div>
              <div className={styles.searchMetric}>
                <div className={styles.searchMetricValue}>{searchAnalytics.quality.avg_duration_ms.toFixed(0)}ms</div>
                <div className={styles.searchMetricLabel}>平均耗时</div>
              </div>
              <div className={styles.searchMetric}>
                <div className={styles.searchMetricValue} style={{ color: searchAnalytics.quality.fallback_rate > 30 ? '#ff4d4f' : undefined }}>
                  {searchAnalytics.quality.fallback_rate.toFixed(1)}%
                </div>
                <div className={styles.searchMetricLabel}>Fallback 率</div>
              </div>
            </div>

            {/* 下半部分：方法分布 + 零结果 | 词云 */}
            <div className={styles.searchDetailRow}>
              <div className={styles.searchDetailLeft}>
                {/* 搜索方式分布 */}
                <div className={styles.searchSubSection}>
                  <div className={styles.searchSubTitle}>搜索方式分布</div>
                  {Object.entries(searchAnalytics.quality.method_distribution).map(([method, count]) => {
                    const total = searchAnalytics.usage.total_searches;
                    const pct = total > 0 ? (count as number) / total * 100 : 0;
                    const barColors: Record<string, string> = { hybrid: '#1677ff', vector: '#52c41a', keyword: '#fa8c16' };
                    return (
                      <div key={method} className={styles.methodBarRow}>
                        <span className={styles.methodBarLabel}>{method}</span>
                        <div className={styles.methodBarTrack}>
                          <div className={styles.methodBarFill} style={{ width: `${pct}%`, background: barColors[method] || '#999' }} />
                        </div>
                        <span className={styles.methodBarCount}>{count as number}</span>
                      </div>
                    );
                  })}
                </div>

                {/* 零结果查询 */}
                {searchAnalytics.zero_result_queries.length > 0 && (
                  <div className={styles.searchSubSection}>
                    <div className={styles.searchSubTitle}>零结果查询 Top 5</div>
                    {searchAnalytics.zero_result_queries.slice(0, 5).map((q, i) => (
                      <div key={i} className={styles.zeroResultItem}>
                        <span className={styles.zeroResultQuery}>{q.query}</span>
                        <span className={styles.zeroResultCount}>{q.count}次</span>
                      </div>
                    ))}
                  </div>
                )}
              </div>

              {/* 词云 */}
              <div className={styles.searchDetailRight}>
                <div className={styles.searchSubTitle}>热门查询</div>
                <div className={styles.wordCloud}>
                  {wordCloudItems.map((item, i) => (
                    <span
                      key={i}
                      className={styles.wordCloudItem}
                      style={{
                        fontSize: `${item.fontSize}px`,
                        color: item.color,
                        transform: `rotate(${item.rotation}deg)`,
                      }}
                      title={`${item.text} (${item.count}次搜索, 平均${item.avgResults.toFixed(1)}条结果)`}
                    >
                      {item.text}
                    </span>
                  ))}
                </div>
              </div>
            </div>
          </>
        )}
      </div>

      {/* Row 3: Task Queues + Resource Type Top 10 */}
      <div className={`${styles.row} ${styles.rowQueueType}`}>
        <div className={styles.card}>
          <div className={styles.cardLabel}>任务队列</div>
          <div className={styles.queueNote}>Workspace 的 Summary 在同步时即时生成，无独立队列</div>
          {/* Embedding 队列 (workspace) */}
          <div className={styles.queueSection}>Embedding 队列 (Workspace)</div>
          <div className={styles.queueItem}>
            <span className={styles.queueLabel}>待处理</span>
            <span className={styles.queueValue}>{data.queue.embedding_pending}</span>
          </div>
          <div className={styles.queueItem}>
            <span className={styles.queueLabel}>处理中</span>
            <span className={styles.queueValue}>{data.queue.embedding_processing}</span>
          </div>
          <div className={styles.queueItem}>
            <span className={styles.queueLabel}>失败</span>
            <span className={styles.queueValue} style={{ color: data.queue.embedding_failed > 0 ? '#ff4d4f' : undefined }}>
              {data.queue.embedding_failed}
            </span>
          </div>
          {/* Summary 队列 (外部源) */}
          <div className={styles.queueSection}>Summary 队列 (外部源)</div>
          <div className={styles.queueItem}>
            <span className={styles.queueLabel}>待处理</span>
            <span className={styles.queueValue}>{data.queue.summary_pending}</span>
          </div>
          <div className={styles.queueItem}>
            <span className={styles.queueLabel}>处理中</span>
            <span className={styles.queueValue}>{data.queue.summary_processing}</span>
          </div>
          <div className={styles.queueItem}>
            <span className={styles.queueLabel}>失败</span>
            <span className={styles.queueValue} style={{ color: data.queue.summary_failed > 0 ? '#ff4d4f' : undefined }}>
              {data.queue.summary_failed}
            </span>
          </div>
          {/* Embedding 队列 (外部源) */}
          <div className={styles.queueSection}>Embedding 队列 (外部源)</div>
          <div className={styles.queueItem}>
            <span className={styles.queueLabel}>待处理</span>
            <span className={styles.queueValue}>{data.queue.ext_embedding_pending}</span>
          </div>
          <div className={styles.queueItem}>
            <span className={styles.queueLabel}>处理中</span>
            <span className={styles.queueValue}>{data.queue.ext_embedding_processing}</span>
          </div>
          <div className={styles.queueItem}>
            <span className={styles.queueLabel}>失败</span>
            <span className={styles.queueValue} style={{ color: data.queue.ext_embedding_failed > 0 ? '#ff4d4f' : undefined }}>
              {data.queue.ext_embedding_failed}
            </span>
          </div>
        </div>

        <div className={styles.card}>
          <div className={styles.cardLabel}>资源类型分布</div>
          {(() => {
            const items = data.resources.type_top10;
            if (!items?.length) return null;
            const maxVal = items[0].count;
            const barColors = ['#1677ff','#597ef7','#85a5ff','#adc6ff','#bae7ff','#87e8de','#b7eb8f','#ffe58f','#ffd591','#ffbb96'];
            const top10Total = items.reduce((sum, t) => sum + t.count, 0);
            const otherCount = data.resources.total - top10Total;
            return (
              <>
                {items.map((t, i) => (
                  <div key={i} className={styles.typeBarRow}>
                    <span className={styles.typeBarLabel}>{t.resource_type}</span>
                    <div className={styles.typeBarTrack}>
                      <div
                        className={styles.typeBarFill}
                        style={{ width: `${(t.count / maxVal * 80).toFixed(1)}%`, background: barColors[i % barColors.length] }}
                      />
                    </div>
                    <span className={styles.typeBarCount}>{t.count}</span>
                  </div>
                ))}
                {/* 来源占比 */}
                <div className={styles.typeBarFooter}>
                  {otherCount > 0 && (
                    <div className={styles.typeBarFooterText}>其他类型: {otherCount} 个资源</div>
                  )}
                  <div className={styles.sourceBarRow}>
                    <span className={styles.sourceBarLabel}>来源占比</span>
                    <div className={styles.sourceBar}>
                      <div
                        className={styles.sourceBarFill}
                        style={{
                          width: data.resources.total > 0 ? `${(data.resources.from_workspace / data.resources.total * 100).toFixed(1)}%` : '0%',
                          background: '#1677ff',
                        }}
                      />
                      <div
                        className={styles.sourceBarFill}
                        style={{
                          width: data.resources.total > 0 ? `${(data.resources.from_external / data.resources.total * 100).toFixed(1)}%` : '0%',
                          background: '#fa8c16',
                        }}
                      />
                    </div>
                    <span className={styles.sourceBarSpacer} />
                  </div>
                  <div className={styles.sourceBarLegend}>
                    <span><span className={styles.legendDot} style={{ background: '#1677ff' }} />Workspace {data.resources.from_workspace}</span>
                    <span><span className={styles.legendDot} style={{ background: '#fa8c16' }} />外部源 {data.resources.from_external}</span>
                  </div>
                </div>
              </>
            );
          })()}
        </div>
      </div>

      {/* Row 4: Recent Sync Activity with Pagination */}
      <div className={styles.card}>
        <div className={styles.cardLabel}>最近同步活动</div>
        {syncLoading ? (
          <Spin style={{ display: 'block', margin: '24px auto' }} />
        ) : syncs.length === 0 ? (
          <div className={styles.emptyHint}>暂无同步记录</div>
        ) : (
          <>
            <table className={styles.syncTable}>
              <thead>
                <tr>
                  <th>来源</th>
                  <th>名称</th>
                  <th>触发方式</th>
                  <th>状态</th>
                  <th>完成时间</th>
                  <th>同步</th>
                  <th>新增</th>
                  <th>更新</th>
                  <th>删除</th>
                </tr>
              </thead>
              <tbody>
                {syncs.map((sync, i) => (
                  <tr key={i}>
                    <td>
                      <span
                        className={`${styles.badge} ${sync.source_type === 'workspace' ? styles.badgeWorkspace : styles.badgeExternal}`}
                      >
                        {sync.source_type === 'workspace' ? 'Workspace' : '外部'}
                      </span>
                    </td>
                    <td>{sync.source_name || sync.source_id}</td>
                    <td>{sync.triggered_by || '-'}</td>
                    <td>
                      <span
                        className={`${styles.badge} ${
                          sync.status === 'success'
                            ? styles.badgeSuccess
                            : sync.status === 'failed'
                              ? styles.badgeFailed
                              : styles.badgeRunning
                        }`}
                      >
                        {sync.status}
                      </span>
                    </td>
                    <td>{formatTime(sync.completed_at)}</td>
                    <td>{sync.resources_synced}</td>
                    <td>{sync.resources_added}</td>
                    <td>{sync.resources_updated}</td>
                    <td>{sync.resources_deleted}</td>
                  </tr>
                ))}
              </tbody>
            </table>
            {syncTotal > syncSize && (
              <div className={styles.pagination}>
                <Pagination
                  current={syncPage}
                  total={syncTotal}
                  pageSize={syncSize}
                  size="small"
                  showTotal={(total) => `共 ${total} 条`}
                  onChange={(page) => setSyncPage(page)}
                />
              </div>
            )}
          </>
        )}
      </div>
    </div>
  );
};

export default CMDBOverviewDashboard;

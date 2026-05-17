import React, { useState, useEffect } from 'react';
import { Spin, Empty, Table, Tooltip, Drawer, Tag, Button, message } from 'antd';
import { QuestionCircleOutlined, ReloadOutlined } from '@ant-design/icons';
import type { ColumnsType } from 'antd/es/table';
import {
  getSummaryAssessmentOverview,
  getIssueResources,
  regenerateSummaries,
} from '../../services/summaryAssessment';
import type {
  SummaryAssessmentOverview,
  ResourceTypeStats,
  IssueResource,
} from '../../services/summaryAssessment';
import styles from './SkillQualityDashboard.module.css';

const ColTitle: React.FC<{ title: string; tip: string }> = ({ title, tip }) => (
  <span>
    {title}{' '}
    <Tooltip title={tip}>
      <QuestionCircleOutlined style={{ color: '#8c8c8c', fontSize: 12, cursor: 'help' }} />
    </Tooltip>
  </span>
);

interface Props {
  days: number;
}

const SummaryQualityTab: React.FC<Props> = ({ days }) => {
  const [data, setData] = useState<SummaryAssessmentOverview | null>(null);
  const [loading, setLoading] = useState(true);
  const [drawerOpen, setDrawerOpen] = useState(false);
  const [drawerTitle, setDrawerTitle] = useState('');
  const [drawerLoading, setDrawerLoading] = useState(false);
  const [issueResources, setIssueResources] = useState<IssueResource[]>([]);
  const [selectedRowKeys, setSelectedRowKeys] = useState<React.Key[]>([]);
  const [regenerating, setRegenerating] = useState(false);

  useEffect(() => {
    loadData();
  }, [days]);

  const loadData = async () => {
    setLoading(true);
    try {
      const result = await getSummaryAssessmentOverview(days);
      setData(result);
    } catch (e) {
      console.error('Failed to load summary assessment data:', e);
    } finally {
      setLoading(false);
    }
  };

  const openIssueDrawer = async (issueType: string, title: string) => {
    setDrawerTitle(title);
    setDrawerOpen(true);
    setDrawerLoading(true);
    setSelectedRowKeys([]);
    try {
      const resources = await getIssueResources(issueType, days);
      setIssueResources(resources || []);
    } catch (e) {
      console.error('Failed to load issue resources:', e);
      setIssueResources([]);
    } finally {
      setDrawerLoading(false);
    }
  };

  const handleRegenerate = async (ids?: number[]) => {
    const resourceIds = ids || selectedRowKeys.map(k => Number(k));
    if (resourceIds.length === 0) {
      message.warning('请先选择要重新生成的资源');
      return;
    }
    setRegenerating(true);
    try {
      const result = await regenerateSummaries(resourceIds);
      message.success(`已提交 ${result.resources_affected} 个资源的摘要重新生成任务`);
      setSelectedRowKeys([]);
      setDrawerOpen(false);
      loadData();
    } catch (e) {
      message.error('提交失败: ' + String(e));
    } finally {
      setRegenerating(false);
    }
  };

  if (!data && loading) {
    return <div style={{ textAlign: 'center', padding: '80px 0' }}><Spin size="large" /></div>;
  }
  if (!data) {
    return <Empty description="暂无摘要评估数据" />;
  }

  const { summary_coverage, assessment, security_tag_stats, issue_distribution, daily_trend, by_resource_type } = data;

  const trendData = (daily_trend || []).slice(-14);
  const barHeight = 100;
  const maxBar = Math.max(...trendData.map(d => d.total), 1);

  const rtColumns: ColumnsType<ResourceTypeStats> = [
    { title: '资源类型', dataIndex: 'resource_type', key: 'resource_type' },
    { title: '数量', dataIndex: 'count', key: 'count', sorter: (a, b) => a.count - b.count },
    {
      title: <ColTitle title="通过率" tip="L1 文本规则检测的通过率" />,
      dataIndex: 'pass_rate', key: 'pass_rate',
      render: (v: number) => <span style={{ color: v >= 80 ? '#52c41a' : v >= 50 ? '#faad14' : '#ff4d4f' }}>{v.toFixed(1)}%</span>,
      sorter: (a, b) => a.pass_rate - b.pass_rate,
    },
    {
      title: <ColTitle title="均分" tip="L1 评估平均分 (0-100)" />,
      dataIndex: 'avg_score', key: 'avg_score',
      render: (v: number) => v.toFixed(1),
      sorter: (a, b) => a.avg_score - b.avg_score,
    },
  ];

  const formatLabel = (type: string) => {
    const map: Record<string, string> = {
      markdown_syntax: 'Markdown 语法',
      over_length: '超长',
      empty_summary: '空摘要',
      first_line_format: '首行格式',
    };
    return map[type] || type;
  };

  return (
    <Spin spinning={loading}>
      {/* KPI Row */}
      <div className={styles.kpiGrid}>
        <div className={styles.statCard}>
          <div className={styles.statLabel}>
            <ColTitle title="摘要覆盖率" tip="有 AI 摘要的资源占全部 managed 资源的比例" />
          </div>
          <div className={styles.statValue} style={{ color: summary_coverage.coverage_pct >= 80 ? '#52c41a' : '#faad14' }}>
            {summary_coverage.coverage_pct.toFixed(1)}%
          </div>
          <div className={styles.statSub}>{summary_coverage.with_summary} / {summary_coverage.total_resources}</div>
        </div>

        <div className={styles.statCard}>
          <div className={styles.statLabel}>
            <ColTitle title="L1 通过率" tip="文本规则检测（格式/安全标注/幻觉初筛）的通过率" />
          </div>
          <div className={styles.statValue} style={{ color: assessment.l1_pass_rate >= 80 ? '#52c41a' : assessment.l1_pass_rate >= 50 ? '#faad14' : '#ff4d4f' }}>
            {assessment.l1_pass_rate.toFixed(1)}%
          </div>
          <div className={styles.statSub}>Pass {assessment.l1_pass} / Fail {assessment.l1_fail} / Warn {assessment.l1_warn}</div>
        </div>

        <div className={styles.statCard}>
          <div className={styles.statLabel}>
            <ColTitle title="L2 均分" tip="LLM Prompt 遵从度评估的平均分（抽样）" />
          </div>
          <div className={styles.statValue} style={{ color: assessment.l2_avg_score >= 80 ? '#52c41a' : '#faad14' }}>
            {assessment.l2_avg_score > 0 ? assessment.l2_avg_score.toFixed(1) : '-'}
          </div>
          <div className={styles.statSub}>Prompt 遵从度</div>
        </div>

        <div className={styles.statCard}>
          <div className={styles.statLabel}>
            <ColTitle title="L3 均分" tip="LLM 内容质量评估的平均分（抽样）" />
          </div>
          <div className={styles.statValue} style={{ color: assessment.l3_avg_score >= 80 ? '#52c41a' : '#faad14' }}>
            {assessment.l3_avg_score > 0 ? assessment.l3_avg_score.toFixed(1) : '-'}
          </div>
          <div className={styles.statSub}>内容质量</div>
        </div>

        <div className={styles.statCard}>
          <div className={styles.statLabel}>
            <ColTitle title="安全标注命中率" tip="应标注安全信息（公网暴露/删除保护/无备份）的命中比例" />
          </div>
          <div className={styles.statValue} style={{ color: security_tag_stats.hit_rate >= 90 ? '#52c41a' : '#ff4d4f' }}>
            {security_tag_stats.hit_rate.toFixed(1)}%
          </div>
          <div className={styles.statSub}>{security_tag_stats.total_hit} / {security_tag_stats.total_expected}</div>
        </div>
      </div>

      {/* Two-column: Trend + Issues */}
      <div className={styles.twoCol}>
        <div className={styles.sectionCard}>
          <div className={styles.sectionTitle}>
            <ColTitle title="每日 L1 通过率趋势" tip="按天的文本规则检测结果" />
          </div>
          {trendData.length > 0 ? (
            <>
              <div className={styles.barChart}>
                {trendData.map((d) => {
                  const passH = (d.pass / maxBar) * barHeight;
                  const failH = (d.fail / maxBar) * barHeight;
                  const warnH = (d.warn / maxBar) * barHeight;
                  return (
                    <Tooltip key={d.date} title={`${d.date}: Pass ${d.pass}, Fail ${d.fail}, Warn ${d.warn}`}>
                      <div className={styles.barGroup}>
                        <div className={styles.bar} style={{ height: warnH, background: '#fa8c16' }} />
                        <div className={styles.bar} style={{ height: failH, background: '#ff4d4f' }} />
                        <div className={styles.bar} style={{ height: passH, background: '#52c41a' }} />
                      </div>
                    </Tooltip>
                  );
                })}
              </div>
              <div className={styles.barXAxis}>
                {trendData.map((d) => (
                  <span key={d.date} className={styles.barXLabel}>{d.date.slice(5)}</span>
                ))}
              </div>
            </>
          ) : (
            <div className={styles.emptyState}>暂无趋势数据</div>
          )}
        </div>

        <div className={styles.sectionCard}>
          <div className={styles.sectionTitle}>
            <ColTitle title="问题分布" tip="各类检测问题的数量分布" />
          </div>
          {((issue_distribution.format_violations || []).length > 0 ||
            issue_distribution.hallucination_suspects > 0 ||
            issue_distribution.security_tag_misses > 0) ? (
            <>
              {(issue_distribution.format_violations || []).map((v) => (
                <div key={v.type} className={styles.hbarRow} style={{ cursor: 'pointer' }} onClick={() => openIssueDrawer(v.type, formatLabel(v.type))}>
                  <span className={styles.hbarLabel} style={{ color: '#1677ff' }}>{formatLabel(v.type)}</span>
                  <div className={styles.hbarTrack}>
                    <div className={styles.hbarFill} style={{ width: `${Math.min(100, v.count * 10)}%`, background: '#ff4d4f' }} />
                  </div>
                  <span className={styles.hbarCount}>{v.count}</span>
                </div>
              ))}
              {issue_distribution.hallucination_suspects > 0 && (
                <div className={styles.hbarRow} style={{ cursor: 'pointer' }} onClick={() => openIssueDrawer('hallucination', '疑似幻觉')}>
                  <span className={styles.hbarLabel} style={{ color: '#1677ff' }}>疑似幻觉</span>
                  <div className={styles.hbarTrack}>
                    <div className={styles.hbarFill} style={{ width: `${Math.min(100, issue_distribution.hallucination_suspects * 10)}%`, background: '#fa8c16' }} />
                  </div>
                  <span className={styles.hbarCount}>{issue_distribution.hallucination_suspects}</span>
                </div>
              )}
              {issue_distribution.security_tag_misses > 0 && (
                <div className={styles.hbarRow} style={{ cursor: 'pointer' }} onClick={() => openIssueDrawer('security_miss', '安全标注缺失')}>
                  <span className={styles.hbarLabel} style={{ color: '#1677ff' }}>安全标注缺失</span>
                  <div className={styles.hbarTrack}>
                    <div className={styles.hbarFill} style={{ width: `${Math.min(100, issue_distribution.security_tag_misses * 10)}%`, background: '#ff4d4f' }} />
                  </div>
                  <span className={styles.hbarCount}>{issue_distribution.security_tag_misses}</span>
                </div>
              )}
              {(security_tag_stats.misses_by_rule || []).map((m) => (
                <div key={m.rule} className={styles.hbarRow} style={{ cursor: 'pointer' }} onClick={() => openIssueDrawer(`security_miss:${m.rule}`, `缺失: ${m.rule}`)}>
                  <span className={styles.hbarLabel} style={{ color: '#1677ff' }}>缺失: {m.rule}</span>
                  <div className={styles.hbarTrack}>
                    <div className={styles.hbarFill} style={{ width: `${Math.min(100, m.miss_count * 10)}%`, background: '#cf1322' }} />
                  </div>
                  <span className={styles.hbarCount}>{m.miss_count}</span>
                </div>
              ))}
            </>
          ) : (
            <div className={styles.emptyState}>无问题记录</div>
          )}
        </div>
      </div>

      {/* Resource Type Table */}
      <div className={`${styles.sectionCard} ${styles.tableCard}`}>
        <div className={styles.sectionTitle}>
          <ColTitle title="各资源类型质量" tip="按资源类型分组的 L1 检测统计" />
        </div>
        <Table
          columns={rtColumns}
          dataSource={by_resource_type || []}
          pagination={false}
          size="small"
          rowKey="resource_type"
        />
      </div>
      <Drawer
        title={`问题资源: ${drawerTitle}`}
        open={drawerOpen}
        onClose={() => { setDrawerOpen(false); setSelectedRowKeys([]); }}
        width={720}
        extra={
          <div style={{ display: 'flex', gap: 8 }}>
            <Button
              icon={<ReloadOutlined />}
              onClick={() => handleRegenerate()}
              disabled={selectedRowKeys.length === 0}
              loading={regenerating}
            >
              重跑选中 ({selectedRowKeys.length})
            </Button>
            <Button
              type="primary"
              icon={<ReloadOutlined />}
              onClick={() => handleRegenerate(issueResources.map(r => r.resource_id))}
              disabled={issueResources.length === 0}
              loading={regenerating}
            >
              全部重跑 ({issueResources.length})
            </Button>
          </div>
        }
      >
        <Spin spinning={drawerLoading}>
          {issueResources.length === 0 && !drawerLoading ? (
            <Empty description="无相关资源" />
          ) : (
            <Table
              dataSource={issueResources}
              rowKey="resource_id"
              size="small"
              pagination={{ pageSize: 20 }}
              rowSelection={{
                selectedRowKeys,
                onChange: setSelectedRowKeys,
              }}
              columns={[
                {
                  title: '资源',
                  key: 'resource',
                  width: 200,
                  render: (_: unknown, r: IssueResource) => (
                    <div>
                      <div style={{ fontWeight: 500 }}>{r.resource_name || `#${r.resource_id}`}</div>
                      <div style={{ fontSize: 11, color: '#8c8c8c' }}>{r.resource_type}</div>
                    </div>
                  ),
                },
                {
                  title: '评分',
                  dataIndex: 'score',
                  width: 70,
                  render: (v: number, r: IssueResource) => (
                    <Tag color={r.verdict === 'pass' ? 'green' : r.verdict === 'fail' ? 'red' : 'orange'}>{v}</Tag>
                  ),
                },
                {
                  title: '问题详情',
                  dataIndex: 'details',
                  ellipsis: true,
                  render: (v: string) => (
                    <Tooltip title={v} placement="leftTop" overlayStyle={{ maxWidth: 500 }}>
                      <span style={{ fontSize: 12 }}>{v}</span>
                    </Tooltip>
                  ),
                },
                {
                  title: '摘要',
                  dataIndex: 'resource_summary',
                  width: 250,
                  ellipsis: true,
                  render: (v: string) => (
                    <Tooltip title={v} placement="leftTop" overlayStyle={{ maxWidth: 500 }}>
                      <span style={{ fontSize: 12 }}>{v}</span>
                    </Tooltip>
                  ),
                },
              ]}
            />
          )}
        </Spin>
      </Drawer>
    </Spin>
  );
};

export default SummaryQualityTab;

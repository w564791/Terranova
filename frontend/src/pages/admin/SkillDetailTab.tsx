import React, { useState, useEffect } from 'react';
import { Select, Segmented, Table, Tag, Tooltip, Spin, Empty } from 'antd';
import { QuestionCircleOutlined } from '@ant-design/icons';
import { useSearchParams } from 'react-router-dom';
import type { ColumnsType } from 'antd/es/table';
import { getCapabilityDetail, getAssessmentOverview, getTopViolations, CapabilityDetail, VersionStats, AssessmentRecord, FeedbackMatrix, TopViolation } from '../../services/skillAssessment';
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

const SkillDetailTab: React.FC<Props> = ({ days }) => {
  const [searchParams, setSearchParams] = useSearchParams();
  const [capabilities, setCapabilities] = useState<string[]>([]);
  const [selected, setSelectedLocal] = useState<string>(() => searchParams.get('cap') || '');
  const [detail, setDetail] = useState<CapabilityDetail | null>(null);
  const [violations, setViolations] = useState<TopViolation[]>([]);
  const [violationLayer, setViolationLayer] = useState<string>('rule');
  const [loading, setLoading] = useState(false);
  const [assessmentPage, setAssessmentPageLocal] = useState<number>(() => Number(searchParams.get('page')) || 1);
  const [assessmentPageSize, setAssessmentPageSizeLocal] = useState<number>(() => Number(searchParams.get('pageSize')) || 10);

  const setAssessmentPagination = (page: number, pageSize: number) => {
    setAssessmentPageLocal(page);
    setAssessmentPageSizeLocal(pageSize);
    const p = new URLSearchParams(searchParams);
    if (page > 1) p.set('page', String(page)); else p.delete('page');
    if (pageSize !== 10) p.set('pageSize', String(pageSize)); else p.delete('pageSize');
    setSearchParams(p, { replace: true });
  };

  const setSelected = (cap: string) => {
    setSelectedLocal(cap);
    setAssessmentPageLocal(1);
    const p = new URLSearchParams(searchParams);
    p.set('cap', cap);
    p.delete('page');
    setSearchParams(p, { replace: true });
  };

  // Load capability list
  useEffect(() => {
    getAssessmentOverview(days).then(ov => {
      const caps = (ov.by_capability || []).map(c => c.capability);
      setCapabilities(caps);
      if (caps.length > 0 && !selected) {
        setSelected(caps[0]);
      }
    });
  }, [days]);

  // Load detail + violations when selection or pagination changes
  useEffect(() => {
    if (!selected) return;
    setLoading(true);
    Promise.all([
      getCapabilityDetail(selected, days, assessmentPage, assessmentPageSize),
      getTopViolations(selected, days, 10),
    ])
      .then(([d, v]) => { setDetail(d); setViolations(v); })
      .catch(e => console.error('Failed to load detail:', e))
      .finally(() => setLoading(false));
  }, [selected, days, assessmentPage, assessmentPageSize]);

  /* ---------- Version timeline ---------- */
  const versionColumns: ColumnsType<VersionStats> = [
    {
      title: <ColTitle title="Content Hash" tip="Skill 内容的 SHA256，每次内容变更产生新 hash" />,
      dataIndex: 'content_hash',
      key: 'content_hash',
      render: (val: string, _rec: VersionStats, idx: number) => (
        <span style={{ fontFamily: 'monospace', fontSize: 12 }}>
          {val ? val.substring(0, 12) + '...' : '-'}
          {idx === 0 && <Tag color="blue" style={{ marginLeft: 6, fontSize: 11 }}>当前</Tag>}
        </span>
      ),
    },
    {
      title: <ColTitle title="Layer 1" tip="Schema 校验通过率" />,
      key: 'l1_pass_rate',
      render: (_: unknown, r: VersionStats) => {
        if (r.l1_pass_rate == null) return <span style={{ color: '#bbb' }}>-</span>;
        const rate = r.l1_pass_rate;
        let color = '#52c41a';
        if (rate < 50) color = '#ff4d4f';
        else if (rate < 80) color = '#faad14';
        return <span style={{ color, fontWeight: 600 }}>{rate.toFixed(1)}%</span>;
      },
    },
    {
      title: <ColTitle title="Layer 2" tip="规则一致性 LLM 评估均分（0-100）。>=70 良好，50-69 有问题，<50 严重违规" />,
      key: 'l2',
      render: (_: unknown, r: VersionStats) => {
        if (r.l2_avg_score == null) return <span style={{ color: '#bbb' }}>-</span>;
        const score = r.l2_avg_score;
        const color = score >= 70 ? '#52c41a' : score >= 50 ? '#faad14' : '#ff4d4f';
        return <span style={{ color, fontWeight: 600 }}>{score.toFixed(0)}<span style={{ fontSize: 11, fontWeight: 400 }}>/100</span></span>;
      },
    },
    {
      title: <ColTitle title="Layer 3" tip="语义质量 LLM 评估均分（0-100）。>=70 良好，50-69 有问题，<50 质量差" />,
      key: 'l3',
      render: (_: unknown, r: VersionStats) => {
        if (r.l3_avg_score == null) return <span style={{ color: '#bbb' }}>-</span>;
        const score = r.l3_avg_score;
        const color = score >= 70 ? '#52c41a' : score >= 50 ? '#faad14' : '#ff4d4f';
        return <span style={{ color, fontWeight: 600 }}>{score.toFixed(0)}<span style={{ fontSize: 11, fontWeight: 400 }}>/100</span></span>;
      },
    },
    {
      title: '调用次数',
      dataIndex: 'total',
      key: 'total',
    },
    {
      title: '首次出现',
      dataIndex: 'first_seen',
      key: 'first_seen',
      render: (val: string) => new Date(val).toLocaleDateString('zh-CN'),
    },
  ];

  /* ---------- Assessment records ---------- */
  const assessmentColumns: ColumnsType<AssessmentRecord> = [
    {
      title: '时间',
      dataIndex: 'assessed_at',
      key: 'assessed_at',
      render: (val: string) => new Date(val).toLocaleString('zh-CN'),
    },
    {
      title: 'Layer',
      dataIndex: 'layer',
      key: 'layer',
      render: (val: string) => <Tag>{val}</Tag>,
    },
    {
      title: 'Verdict',
      dataIndex: 'verdict',
      key: 'verdict',
      render: (val: string) => (
        <Tag color={val === 'fail' ? 'red' : val === 'warn' ? 'orange' : 'green'}>{val}</Tag>
      ),
    },
    {
      title: 'Score',
      dataIndex: 'score',
      key: 'score',
      render: (val: number) => (
        <span style={{ color: val === 0 ? '#ff4d4f' : '#52c41a', fontWeight: 600 }}>{val}</span>
      ),
    },
    {
      title: '耗时',
      dataIndex: 'latency_ms',
      key: 'latency_ms',
      render: (val: number | null) => val != null ? `${val}ms` : '-',
    },
    {
      title: '问题',
      key: 'issues',
      render: (_: unknown, r: AssessmentRecord) => {
        const issues: string[] = [];
        const mf = r.missing_fields || [];
        const ie = r.invalid_enum_fields || [];
        if (mf.length > 0) issues.push(`缺失: ${mf.join(', ')}`);
        if (ie.length > 0) issues.push(`枚举: ${ie.join(', ')}`);
        if (issues.length === 0 && r.verdict === 'fail') issues.push('输出非合法 JSON');
        const text = issues.join('; ') || '-';
        return <Tooltip title={text}><span>{text.substring(0, 50)}{text.length > 50 ? '...' : ''}</span></Tooltip>;
      },
    },
    {
      title: 'Usage Log ID',
      dataIndex: 'usage_log_id',
      key: 'usage_log_id',
      render: (val: string) => (
        <Tooltip title={val}>
          <span style={{ fontFamily: 'monospace', fontSize: 11 }}>{val.substring(0, 8)}...</span>
        </Tooltip>
      ),
    },
  ];

  // Violations: split by layer for display
  const ruleViolations = violations.filter(v => v.layer === 'rule');
  const semanticViolations = violations.filter(v => v.layer === 'semantic');
  const allViolations = [...ruleViolations, ...semanticViolations];
  const violationMax = allViolations.length > 0 ? Math.max(...allViolations.map(v => v.count)) : 1;

  if (loading) {
    return <div style={{ textAlign: 'center', padding: '80px 0' }}><Spin size="large" /></div>;
  }

  return (
    <div className={styles.container}>
      {/* Capability selector */}
      <div style={{ marginBottom: 20, display: 'flex', gap: 12, alignItems: 'center' }}>
        <span style={{ fontSize: 14, fontWeight: 500 }}>选择 Capability:</span>
        <Select
          value={selected}
          onChange={setSelected}
          style={{ minWidth: 240 }}
          options={capabilities.map(c => ({ value: c, label: c }))}
        />
      </div>

      {!detail ? (
        <Empty description="请选择一个 Capability" />
      ) : (
        <>
          {/* KPI cards */}
          <div className={styles.kpiGrid} style={{ gridTemplateColumns: 'repeat(4, 1fr)' }}>
            <div className={styles.statCard}>
              <div className={styles.statLabel}>Schema 通过率</div>
              <div className={styles.statValue} style={{
                color: detail.pass_rate >= 80 ? '#52c41a' : detail.pass_rate >= 50 ? '#faad14' : '#ff4d4f'
              }}>
                {detail.pass_rate.toFixed(1)}%
              </div>
              <div className={styles.statSub}>{detail.pass} pass / {detail.total} total</div>
            </div>
            <div className={styles.statCard}>
              <div className={styles.statLabel}>总调用次数</div>
              <div className={styles.statValue} style={{ color: '#262626' }}>{detail.total}</div>
              <div className={styles.statSub}>近 {days} 天</div>
            </div>
            <div className={styles.statCard}>
              <div className={styles.statLabel}>平均耗时</div>
              <div className={styles.statValue} style={{
                color: detail.avg_latency_ms > 10000 ? '#ff4d4f' : detail.avg_latency_ms > 5000 ? '#faad14' : '#262626',
                fontSize: detail.avg_latency_ms > 1000 ? 24 : 28,
              }}>
                {detail.avg_latency_ms > 1000
                  ? `${(detail.avg_latency_ms / 1000).toFixed(1)}s`
                  : `${Math.round(detail.avg_latency_ms)}ms`}
              </div>
              <div className={styles.statSub}>Skill 调用均耗时</div>
            </div>
            <div className={styles.statCard}>
              <div className={styles.statLabel}>当前版本</div>
              <div className={styles.statValue} style={{ fontSize: 14, fontFamily: 'monospace' }}>
                {detail.latest_hash ? detail.latest_hash.substring(0, 12) : '-'}
              </div>
              <div className={styles.statSub}>
                {detail.task_skill || detail.capability}
                {(() => {
                  const current = detail.versions?.[0];
                  if (!current?.avg_latency_ms) return null;
                  const ms = current.avg_latency_ms;
                  const text = ms > 1000 ? `${(ms / 1000).toFixed(1)}s` : `${Math.round(ms)}ms`;
                  return <span style={{ marginLeft: 8, color: '#8c8c8c' }}>⏱ {text}</span>;
                })()}
              </div>
            </div>
          </div>

          {/* Two columns: Version timeline + Violation distribution */}
          <div className={styles.twoCol}>
            {/* Version quality trend */}
            <div className={styles.sectionCard}>
              <div className={styles.sectionTitle}>
                <ColTitle title="版本质量趋势" tip="按 content_hash 分组，每个 hash 代表一个 Skill 内容版本" />
              </div>
              {detail.versions.length > 0 ? (
                <Table
                  columns={versionColumns}
                  dataSource={detail.versions}
                  pagination={false}
                  size="small"
                  rowKey="content_hash"
                />
              ) : (
                <div className={styles.emptyState}>暂无版本数据</div>
              )}
            </div>

            {/* Violation distribution — from API */}
            <div className={styles.sectionCard}>
              <div className={styles.sectionTitle} style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                <ColTitle title="高频违规分布" tip="从 L2 规则违反和 L3 质量问题中提取的高频问题（后端聚合）" />
                <Segmented
                  size="small"
                  value={violationLayer}
                  onChange={(v) => setViolationLayer(v as string)}
                  options={[
                    { label: `L2 规则 (${ruleViolations.length})`, value: 'rule' },
                    { label: `L3 语义 (${semanticViolations.length})`, value: 'semantic' },
                  ]}
                />
              </div>
              {(() => {
                const items = violationLayer === 'rule' ? ruleViolations : semanticViolations;
                const color = violationLayer === 'rule' ? '#ff4d4f' : '#fa8c16';
                const max = items.length > 0 ? Math.max(...items.map(v => v.count)) : 1;
                if (items.length === 0) return <div className={styles.emptyState}>无违规记录</div>;
                return items.map((v, i) => (
                  <div key={i} className={styles.hbarRow}>
                    <Tooltip title={v.rule_name}>
                      <span className={styles.hbarLabel}>{v.rule_name}</span>
                    </Tooltip>
                    <div className={styles.hbarTrack}>
                      <div className={styles.hbarFill} style={{ width: `${(v.count / max) * 100}%`, background: color }} />
                    </div>
                    <span className={styles.hbarCount}>{v.count}</span>
                  </div>
                ));
              })()}
            </div>
          </div>

          {/* Assessment vs User Feedback matrix */}
          <div className={styles.sectionCard} style={{ marginBottom: 16 }}>
            <div className={styles.sectionTitle}>
              <ColTitle title="评估结果 vs 用户反馈" tip="评估结论与用户实际反馈的交叉对比，用于检测评估盲区" />
            </div>
            <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 24 }}>
              {(() => {
                const fm = detail.feedback_matrix;
                const cell = (v: number) => (
                  <td style={{ padding: '8px 12px', borderBottom: '1px solid #f0f0f0', textAlign: 'center', color: v > 0 ? '#262626' : '#bbb' }}>
                    {v > 0 ? v : '-'}
                  </td>
                );
                const totalWithFeedback = fm
                  ? fm.pass_positive + fm.pass_negative + fm.warn_positive + fm.warn_negative + fm.fail_positive + fm.fail_negative
                  : 0;
                const blindSpot = totalWithFeedback > 0 && fm
                  ? ((fm.pass_negative / totalWithFeedback) * 100).toFixed(1)
                  : null;
                return (
                  <>
                    <div>
                      <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: 13 }}>
                        <thead>
                          <tr style={{ background: '#fafafa' }}>
                            <th style={{ padding: '8px 12px', textAlign: 'left', borderBottom: '1px solid #e8e8e8' }}></th>
                            <th style={{ padding: '8px 12px', textAlign: 'center', borderBottom: '1px solid #e8e8e8' }}>好评 (4-5)</th>
                            <th style={{ padding: '8px 12px', textAlign: 'center', borderBottom: '1px solid #e8e8e8' }}>差评 (1-2)</th>
                            <th style={{ padding: '8px 12px', textAlign: 'center', borderBottom: '1px solid #e8e8e8' }}>无反馈</th>
                          </tr>
                        </thead>
                        <tbody>
                          <tr>
                            <td style={{ padding: '8px 12px', borderBottom: '1px solid #f0f0f0' }}><Tag color="green">评估 pass</Tag></td>
                            {cell(fm?.pass_positive ?? 0)}
                            {cell(fm?.pass_negative ?? 0)}
                            {cell(fm?.pass_no_feedback ?? 0)}
                          </tr>
                          <tr>
                            <td style={{ padding: '8px 12px', borderBottom: '1px solid #f0f0f0' }}><Tag color="orange">评估 warn</Tag></td>
                            {cell(fm?.warn_positive ?? 0)}
                            {cell(fm?.warn_negative ?? 0)}
                            {cell(fm?.warn_no_feedback ?? 0)}
                          </tr>
                          <tr>
                            <td style={{ padding: '8px 12px' }}><Tag color="red">评估 fail</Tag></td>
                            {cell(fm?.fail_positive ?? 0)}
                            {cell(fm?.fail_negative ?? 0)}
                            {cell(fm?.fail_no_feedback ?? 0)}
                          </tr>
                        </tbody>
                      </table>
                    </div>
                    <div style={{ padding: 16 }}>
                      <div style={{ fontSize: 13, fontWeight: 600, color: '#262626', marginBottom: 12 }}>盲区检测</div>
                      <div style={{ fontSize: 12, color: '#595959', lineHeight: 1.8 }}>
                        {totalWithFeedback > 0 ? (
                          <>
                            <div>共 <b>{totalWithFeedback}</b> 条有用户评分的记录：</div>
                            {fm!.pass_negative > 0 && (
                              <div style={{ color: '#ff4d4f', fontWeight: 500 }}>
                                ⚠ AI 评估通过但用户给了差评：<b>{fm!.pass_negative}</b> 条
                                <span style={{ color: '#8c8c8c', fontWeight: 400 }}>（占 {((fm!.pass_negative / totalWithFeedback) * 100).toFixed(0)}%，说明评估标准可能遗漏了用户关注的问题）</span>
                              </div>
                            )}
                            {fm!.fail_positive > 0 && (
                              <div style={{ color: '#fa8c16', fontWeight: 500 }}>
                                ⚠ AI 评估失败但用户给了好评：<b>{fm!.fail_positive}</b> 条
                                <span style={{ color: '#8c8c8c', fontWeight: 400 }}>（说明评估标准可能过于严格）</span>
                              </div>
                            )}
                            {fm!.pass_negative === 0 && fm!.fail_positive === 0 && (
                              <div style={{ color: '#52c41a' }}>✓ 评估结果与用户反馈一致，暂无盲区</div>
                            )}
                          </>
                        ) : (
                          <div style={{ color: '#8c8c8c' }}>暂无用户评分数据，无法检测盲区</div>
                        )}
                      </div>
                    </div>
                  </>
                );
              })()}
            </div>
          </div>

          {/* Recent assessment records (all verdicts) */}
          <div className={`${styles.sectionCard} ${styles.tableCard}`}>
            <div className={styles.sectionTitle}>
              <ColTitle title="最近评估记录" tip="该 Capability 的评估记录（包含所有 verdict）" />
            </div>
            {detail.assessments.length > 0 ? (
              <Table
                columns={assessmentColumns}
                dataSource={detail.assessments}
                pagination={{
                  current: assessmentPage,
                  pageSize: assessmentPageSize,
                  total: detail.assessment_total || 0,
                  showSizeChanger: true,
                  pageSizeOptions: ['10', '20', '50'],
                  showTotal: (total) => `共 ${total} 条`,
                  onChange: (page, size) => setAssessmentPagination(page, size),
                }}
                size="small"
                rowKey="usage_log_id"
              />
            ) : (
              <Empty description="暂无评估记录" image={Empty.PRESENTED_IMAGE_SIMPLE} />
            )}
          </div>
        </>
      )}
    </div>
  );
};

export default SkillDetailTab;

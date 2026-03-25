import React, { useState, useEffect } from 'react';
import { Select, Table, Tag, Tooltip, Spin, Empty } from 'antd';
import { QuestionCircleOutlined } from '@ant-design/icons';
import type { ColumnsType } from 'antd/es/table';
import { getCapabilityDetail, getAssessmentOverview, CapabilityDetail, VersionStats, AssessmentRecord, FeedbackMatrix } from '../../services/skillAssessment';
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
  const [capabilities, setCapabilities] = useState<string[]>([]);
  const [selected, setSelected] = useState<string>('');
  const [detail, setDetail] = useState<CapabilityDetail | null>(null);
  const [loading, setLoading] = useState(false);

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

  // Load detail when selection changes
  useEffect(() => {
    if (!selected) return;
    setLoading(true);
    getCapabilityDetail(selected, days)
      .then(setDetail)
      .catch(e => console.error('Failed to load detail:', e))
      .finally(() => setLoading(false));
  }, [selected, days]);

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
      key: 'pass_rate',
      render: (_: unknown, r: VersionStats) => {
        let color = '#52c41a';
        if (r.pass_rate < 50) color = '#ff4d4f';
        else if (r.pass_rate < 80) color = '#faad14';
        return <span style={{ color, fontWeight: 600 }}>{r.pass_rate.toFixed(1)}%</span>;
      },
    },
    {
      title: <ColTitle title="Layer 2" tip="规则一致性评估通过率（LLM 评估）" />,
      key: 'l2',
      render: (_: unknown, r: VersionStats) => {
        if (r.l2_pass_rate == null) return <span style={{ color: '#bbb' }}>-</span>;
        const color = r.l2_pass_rate >= 80 ? '#52c41a' : r.l2_pass_rate >= 50 ? '#faad14' : '#ff4d4f';
        return <Tooltip title={`均分: ${r.l2_avg_score?.toFixed(1) ?? '-'}`}><span style={{ color, fontWeight: 600 }}>{r.l2_pass_rate.toFixed(1)}%</span></Tooltip>;
      },
    },
    {
      title: <ColTitle title="Layer 3" tip="语义质量评估通过率（LLM 评估）" />,
      key: 'l3',
      render: (_: unknown, r: VersionStats) => {
        if (r.l3_pass_rate == null) return <span style={{ color: '#bbb' }}>-</span>;
        const color = r.l3_pass_rate >= 80 ? '#52c41a' : r.l3_pass_rate >= 50 ? '#faad14' : '#ff4d4f';
        return <Tooltip title={`均分: ${r.l3_avg_score?.toFixed(1) ?? '-'}`}><span style={{ color, fontWeight: 600 }}>{r.l3_pass_rate.toFixed(1)}%</span></Tooltip>;
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

  /* ---------- Violation distribution from assessments ---------- */
  const violations = React.useMemo(() => {
    if (!detail?.assessments) return [];
    const counts: Record<string, { count: number; color: string }> = {};
    for (const a of detail.assessments) {
      if (a.verdict !== 'fail') continue;
      for (const mf of a.missing_fields || []) {
        const k = `缺失: ${mf}`;
        if (!counts[k]) counts[k] = { count: 0, color: '#ff4d4f' };
        counts[k].count++;
      }
      for (const ie of a.invalid_enum_fields || []) {
        const k = `枚举: ${ie}`;
        if (!counts[k]) counts[k] = { count: 0, color: '#fa8c16' };
        counts[k].count++;
      }
      if (!(a.missing_fields?.length) && !(a.invalid_enum_fields?.length)) {
        const k = '输出非合法 JSON';
        if (!counts[k]) counts[k] = { count: 0, color: '#ff4d4f' };
        counts[k].count++;
      }
    }
    return Object.entries(counts)
      .sort((a, b) => b[1].count - a[1].count)
      .slice(0, 10)
      .map(([label, { count, color }]) => ({ label, count, color }));
  }, [detail?.assessments]);

  const violationMax = violations.length > 0 ? violations[0].count : 1;

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
              <div className={styles.statSub}>{detail.task_skill || detail.capability}</div>
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

            {/* Violation distribution */}
            <div className={styles.sectionCard}>
              <div className={styles.sectionTitle}>
                <ColTitle title="违规分布" tip="该 Capability 下最常见的 Schema 校验失败原因" />
              </div>
              <div style={{ fontSize: 12, color: '#8c8c8c', marginBottom: 8, fontWeight: 500 }}>Layer 1 - Schema 违规</div>
              {violations.length > 0 ? (
                violations.map(v => (
                  <div key={v.label} className={styles.hbarRow}>
                    <Tooltip title={v.label}>
                      <span className={styles.hbarLabel}>{v.label}</span>
                    </Tooltip>
                    <div className={styles.hbarTrack}>
                      <div className={styles.hbarFill} style={{ width: `${(v.count / violationMax) * 100}%`, background: v.color }} />
                    </div>
                    <span className={styles.hbarCount}>{v.count} 次</span>
                  </div>
                ))
              ) : (
                <div className={styles.emptyState}>无违规记录</div>
              )}
              <div style={{ marginTop: 24, fontSize: 12, color: '#8c8c8c', fontWeight: 500 }}>Layer 2 - 规则违规</div>
              {(() => {
                // Parse rule_violations from assessments with layer=rule
                const ruleViolations: { label: string; count: number }[] = [];
                const counts: Record<string, number> = {};
                for (const a of detail?.assessments || []) {
                  if (a.layer !== 'rule' || a.verdict === 'pass') continue;
                  // rule_violations is JSON array of {rule, detail}
                  try {
                    const violations = typeof a.missing_fields === 'string' ? [] : (a as any).rule_violations;
                    // rule_violations may not be in AssessmentRecord type, check raw
                  } catch { /* ignore */ }
                }
                // Simpler: just show count of rule layer failures
                const ruleAssessments = (detail?.assessments || []).filter(a => a.layer === 'rule');
                const ruleFails = ruleAssessments.filter(a => a.verdict === 'fail' || a.verdict === 'warn');
                if (ruleFails.length === 0) {
                  return <div className={styles.emptyState} style={{ padding: 16 }}>无规则违规记录</div>;
                }
                return ruleFails.map(a => (
                  <div key={a.usage_log_id} className={styles.hbarRow}>
                    <span className={styles.hbarLabel}>评估 {a.verdict} (score: {a.score})</span>
                    <div className={styles.hbarTrack}>
                      <div className={styles.hbarFill} style={{ width: `${100 - a.score}%`, background: a.verdict === 'fail' ? '#ff4d4f' : '#fa8c16' }} />
                    </div>
                    <span className={styles.hbarCount}>{a.score}分</span>
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
              <ColTitle title="最近评估记录" tip="该 Capability 最近 20 条评估记录（包含所有 verdict）" />
            </div>
            {detail.assessments.length > 0 ? (
              <Table
                columns={assessmentColumns}
                dataSource={detail.assessments}
                pagination={false}
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

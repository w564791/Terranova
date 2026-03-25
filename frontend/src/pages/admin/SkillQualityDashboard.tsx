import React, { useState, useEffect, useMemo } from 'react';
import { Table, Tag, Tabs, Segmented, Spin, Empty, Tooltip } from 'antd';
import { QuestionCircleOutlined } from '@ant-design/icons';
import SkillDetailTab from './SkillDetailTab';
import type { ColumnsType } from 'antd/es/table';
import { useSearchParams } from 'react-router-dom';
import { getAssessmentOverview, AssessmentOverview, CapabilityStats, RecentFailure, DailyTrendItem } from '../../services/skillAssessment';
import styles from './SkillQualityDashboard.module.css';

const timeRangeMap: Record<string, number> = {
  '24h': 1,
  '7天': 7,
  '30天': 30,
};
const daysToLabel: Record<number, string> = { 1: '24h', 7: '7天', 30: '30天' };

// 列标题 + 说明 tooltip
const ColTitle: React.FC<{ title: string; tip: string }> = ({ title, tip }) => (
  <span>
    {title}{' '}
    <Tooltip title={tip}>
      <QuestionCircleOutlined style={{ color: '#8c8c8c', fontSize: 12, cursor: 'help' }} />
    </Tooltip>
  </span>
);

/* ------------------------------------------------------------------ */
/* Violation distribution: parse from recent_failures                  */
/* ------------------------------------------------------------------ */
interface ViolationEntry {
  label: string;
  count: number;
  color: string;
}

function buildViolations(failures: RecentFailure[] | undefined): ViolationEntry[] {
  if (!failures || failures.length === 0) return [];
  const counts: Record<string, { count: number; type: 'missing' | 'enum' }> = {};
  for (const f of failures) {
    for (const mf of f.missing_fields || []) {
      const key = `缺失字段: ${mf}`;
      if (!counts[key]) counts[key] = { count: 0, type: 'missing' };
      counts[key].count++;
    }
    for (const ie of f.invalid_enum_fields || []) {
      const key = `枚举无效: ${ie}`;
      if (!counts[key]) counts[key] = { count: 0, type: 'enum' };
      counts[key].count++;
    }
  }
  return Object.entries(counts)
    .sort((a, b) => b[1].count - a[1].count)
    .slice(0, 10)
    .map(([label, { count, type }]) => ({
      label,
      count,
      color: type === 'missing' ? '#ff4d4f' : '#fa8c16',
    }));
}

/* ------------------------------------------------------------------ */
/* Main component                                                      */
/* ------------------------------------------------------------------ */
const SkillQualityDashboard: React.FC = () => {
  const [searchParams, setSearchParams] = useSearchParams();
  const [data, setData] = useState<AssessmentOverview | null>(null);
  const [loading, setLoading] = useState(true);

  // Local state for immediate UI response, URL for persistence
  const [days, setDaysLocal] = useState(() => Number(searchParams.get('days')) || 7);
  const setDays = (d: number) => {
    setDaysLocal(d);
    const p = new URLSearchParams(searchParams);
    p.set('days', String(d));
    setSearchParams(p, { replace: true });
  };

  useEffect(() => {
    loadData();
  }, [days]);

  const loadData = async () => {
    setLoading(true);
    try {
      const result = await getAssessmentOverview(days);
      setData(result);
    } catch (e) {
      console.error('Failed to load assessment data:', e);
    } finally {
      setLoading(false);
    }
  };

  const violations = useMemo(() => buildViolations(data?.recent_failures), [data?.recent_failures]);

  /* ---------- Capability table columns ---------- */
  const capabilityColumns: ColumnsType<CapabilityStats> = [
    {
      title: <ColTitle title="Capability" tip="AI 能力类型：form_generation（配置生成）、plan_summary（Plan 分析）、apply_summary（Apply 分析）、module_skill_generation（Skill 自动生成）" />,
      dataIndex: 'capability',
      key: 'capability',
    },
    {
      title: <ColTitle title="Total" tip="该时间范围内的总评估次数" />,
      dataIndex: 'total',
      key: 'total',
    },
    {
      title: <ColTitle title="Pass" tip="Schema 校验通过的次数（输出结构合规）" />,
      dataIndex: 'pass',
      key: 'pass',
      render: (val: number) => <span style={{ color: '#52c41a' }}>{val}</span>,
    },
    {
      title: <ColTitle title="Fail" tip="Schema 校验失败的次数（缺少必填字段、枚举值非法或 JSON 格式错误）" />,
      dataIndex: 'fail',
      key: 'fail',
      render: (val: number) => <span style={{ color: val > 0 ? '#ff4d4f' : undefined }}>{val}</span>,
    },
    {
      title: <ColTitle title="均分" tip="评估平均分（0-100），Schema 校验 pass=100, fail=0" />,
      dataIndex: 'avg_score',
      key: 'avg_score',
      render: (val: number) => {
        let color = '#52c41a';
        if (val < 50) color = '#ff4d4f';
        else if (val < 80) color = '#faad14';
        return <span style={{ color, fontWeight: 600 }}>{val.toFixed(1)}</span>;
      },
    },
    {
      title: <ColTitle title="通过率" tip="Pass / Total 的百分比" />,
      dataIndex: 'pass_rate',
      key: 'pass_rate',
      render: (val: number) => {
        let color = '#52c41a';
        if (val < 50) color = '#ff4d4f';
        else if (val < 80) color = '#faad14';
        return <span style={{ color, fontWeight: 600 }}>{val.toFixed(1)}%</span>;
      },
    },
    {
      title: <ColTitle title="Skill 均耗时" tip="该 Capability 下 Skill 调用的平均耗时（毫秒），包含 AI 调用时间" />,
      dataIndex: 'avg_latency_ms',
      key: 'avg_latency_ms',
      render: (val: number) => {
        if (!val) return '-';
        if (val > 10000) return <span style={{ color: '#ff4d4f' }}>{(val / 1000).toFixed(1)}s</span>;
        if (val > 5000) return <span style={{ color: '#faad14' }}>{(val / 1000).toFixed(1)}s</span>;
        return `${Math.round(val)}ms`;
      },
    },
    {
      title: <ColTitle title="状态" tip="健康: 通过率 >= 80%，风险: 通过率 < 80%" />,
      key: 'status',
      render: (_: unknown, record: CapabilityStats) => {
        if (record.total === 0) return <Tag>无数据</Tag>;
        if (record.pass_rate >= 80) return <Tag color="green">健康</Tag>;
        return <Tag color="red">风险</Tag>;
      },
    },
  ];

  /* ---------- Recent failures table columns ---------- */
  const failureColumns: ColumnsType<RecentFailure> = [
    {
      title: <ColTitle title="时间" tip="评估完成的时间" />,
      dataIndex: 'assessed_at',
      key: 'assessed_at',
      render: (val: string) => new Date(val).toLocaleString('zh-CN'),
    },
    {
      title: 'Capability',
      dataIndex: 'capability',
      key: 'capability',
    },
    {
      title: <ColTitle title="Skill Name" tip="被评估的 Task Skill 名称，或 capability 名（scanner 兜底）" />,
      dataIndex: 'skill_name',
      key: 'skill_name',
    },
    {
      title: 'Verdict',
      dataIndex: 'verdict',
      key: 'verdict',
      render: (val: string) => <Tag color={val === 'fail' ? 'red' : val === 'warn' ? 'orange' : 'green'}>{val}</Tag>,
    },
    {
      title: <ColTitle title="评分" tip="Schema 校验评分：pass=100, fail=0" />,
      dataIndex: 'score',
      key: 'score',
      render: (val: number) => <span style={{ color: val === 0 ? '#ff4d4f' : '#52c41a', fontWeight: 600 }}>{val}</span>,
    },
    {
      title: <ColTitle title="问题" tip="Schema 校验发现的具体问题：缺失的必填字段或枚举值非法" />,
      key: 'issues',
      render: (_: unknown, record: RecentFailure) => {
        const issues: string[] = [];
        const mf = record.missing_fields || [];
        const ie = record.invalid_enum_fields || [];
        if (mf.length > 0) {
          issues.push(`缺失字段: ${mf.join(', ')}`);
        }
        if (ie.length > 0) {
          issues.push(`枚举无效: ${ie.join(', ')}`);
        }
        if (issues.length === 0 && record.verdict === 'fail') {
          issues.push('输出非合法 JSON');
        }
        const text = issues.join('; ');
        return (
          <Tooltip title={text}>
            <span>{text.substring(0, 60)}{text.length > 60 ? '...' : ''}</span>
          </Tooltip>
        );
      },
    },
    {
      title: <ColTitle title="Content Hash" tip="Skill 内容的 SHA256 hash，用于追踪版本变更" />,
      dataIndex: 'content_hash',
      key: 'content_hash',
      render: (val: string) => (
        <Tooltip title={val}>
          <span style={{ fontFamily: 'monospace', fontSize: 12 }}>{val ? val.substring(0, 12) + '...' : '-'}</span>
        </Tooltip>
      ),
    },
  ];

  /* ---------- Loading / empty states ---------- */
  if (loading) {
    return (
      <div style={{ textAlign: 'center', padding: '80px 0' }}>
        <Spin size="large" />
      </div>
    );
  }

  if (!data) {
    return <Empty description="暂无评估数据" />;
  }

  /* ---------- Derived values ---------- */
  const totalAssessed = data.total_pass + data.total_fail + data.total_warn;
  const passRate = totalAssessed > 0 ? (data.total_pass / totalAssessed) * 100 : 0;
  const coverage = data.total_logs > 0 ? ((data.assessed_logs / data.total_logs) * 100) : 0;

  // Donut angles
  const passDeg = totalAssessed > 0 ? (data.total_pass / totalAssessed) * 360 : 0;
  const failDeg = totalAssessed > 0 ? (data.total_fail / totalAssessed) * 360 : 0;
  const donutGradient = totalAssessed > 0
    ? `conic-gradient(#52c41a 0deg ${passDeg}deg, #ff4d4f ${passDeg}deg ${passDeg + failDeg}deg, #fa8c16 ${passDeg + failDeg}deg 360deg)`
    : '#f0f0f0';

  // Bar chart scaling
  const trendData = data.daily_trend || [];
  const maxBar = Math.max(...trendData.map(d => d.pass + d.fail + d.warn), 1);
  const barHeight = 170; // px available for bars

  // Alert summary
  const hasSchemaErrorRate = (data.by_capability || []).some(c => c.total > 0 && c.fail / c.total > 0.1);
  const hasHighRisk = (data.high_risk_skills?.length || 0) > 0;
  const pendingBacklog = data.total_logs - data.assessed_logs;

  // Violation max
  const violationMax = violations.length > 0 ? violations[0].count : 1;

  const [activeSubTab, setActiveSubTabLocal] = useState(() => searchParams.get('subtab') || 'overview');
  const setActiveSubTab = (key: string) => {
    setActiveSubTabLocal(key);
    const p = new URLSearchParams(searchParams);
    p.set('subtab', key);
    setSearchParams(p, { replace: true });
  };

  return (
    <div className={styles.container}>
      <div className={styles.timeRange}>
        <Segmented
          options={['24h', '7天', '30天']}
          value={daysToLabel[days] || '7天'}
          onChange={(val) => setDays(timeRangeMap[val as string])}
        />
      </div>

      <Tabs
        activeKey={activeSubTab}
        onChange={setActiveSubTab}
        items={[
          { key: 'overview', label: '全局概览', children: null },
          { key: 'detail', label: 'Skill 详情', children: null },
        ]}
        style={{ marginBottom: 16 }}
      />

      {activeSubTab === 'detail' ? (
        <SkillDetailTab days={days} />
      ) : (
      <>

      {/* ===================== KPI Row ===================== */}
      <div className={styles.kpiGrid}>
        <div className={styles.statCard}>
          <div className={styles.statLabel}>
            <ColTitle title="Schema 通过率" tip="Layer 1 纯代码校验的通过率，检查输出 JSON 结构是否符合预期（必填字段、枚举值）" />
          </div>
          <div className={styles.statValue} style={{ color: data.pass_rate >= 80 ? '#52c41a' : data.pass_rate >= 50 ? '#faad14' : '#ff4d4f' }}>
            {data.pass_rate.toFixed(1)}%
          </div>
          <div className={styles.statSub}>Pass {data.total_pass} / Total {totalAssessed}</div>
        </div>

        <div className={styles.statCard}>
          <div className={styles.statLabel}>
            <ColTitle title="评估覆盖率" tip="已完成评估的记录占总记录的百分比，100% 表示所有 Skill 调用都已评估" />
          </div>
          <div className={styles.statValue} style={{ color: '#1677ff' }}>
            {coverage.toFixed(1)}%
          </div>
          <div className={styles.statSub}>{data.assessed_logs} / {data.total_logs} 条记录</div>
        </div>

        <div className={styles.statCard}>
          <div className={styles.statLabel}>
            <ColTitle title="活跃 Skill 数" tip="该时间范围内有调用记录的不同 Capability 数量" />
          </div>
          <div className={styles.statValue} style={{ color: '#262626' }}>
            {data.active_skills}
          </div>
          <div className={styles.statSub}>Capability 类型</div>
        </div>

        <div className={styles.statCard}>
          <div className={styles.statLabel}>
            <ColTitle title="高风险 Skill" tip="Schema 校验 fail 率超过 20% 的 Capability 数量" />
          </div>
          <div className={styles.statValue} style={{ color: (data.high_risk_skills?.length || 0) > 0 ? '#ff4d4f' : '#52c41a' }}>
            {data.high_risk_skills ? data.high_risk_skills.length : 0}
          </div>
          {data.high_risk_skills && data.high_risk_skills.length > 0 && (
            <div className={styles.statSub}>{data.high_risk_skills.join(', ')}</div>
          )}
        </div>

        <div className={styles.statCard}>
          <div className={styles.statLabel}>
            <ColTitle title="总评估数" tip="该时间范围内完成的 Schema 校验总次数" />
          </div>
          <div className={styles.statValue} style={{ color: '#262626' }}>
            {totalAssessed}
          </div>
          <div className={styles.statSub}>Pass {data.total_pass} / Fail {data.total_fail} / Warn {data.total_warn}</div>
        </div>
      </div>

      {/* ===================== Donut + Alert Summary ===================== */}
      <div className={styles.twoCol}>
        {/* Evaluation Distribution — Donut */}
        <div className={styles.sectionCard}>
          <div className={styles.sectionTitle}>
            <ColTitle title="评估结果分布" tip="Pass/Fail/Warn 的数量和占比" />
          </div>
          <div className={styles.donutWrap}>
            <div
              className={styles.donut}
              style={{ background: donutGradient }}
            >
              <div className={styles.donutCenter}>
                <span className={styles.donutRate}>{passRate.toFixed(0)}%</span>
                <span className={styles.donutRateLabel}>通过率</span>
              </div>
            </div>
            <div className={styles.legend}>
              <div className={styles.legendItem}>
                <span className={styles.legendDot} style={{ background: '#52c41a' }} />
                Pass <span className={styles.legendValue}>{data.total_pass}</span>
              </div>
              <div className={styles.legendItem}>
                <span className={styles.legendDot} style={{ background: '#ff4d4f' }} />
                Fail <span className={styles.legendValue}>{data.total_fail}</span>
              </div>
              <div className={styles.legendItem}>
                <span className={styles.legendDot} style={{ background: '#fa8c16' }} />
                Warn <span className={styles.legendValue}>{data.total_warn}</span>
              </div>
            </div>
          </div>
        </div>

        {/* Alert Summary */}
        <div className={styles.sectionCard}>
          <div className={styles.sectionTitle}>
            <ColTitle title="告警摘要" tip="基于当前数据自动检测的风险告警" />
          </div>
          {hasSchemaErrorRate || hasHighRisk || pendingBacklog > 0 ? (
            <>
              {hasSchemaErrorRate && (
                <div className={`${styles.alertRow} ${styles.alertP1}`}>
                  <span className={styles.alertLabel}>Schema 错误率超过 10%</span>
                  <span className={`${styles.alertBadge} ${styles.alertBadgeP1}`}>P1 告警</span>
                </div>
              )}
              {hasHighRisk && (
                <div className={`${styles.alertRow} ${styles.alertP2}`}>
                  <span className={styles.alertLabel}>高风险 Skill: {data.high_risk_skills!.join(', ')}</span>
                  <span className={`${styles.alertBadge} ${styles.alertBadgeP2}`}>P2 风险</span>
                </div>
              )}
              {pendingBacklog > 0 && (
                <div className={`${styles.alertRow} ${styles.alertP3}`}>
                  <span className={styles.alertLabel}>评估积压 {pendingBacklog} 条</span>
                  <span className={`${styles.alertBadge} ${styles.alertBadgeP3}`}>P3 待处理</span>
                </div>
              )}
            </>
          ) : (
            <div className={styles.emptyState} style={{ padding: '32px 0', color: '#52c41a' }}>
              暂无告警
            </div>
          )}
        </div>
      </div>

      {/* ===================== Capability Table ===================== */}
      <div className={`${styles.sectionCard} ${styles.tableCard}`}>
        <div className={styles.sectionTitle}>
          <ColTitle title="各 Capability 通过率" tip="按 AI 能力类型分组的 Schema 校验统计，包含通过率、平均分和平均耗时" />
        </div>
        <Table
          columns={capabilityColumns}
          dataSource={data.by_capability}
          pagination={false}
          size="small"
          rowKey="capability"
        />
      </div>

      {/* ===================== Two-column: Trend bar chart + Violation distribution ===================== */}
      <div className={styles.twoCol}>
        {/* Daily Trend — Bar Chart */}
        <div className={styles.sectionCard}>
          <div className={styles.sectionTitle}>
            <ColTitle title="每日评估趋势" tip="按调用日期聚合的每日 Schema 校验结果统计" />
          </div>
          {trendData.length > 0 ? (
            <>
              <div className={styles.barChart}>
                {trendData.map((d) => {
                  const total = d.pass + d.fail + d.warn;
                  const passH = (d.pass / maxBar) * barHeight;
                  const failH = (d.fail / maxBar) * barHeight;
                  const warnH = (d.warn / maxBar) * barHeight;
                  return (
                    <Tooltip key={d.date} title={`${d.date}: Pass ${d.pass}, Fail ${d.fail}, Warn ${d.warn}, Total ${total}`}>
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
              <div className={styles.barLegend}>
                <span className={styles.barLegendItem}>
                  <span className={styles.barLegendDot} style={{ background: '#52c41a' }} /> Pass
                </span>
                <span className={styles.barLegendItem}>
                  <span className={styles.barLegendDot} style={{ background: '#ff4d4f' }} /> Fail
                </span>
                <span className={styles.barLegendItem}>
                  <span className={styles.barLegendDot} style={{ background: '#fa8c16' }} /> Warn
                </span>
              </div>
            </>
          ) : (
            <div className={styles.emptyState}>暂无趋势数据</div>
          )}
        </div>

        {/* Violation Distribution — Horizontal Bar Chart */}
        <div className={styles.sectionCard}>
          <div className={styles.sectionTitle}>
            <ColTitle title="违规分布" tip="最近失败记录中最常见的字段缺失和枚举非法问题" />
          </div>
          {violations.length > 0 ? (
            violations.map((v) => (
              <div key={v.label} className={styles.hbarRow}>
                <Tooltip title={v.label}>
                  <span className={styles.hbarLabel}>{v.label}</span>
                </Tooltip>
                <div className={styles.hbarTrack}>
                  <div
                    className={styles.hbarFill}
                    style={{ width: `${(v.count / violationMax) * 100}%`, background: v.color }}
                  />
                </div>
                <span className={styles.hbarCount}>{v.count}</span>
              </div>
            ))
          ) : (
            <div className={styles.emptyState}>无违规记录</div>
          )}
        </div>
      </div>

      {/* ===================== Recent Failures Table ===================== */}
      <div className={`${styles.sectionCard} ${styles.tableCard}`}>
        <div className={styles.sectionTitle} style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
          <ColTitle title="最近失败记录" tip="最近 10 条 Schema 校验失败的记录，显示失败原因和 Skill 版本" />
          {data.recent_failures && data.recent_failures.length > 0 && (
            <Tag color="red">{data.recent_failures.length}</Tag>
          )}
        </div>
        {data.recent_failures && data.recent_failures.length > 0 ? (
          <Table
            columns={failureColumns}
            dataSource={data.recent_failures}
            pagination={false}
            size="small"
            rowKey="usage_log_id"
          />
        ) : (
          <Empty description="无失败记录" image={Empty.PRESENTED_IMAGE_SIMPLE} />
        )}
      </div>
      </>
      )}
    </div>
  );
};

export default SkillQualityDashboard;

import React, { useState, useEffect } from 'react';
import { Card, Row, Col, Statistic, Table, Tag, Progress, Segmented, Spin, Empty, Tooltip } from 'antd';
import { QuestionCircleOutlined } from '@ant-design/icons';
import type { ColumnsType } from 'antd/es/table';
import { getAssessmentOverview, AssessmentOverview, CapabilityStats, RecentFailure, DailyTrendItem } from '../../services/skillAssessment';
import styles from './SkillQualityDashboard.module.css';

const timeRangeMap: Record<string, number> = {
  '24h': 1,
  '7天': 7,
  '30天': 30,
};

// 列标题 + 说明 tooltip
const ColTitle: React.FC<{ title: string; tip: string }> = ({ title, tip }) => (
  <span>
    {title}{' '}
    <Tooltip title={tip}>
      <QuestionCircleOutlined style={{ color: '#8c8c8c', fontSize: 12, cursor: 'help' }} />
    </Tooltip>
  </span>
);

const SkillQualityDashboard: React.FC = () => {
  const [data, setData] = useState<AssessmentOverview | null>(null);
  const [loading, setLoading] = useState(true);
  const [days, setDays] = useState(7);

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

  const trendColumns: ColumnsType<DailyTrendItem> = [
    {
      title: '日期',
      dataIndex: 'date',
      key: 'date',
    },
    {
      title: <ColTitle title="Pass" tip="当天 Schema 校验通过的次数" />,
      dataIndex: 'pass',
      key: 'pass',
      render: (val: number) => <span style={{ color: '#52c41a' }}>{val}</span>,
    },
    {
      title: <ColTitle title="Fail" tip="当天 Schema 校验失败的次数" />,
      dataIndex: 'fail',
      key: 'fail',
      render: (val: number) => <span style={{ color: val > 0 ? '#ff4d4f' : undefined }}>{val}</span>,
    },
    {
      title: 'Warn',
      dataIndex: 'warn',
      key: 'warn',
      render: (val: number) => <span style={{ color: val > 0 ? '#faad14' : undefined }}>{val}</span>,
    },
    {
      title: 'Total',
      key: 'total',
      render: (_: unknown, record: DailyTrendItem) => record.pass + record.fail + record.warn,
    },
    {
      title: <ColTitle title="通过率" tip="当天 Pass / Total 百分比" />,
      key: 'pass_rate',
      render: (_: unknown, record: DailyTrendItem) => {
        const total = record.pass + record.fail + record.warn;
        if (total === 0) return '-';
        const rate = (record.pass / total) * 100;
        let color = '#52c41a';
        if (rate < 50) color = '#ff4d4f';
        else if (rate < 80) color = '#faad14';
        return <span style={{ color, fontWeight: 600 }}>{rate.toFixed(1)}%</span>;
      },
    },
  ];

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

  const totalAssessed = data.total_pass + data.total_fail + data.total_warn;
  const passPercent = totalAssessed > 0 ? Math.round((data.total_pass / totalAssessed) * 100) : 0;
  const failPercent = totalAssessed > 0 ? Math.round((data.total_fail / totalAssessed) * 100) : 0;
  const warnPercent = totalAssessed > 0 ? Math.round((data.total_warn / totalAssessed) * 100) : 0;
  const coverage = data.total_logs > 0 ? ((data.assessed_logs / data.total_logs) * 100).toFixed(1) : '0';

  return (
    <div className={styles.container}>
      <div className={styles.timeRange}>
        <Segmented
          options={['24h', '7天', '30天']}
          defaultValue="7天"
          onChange={(val) => setDays(timeRangeMap[val as string])}
        />
      </div>

      <Row gutter={16} className={styles.kpiRow}>
        <Col span={5}>
          <Card size="small">
            <Statistic
              title={<ColTitle title="Schema 通过率" tip="Layer 1 纯代码校验的通过率，检查输出 JSON 结构是否符合预期（必填字段、枚举值）" />}
              value={data.pass_rate}
              precision={1}
              suffix="%"
              valueStyle={{ color: data.pass_rate >= 80 ? '#52c41a' : data.pass_rate >= 50 ? '#faad14' : '#ff4d4f' }}
            />
          </Card>
        </Col>
        <Col span={5}>
          <Card size="small">
            <Statistic
              title={<ColTitle title="评估覆盖率" tip="已完成评估的记录占总记录的百分比，100% 表示所有 Skill 调用都已评估" />}
              value={parseFloat(coverage)}
              precision={1}
              suffix="%"
            />
          </Card>
        </Col>
        <Col span={5}>
          <Card size="small">
            <Statistic
              title={<ColTitle title="活跃 Skill 数" tip="该时间范围内有调用记录的不同 Capability 数量" />}
              value={data.active_skills}
            />
          </Card>
        </Col>
        <Col span={5}>
          <Card size="small">
            <Statistic
              title={<ColTitle title="高风险 Skill" tip="Schema 校验 fail 率超过 20% 的 Capability 数量" />}
              value={data.high_risk_skills ? data.high_risk_skills.length : 0}
              valueStyle={{ color: (data.high_risk_skills?.length || 0) > 0 ? '#cf1322' : undefined }}
            />
            {data.high_risk_skills && data.high_risk_skills.length > 0 && (
              <div style={{ fontSize: 12, color: '#8c8c8c', marginTop: 4 }}>
                {data.high_risk_skills.join(', ')}
              </div>
            )}
          </Card>
        </Col>
        <Col span={4}>
          <Card size="small">
            <Statistic
              title={<ColTitle title="总评估数" tip="该时间范围内完成的 Schema 校验总次数" />}
              value={totalAssessed}
            />
          </Card>
        </Col>
      </Row>

      <Row gutter={16}>
        <Col span={14}>
          <Card title={<ColTitle title="各 Capability 通过率" tip="按 AI 能力类型分组的 Schema 校验统计，包含通过率、平均分和平均耗时" />} className={styles.card}>
            <Table
              columns={capabilityColumns}
              dataSource={data.by_capability}
              pagination={false}
              size="small"
              rowKey="capability"
            />
          </Card>
        </Col>
        <Col span={10}>
          <Card title={<ColTitle title="评估结果分布" tip="Pass/Fail/Warn 的数量和占比" />} className={styles.card}>
            <div style={{ marginBottom: 16 }}>
              <div style={{ marginBottom: 4, fontSize: 13 }}>Pass <span style={{ color: '#8c8c8c' }}>({data.total_pass})</span></div>
              <Progress percent={passPercent} strokeColor="#52c41a" format={(pct) => `${pct}%`} />
            </div>
            <div style={{ marginBottom: 16 }}>
              <div style={{ marginBottom: 4, fontSize: 13 }}>Fail <span style={{ color: '#8c8c8c' }}>({data.total_fail})</span></div>
              <Progress percent={failPercent} strokeColor="#ff4d4f" format={(pct) => `${pct}%`} />
            </div>
            <div>
              <div style={{ marginBottom: 4, fontSize: 13 }}>Warn <span style={{ color: '#8c8c8c' }}>({data.total_warn})</span></div>
              <Progress percent={warnPercent} strokeColor="#faad14" format={(pct) => `${pct}%`} />
            </div>
          </Card>
        </Col>
      </Row>

      <Card title={<ColTitle title="每日评估趋势" tip="按调用日期聚合的每日 Schema 校验结果统计" />} className={styles.card}>
        <Table
          columns={trendColumns}
          dataSource={data.daily_trend}
          pagination={false}
          size="small"
          rowKey="date"
        />
      </Card>

      <Card
        title={<ColTitle title="最近失败记录" tip="最近 10 条 Schema 校验失败的记录，显示失败原因和 Skill 版本" />}
        className={styles.card}
        extra={data.recent_failures && data.recent_failures.length > 0 ? <Tag color="red">{data.recent_failures.length}</Tag> : null}
      >
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
      </Card>
    </div>
  );
};

export default SkillQualityDashboard;

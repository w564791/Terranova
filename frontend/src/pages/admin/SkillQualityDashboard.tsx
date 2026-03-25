import React, { useState, useEffect } from 'react';
import { Card, Row, Col, Statistic, Table, Tag, Progress, Segmented, Spin, Empty, Tooltip } from 'antd';
import type { ColumnsType } from 'antd/es/table';
import { getAssessmentOverview, AssessmentOverview, CapabilityStats, RecentFailure, DailyTrendItem } from '../../services/skillAssessment';
import styles from './SkillQualityDashboard.module.css';

const timeRangeMap: Record<string, number> = {
  '24h': 1,
  '7天': 7,
  '30天': 30,
};

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
      title: 'Capability',
      dataIndex: 'capability',
      key: 'capability',
    },
    {
      title: 'Total',
      dataIndex: 'total',
      key: 'total',
    },
    {
      title: 'Pass',
      dataIndex: 'pass',
      key: 'pass',
      render: (val: number) => <span style={{ color: '#52c41a' }}>{val}</span>,
    },
    {
      title: 'Fail',
      dataIndex: 'fail',
      key: 'fail',
      render: (val: number) => <span style={{ color: '#ff4d4f' }}>{val}</span>,
    },
    {
      title: 'Pass Rate',
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
      title: 'Status',
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
      title: '时间',
      dataIndex: 'assessed_at',
      key: 'assessed_at',
      render: (val: string) => {
        const d = new Date(val);
        return d.toLocaleString('zh-CN');
      },
    },
    {
      title: 'Capability',
      dataIndex: 'capability',
      key: 'capability',
    },
    {
      title: 'Skill Name',
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
      title: '问题',
      key: 'issues',
      render: (_: unknown, record: RecentFailure) => {
        const issues: string[] = [];
        if (record.missing_fields && record.missing_fields.length > 0) {
          issues.push(`缺失字段: ${record.missing_fields.join(', ')}`);
        }
        if (record.invalid_enum_fields && record.invalid_enum_fields.length > 0) {
          issues.push(`枚举无效: ${record.invalid_enum_fields.join(', ')}`);
        }
        return (
          <Tooltip title={issues.join('; ')}>
            <span>{issues.join('; ').substring(0, 60)}{issues.join('; ').length > 60 ? '...' : ''}</span>
          </Tooltip>
        );
      },
    },
    {
      title: 'Content Hash',
      dataIndex: 'content_hash',
      key: 'content_hash',
      render: (val: string) => (
        <Tooltip title={val}>
          <span style={{ fontFamily: 'monospace' }}>{val ? val.substring(0, 12) + '...' : '-'}</span>
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
      title: 'Pass',
      dataIndex: 'pass',
      key: 'pass',
      render: (val: number) => <span style={{ color: '#52c41a' }}>{val}</span>,
    },
    {
      title: 'Fail',
      dataIndex: 'fail',
      key: 'fail',
      render: (val: number) => <span style={{ color: '#ff4d4f' }}>{val}</span>,
    },
    {
      title: 'Warn',
      dataIndex: 'warn',
      key: 'warn',
      render: (val: number) => <span style={{ color: '#faad14' }}>{val}</span>,
    },
    {
      title: 'Total',
      key: 'total',
      render: (_: unknown, record: DailyTrendItem) => record.pass + record.fail + record.warn,
    },
    {
      title: 'Pass Rate',
      key: 'pass_rate',
      render: (_: unknown, record: DailyTrendItem) => {
        const total = record.pass + record.fail + record.warn;
        if (total === 0) return '-';
        return ((record.pass / total) * 100).toFixed(1) + '%';
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
            <Statistic title="Schema 通过率" value={data.pass_rate} precision={1} suffix="%" />
          </Card>
        </Col>
        <Col span={5}>
          <Card size="small">
            <Statistic title="评估覆盖率" value={parseFloat(coverage)} precision={1} suffix="%" />
          </Card>
        </Col>
        <Col span={5}>
          <Card size="small">
            <Statistic title="活跃 Skill 数" value={data.active_skills} />
          </Card>
        </Col>
        <Col span={5}>
          <Card size="small">
            <Statistic
              title="高风险 Skill"
              value={data.high_risk_skills ? data.high_risk_skills.length : 0}
              valueStyle={{ color: '#cf1322' }}
            />
          </Card>
        </Col>
        <Col span={4}>
          <Card size="small">
            <Statistic title="总评估数" value={totalAssessed} />
          </Card>
        </Col>
      </Row>

      <Row gutter={16}>
        <Col span={12}>
          <Card title="各 Capability 通过率" className={styles.card}>
            <Table
              columns={capabilityColumns}
              dataSource={data.by_capability}
              pagination={false}
              size="small"
              rowKey="capability"
            />
          </Card>
        </Col>
        <Col span={12}>
          <Card title="评估结果分布" className={styles.card}>
            <div style={{ marginBottom: 16 }}>
              <div style={{ marginBottom: 8 }}>Pass ({data.total_pass})</div>
              <Progress percent={passPercent} strokeColor="#52c41a" />
            </div>
            <div style={{ marginBottom: 16 }}>
              <div style={{ marginBottom: 8 }}>Fail ({data.total_fail})</div>
              <Progress percent={failPercent} strokeColor="#ff4d4f" />
            </div>
            <div>
              <div style={{ marginBottom: 8 }}>Warn ({data.total_warn})</div>
              <Progress percent={warnPercent} strokeColor="#faad14" />
            </div>
          </Card>
        </Col>
      </Row>

      <Card title="每日评估趋势" className={styles.card}>
        <Table
          columns={trendColumns}
          dataSource={data.daily_trend}
          pagination={false}
          size="small"
          rowKey="date"
        />
      </Card>

      <Card title="最近失败记录" className={styles.card}>
        <Table
          columns={failureColumns}
          dataSource={data.recent_failures}
          pagination={false}
          size="small"
          rowKey="usage_log_id"
        />
      </Card>
    </div>
  );
};

export default SkillQualityDashboard;

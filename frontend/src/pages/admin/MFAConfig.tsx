import React, { useState, useEffect } from 'react';
import { Card, Form, Switch, Select, InputNumber, Input, Button, Progress, Statistic, Row, Col, Alert, Spin, message, Divider, Typography } from 'antd';
import { SafetyCertificateOutlined, UserOutlined, SettingOutlined } from '@ant-design/icons';
import { getMFAConfig, updateMFAConfig } from '../../services/mfaService';
import type { MFAConfig, MFAStatistics } from '../../services/mfaService';
import styles from './MFAConfig.module.css';

const { Title, Text, Paragraph } = Typography;
const { Option } = Select;

const MFAConfigPage: React.FC = () => {
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [config, setConfig] = useState<MFAConfig | null>(null);
  const [statistics, setStatistics] = useState<MFAStatistics | null>(null);
  const [form] = Form.useForm();

  useEffect(() => {
    loadConfig();
  }, []);

  const loadConfig = async () => {
    try {
      setLoading(true);
      const response: any = await getMFAConfig();
      setConfig(response.data.config);
      setStatistics(response.data.statistics);
      form.setFieldsValue(response.data.config);
    } catch (error: any) {
      message.error(error || '加载MFA配置失败');
    } finally {
      setLoading(false);
    }
  };

  const handleSave = async (values: Partial<MFAConfig>) => {
    try {
      setSaving(true);
      await updateMFAConfig(values);
      message.success('MFA配置已更新');
      loadConfig();
    } catch (error: any) {
      message.error(error || '保存MFA配置失败');
    } finally {
      setSaving(false);
    }
  };

  const getEnforcementDescription = (enforcement: string) => {
    switch (enforcement) {
      case 'optional':
        return '用户可以自行选择是否启用MFA，不强制要求。';
      case 'required_new':
        return '策略启用后创建的新用户必须设置MFA，存量用户可选。';
      case 'required_all':
        return '所有用户都必须启用MFA，存量用户有宽限期。';
      default:
        return '';
    }
  };

  if (loading) {
    return (
      <div className={styles.container}>
        <div className={styles.loading}>
          <Spin size="large" />
          <Text>加载中...</Text>
        </div>
      </div>
    );
  }

  const mfaPercentage = statistics ? Math.round((statistics.mfa_enabled_users / statistics.total_users) * 100) || 0 : 0;

  return (
    <div className={styles.container}>
      <div className={styles.header}>
        <SafetyCertificateOutlined className={styles.headerIcon} />
        <div>
          <Title level={2} style={{ margin: 0 }}>MFA安全设置</Title>
          <Paragraph type="secondary">
            配置多因素认证策略，增强平台安全性
          </Paragraph>
        </div>
      </div>

      {/* 统计信息 */}
      {statistics && (
        <Card className={styles.statsCard}>
          <Row gutter={24}>
            <Col span={6}>
              <Statistic
                title="总用户数"
                value={statistics.total_users}
                prefix={<UserOutlined />}
              />
            </Col>
            <Col span={6}>
              <Statistic
                title="已启用MFA"
                value={statistics.mfa_enabled_users}
                valueStyle={{ color: '#52c41a' }}
              />
            </Col>
            <Col span={6}>
              <Statistic
                title="未启用MFA"
                value={statistics.mfa_pending_users}
                valueStyle={{ color: statistics.mfa_pending_users > 0 ? '#faad14' : '#52c41a' }}
              />
            </Col>
            <Col span={6}>
              <div className={styles.progressSection}>
                <Text type="secondary">MFA覆盖率</Text>
                <Progress
                  percent={mfaPercentage}
                  status={mfaPercentage >= 80 ? 'success' : mfaPercentage >= 50 ? 'normal' : 'exception'}
                  strokeColor={mfaPercentage >= 80 ? '#52c41a' : mfaPercentage >= 50 ? '#1890ff' : '#ff4d4f'}
                />
              </div>
            </Col>
          </Row>
        </Card>
      )}

      {/* 配置表单 */}
      <Card title={<><SettingOutlined /> MFA配置</>} className={styles.configCard}>
        <Form
          form={form}
          layout="vertical"
          onFinish={handleSave}
          initialValues={config || {}}
        >
          <Form.Item
            name="enabled"
            label="启用MFA功能"
            valuePropName="checked"
          >
            <Switch checkedChildren="开启" unCheckedChildren="关闭" />
          </Form.Item>

          <Divider />

          <Form.Item
            name="enforcement"
            label="MFA强制策略"
            extra={
              <Alert
                message={getEnforcementDescription(form.getFieldValue('enforcement'))}
                type="info"
                showIcon
                style={{ marginTop: 8 }}
              />
            }
          >
            <Select style={{ width: 300 }} onChange={() => form.validateFields(['enforcement'])}>
              <Option value="optional">可选 - 用户自行决定</Option>
              <Option value="required_new">新用户必须 - 渐进式推广</Option>
              <Option value="required_all">所有用户必须 - 最高安全</Option>
            </Select>
          </Form.Item>

          <Form.Item
            noStyle
            shouldUpdate={(prevValues, currentValues) => prevValues.enforcement !== currentValues.enforcement}
          >
            {({ getFieldValue }) =>
              getFieldValue('enforcement') === 'required_all' && (
                <Form.Item
                  name="grace_period_days"
                  label="宽限期（天）"
                  extra="存量用户在此期间可正常登录但会收到设置MFA的提醒"
                >
                  <InputNumber min={1} max={90} style={{ width: 120 }} />
                </Form.Item>
              )
            }
          </Form.Item>

          <Divider />

          <Form.Item
            name="issuer"
            label="发行者名称"
            extra="显示在Authenticator应用中的账户名称"
          >
            <Input style={{ width: 300 }} placeholder="IaC Platform" />
          </Form.Item>

          <Divider />

          <Title level={5}>安全设置</Title>

          <Row gutter={24}>
            <Col span={12}>
              <Form.Item
                name="max_failed_attempts"
                label="最大失败尝试次数"
                extra="超过此次数后账户将被临时锁定"
              >
                <InputNumber min={3} max={10} style={{ width: 120 }} />
              </Form.Item>
            </Col>
            <Col span={12}>
              <Form.Item
                name="lockout_duration_minutes"
                label="锁定时长（分钟）"
                extra="账户被锁定后的等待时间"
              >
                <InputNumber min={5} max={60} style={{ width: 120 }} />
              </Form.Item>
            </Col>
          </Row>

          <Divider />

          <Form.Item>
            <Button type="primary" htmlType="submit" loading={saving}>
              保存配置
            </Button>
            <Button style={{ marginLeft: 8 }} onClick={loadConfig}>
              重置
            </Button>
          </Form.Item>
        </Form>
      </Card>

      {/* 说明信息 */}
      <Card className={styles.infoCard}>
        <Title level={5}>关于MFA</Title>
        <Paragraph>
          多因素认证（MFA）通过要求用户提供两种或更多验证因素来增强账户安全性。
          本平台支持基于时间的一次性密码（TOTP）标准，兼容以下应用：
        </Paragraph>
        <ul>
          <li>Google Authenticator</li>
          <li>Microsoft Authenticator</li>
          <li>Authy</li>
          <li>其他支持TOTP标准的应用</li>
        </ul>
        <Paragraph type="secondary">
          建议在生产环境中启用MFA强制策略，以确保所有用户账户的安全。
        </Paragraph>
      </Card>
    </div>
  );
};

export default MFAConfigPage;
import React, { useState, useEffect } from 'react';
import { Card, Form, Switch, TimePicker, InputNumber, Button, message, Spin, Space, Typography, Alert, Tooltip } from 'antd';
import { SyncOutlined, ClockCircleOutlined, SettingOutlined, InfoCircleOutlined } from '@ant-design/icons';
import dayjs from 'dayjs';
import customParseFormat from 'dayjs/plugin/customParseFormat';
import { getDriftConfig, updateDriftConfig, triggerDriftCheck, getDriftStatus, cancelDriftCheck } from '../../services/drift';

// 启用 customParseFormat 插件以支持自定义时间格式解析
dayjs.extend(customParseFormat);
import type { DriftConfig, DriftResult } from '../../services/drift';
import styles from './DriftConfig.module.css';

const { Text, Title } = Typography;

interface DriftConfigProps {
  workspaceId: string;
}

const DriftConfigComponent: React.FC<DriftConfigProps> = ({ workspaceId }) => {
  const [form] = Form.useForm();
  const [loading, setLoading] = useState(false);
  const [saving, setSaving] = useState(false);
  const [triggering, setTriggering] = useState(false);
  const [cancelling, setCancelling] = useState(false);
  const [config, setConfig] = useState<DriftConfig | null>(null);
  const [driftStatus, setDriftStatus] = useState<DriftResult | null>(null);

  // 检查是否有正在运行的检测任务
  const isCheckRunning = driftStatus?.check_status === 'pending' || driftStatus?.check_status === 'running';
  
  // 调试日志
  console.log('[DriftConfig] driftStatus:', driftStatus);
  console.log('[DriftConfig] check_status:', driftStatus?.check_status);
  console.log('[DriftConfig] isCheckRunning:', isCheckRunning);

  // 加载配置
  const loadConfig = async () => {
    setLoading(true);
    try {
      console.log('[DriftConfig] Loading config and status for workspace:', workspaceId);
      
      const [configData, statusData] = await Promise.all([
        getDriftConfig(workspaceId).catch((err) => {
          console.error('[DriftConfig] getDriftConfig error:', err);
          console.error('[DriftConfig] getDriftConfig error response:', err?.response?.data);
          return null;
        }),
        getDriftStatus(workspaceId).catch((err) => {
          console.error('[DriftConfig] getDriftStatus error:', err);
          console.error('[DriftConfig] getDriftStatus error response:', err?.response?.data);
          return null;
        })
      ]);
      
      console.log('[DriftConfig] Loaded config:', configData);
      console.log('[DriftConfig] Loaded status:', statusData);
      console.log('[DriftConfig] statusData check_status:', statusData?.check_status);
      
      setDriftStatus(statusData);
      
      if (!configData) {
        console.warn('[DriftConfig] Config data is null, using defaults');
        setConfig({
          drift_check_enabled: false,
          drift_check_start_time: '07:00',
          drift_check_end_time: '22:00',
          drift_check_interval: 1440,
          continue_on_failure: false,
          continue_on_success: false,
        });
      } else {
        setConfig(configData);
      }
    } catch (error) {
      console.error('[DriftConfig] Load error:', error);
      message.error('加载 Drift 配置失败');
    } finally {
      setLoading(false);
    }
  };

  // 当 config 变化时更新表单
  useEffect(() => {
    if (config) {
      // 数据库返回的时间格式可能是 "07:00:00" 或 "07:00"，需要兼容处理
      const parseTime = (timeStr: string | null | undefined) => {
        if (!timeStr) return null;
        const formats = ['HH:mm:ss', 'HH:mm'];
        for (const fmt of formats) {
          const parsed = dayjs(timeStr, fmt);
          if (parsed.isValid()) return parsed;
        }
        return null;
      };
      
      const formValues = {
        drift_check_enabled: config.drift_check_enabled === true,
        drift_check_start_time: parseTime(config.drift_check_start_time),
        drift_check_end_time: parseTime(config.drift_check_end_time),
        drift_check_interval: config.drift_check_interval || 60,
        continue_on_failure: config.continue_on_failure || false,
        continue_on_success: config.continue_on_success || false,
      };
      
      console.log('[DriftConfig] Setting form values from config:', formValues);
      form.setFieldsValue(formValues);
    }
  }, [config, form]);

  useEffect(() => {
    loadConfig();
  }, [workspaceId]);

  // 当有检测任务运行时，定时轮询状态
  useEffect(() => {
    if (!isCheckRunning) return;
    
    console.log('[DriftConfig] Starting polling for drift check status...');
    
    const interval = setInterval(async () => {
      try {
        const newStatus = await getDriftStatus(workspaceId);
        console.log('[DriftConfig] Polling result:', newStatus?.check_status);
        setDriftStatus(newStatus);
        
        // 如果检测完成，停止轮询并显示结果
        if (newStatus?.check_status === 'success' || newStatus?.check_status === 'failed') {
          console.log('[DriftConfig] Drift check completed:', newStatus?.check_status);
          if (newStatus?.has_drift) {
            message.warning(`检测到 ${newStatus.drift_count} 个资源存在 Drift`);
          } else {
            message.success('Drift 检测完成，未发现配置漂移');
          }
        }
      } catch (err) {
        console.error('[DriftConfig] Polling error:', err);
      }
    }, 3000); // 每 3 秒轮询一次
    
    return () => {
      console.log('[DriftConfig] Stopping polling');
      clearInterval(interval);
    };
  }, [isCheckRunning, workspaceId]);

  // 保存配置
  const handleSave = async (values: Record<string, unknown>) => {
    setSaving(true);
    try {
      const configData: DriftConfig = {
        drift_check_enabled: values.drift_check_enabled as boolean,
        drift_check_start_time: values.drift_check_start_time ? (values.drift_check_start_time as dayjs.Dayjs).format('HH:mm') : '',
        drift_check_end_time: values.drift_check_end_time ? (values.drift_check_end_time as dayjs.Dayjs).format('HH:mm') : '',
        drift_check_interval: values.drift_check_interval as number,
        continue_on_failure: values.continue_on_failure as boolean || false,
        continue_on_success: values.continue_on_success as boolean || false,
      };
      await updateDriftConfig(workspaceId, configData);
      message.success('Drift 配置已保存');
      // 重新加载配置以获取最新状态
      await loadConfig();
    } catch (error) {
      message.error('保存 Drift 配置失败');
    } finally {
      setSaving(false);
    }
  };

  // 手动触发检测
  const handleTriggerCheck = async () => {
    setTriggering(true);
    try {
      await triggerDriftCheck(workspaceId);
      message.success('Drift 检测已触发，请稍后查看结果');
      // 立即刷新状态，触发轮询
      const newStatus = await getDriftStatus(workspaceId);
      console.log('[DriftConfig] Status after trigger:', newStatus);
      setDriftStatus(newStatus);
    } catch (error: unknown) {
      const err = error as { response?: { data?: { error?: string } } };
      message.error(err.response?.data?.error || '触发 Drift 检测失败');
    } finally {
      setTriggering(false);
    }
  };

  // 取消检测
  const handleCancelCheck = async () => {
    setCancelling(true);
    try {
      await cancelDriftCheck(workspaceId);
      message.success('Drift 检测已取消');
      // 刷新状态
      const newStatus = await getDriftStatus(workspaceId);
      setDriftStatus(newStatus);
    } catch (error: unknown) {
      const err = error as { response?: { data?: { error?: string } } };
      message.error(err.response?.data?.error || '取消 Drift 检测失败');
    } finally {
      setCancelling(false);
    }
  };

  if (loading) {
    return (
      <Card>
        <div style={{ textAlign: 'center', padding: '40px' }}>
          <Spin size="large" />
        </div>
      </Card>
    );
  }

  return (
    <div className={styles.container}>
      <Card>
        <Form
          form={form}
          layout="vertical"
          onFinish={handleSave}
          initialValues={{
            drift_check_enabled: false,
            drift_check_interval: 60,
          }}
        >
          <Form.Item
            name="drift_check_enabled"
            label="启用 Drift 检测"
            valuePropName="checked"
          >
            <Switch 
              checkedChildren="开启" 
              unCheckedChildren="关闭"
              onChange={(checked) => {
                // 立即保存启用状态
                const currentValues = form.getFieldsValue();
                handleSave({
                  ...currentValues,
                  drift_check_enabled: checked,
                });
              }}
            />
          </Form.Item>

          <Form.Item noStyle shouldUpdate={(prevValues, currentValues) => prevValues.drift_check_enabled !== currentValues.drift_check_enabled}>
            {({ getFieldValue }) => {
              const isEnabled = getFieldValue('drift_check_enabled');
              return (
                <>
                  <div style={{ opacity: isEnabled ? 1 : 0.5, pointerEvents: isEnabled ? 'auto' : 'none' }}>
                    <Title level={5}>
                      <ClockCircleOutlined /> 检测时间窗口
                    </Title>
                    <Text type="secondary" style={{ display: 'block', marginBottom: 16 }}>
                      设置每天允许执行 Drift 检测的时间范围，建议设置在业务低峰期。
                      <br />
                      <Text type="warning">时区: 服务器本地时间 (UTC+8)</Text>
                    </Text>

                    <Space size="large">
                      <Form.Item
                        name="drift_check_start_time"
                        label="开始时间"
                        rules={[{ required: isEnabled, message: '请选择开始时间' }]}
                      >
                        <TimePicker format="HH:mm" placeholder="选择开始时间" disabled={!isEnabled} />
                      </Form.Item>

                      <Form.Item
                        name="drift_check_end_time"
                        label="结束时间"
                        rules={[{ required: isEnabled, message: '请选择结束时间' }]}
                      >
                        <TimePicker format="HH:mm" placeholder="选择结束时间" disabled={!isEnabled} />
                      </Form.Item>
                    </Space>

                    <Form.Item
                      name="drift_check_interval"
                      label="检测间隔（分钟）"
                      rules={[
                        { required: true, message: '请输入检测间隔' },
                        { type: 'number', min: 30, message: '最小间隔为 30 分钟' },
                      ]}
                    >
                      <InputNumber
                        min={30}
                        max={1440}
                        step={30}
                        style={{ width: 200 }}
                        addonAfter="分钟"
                        disabled={!isEnabled}
                      />
                    </Form.Item>

                    {/* 继续检测设置 */}
                    <div style={{ marginTop: 24, marginBottom: 16 }}>
                      <Title level={5}>
                        <SettingOutlined /> 继续检测设置
                      </Title>

                      <Space direction="vertical" size="middle" style={{ width: '100%' }}>
                        <Form.Item
                          name="continue_on_failure"
                          label={
                            <span>
                              失败后继续
                              <Tooltip title="当 Drift 检测失败时，是否在下一个检测周期继续尝试检测">
                                <InfoCircleOutlined style={{ marginLeft: 8, color: '#999' }} />
                              </Tooltip>
                            </span>
                          }
                          valuePropName="checked"
                          style={{ marginBottom: 8 }}
                        >
                          <Switch 
                            checkedChildren="继续" 
                            unCheckedChildren="停止"
                            disabled={!isEnabled}
                          />
                        </Form.Item>

                        <Form.Item
                          name="continue_on_success"
                          label={
                            <span>
                              成功后继续
                              <Tooltip title="当 Drift 检测成功时，是否在下一个检测周期继续检测">
                                <InfoCircleOutlined style={{ marginLeft: 8, color: '#999' }} />
                              </Tooltip>
                            </span>
                          }
                          valuePropName="checked"
                          style={{ marginBottom: 8 }}
                        >
                          <Switch 
                            checkedChildren="继续" 
                            unCheckedChildren="停止"
                            disabled={!isEnabled}
                          />
                        </Form.Item>
                      </Space>

                      <Text type="secondary" style={{ display: 'block', marginTop: 12 }}>
                        默认情况下，检测成功或失败后都会停止后续检测。
                        开启相应选项后，系统会按照检测间隔继续执行检测。
                      </Text>
                    </div>
                  </div>

                  <Form.Item>
                    <Button type="primary" htmlType="submit" loading={saving}>
                      保存配置
                    </Button>
                  </Form.Item>
                </>
              );
            }}
          </Form.Item>
        </Form>
      </Card>
    </div>
  );
};

export default DriftConfigComponent;

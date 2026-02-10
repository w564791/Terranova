import React, { useState, useEffect } from 'react';
import { Card, Form, Switch, Input, Button, Select, InputNumber, Table, Tag, Space, Popconfirm, Alert, Spin, message, Divider, Typography, Tabs } from 'antd';
import { SafetyCertificateOutlined, PlusOutlined, DeleteOutlined, EditOutlined, SettingOutlined, UnorderedListOutlined, CloseOutlined } from '@ant-design/icons';
import { ssoAdminService, type SSOProviderConfig, type SSOGlobalConfig, type SSOLoginLog } from '../../services/ssoService';
import styles from './SSOConfig.module.css';

const { Title, Text, Paragraph } = Typography;
const { Option } = Select;
const { TextArea } = Input;

const SSOConfigPage: React.FC = () => {
  const [loading, setLoading] = useState(true);
  const [providers, setProviders] = useState<SSOProviderConfig[]>([]);
  const [globalConfig, setGlobalConfig] = useState<SSOGlobalConfig>({ disable_local_login: false });
  const [savingConfig, setSavingConfig] = useState(false);

  // Provider 编辑状态（内联表单）
  const [showProviderForm, setShowProviderForm] = useState(false);
  const [editingProvider, setEditingProvider] = useState<SSOProviderConfig | null>(null);
  const [savingProvider, setSavingProvider] = useState(false);

  // 日志
  const [logs, setLogs] = useState<SSOLoginLog[]>([]);
  const [logsTotal, setLogsTotal] = useState(0);
  const [logsPage, setLogsPage] = useState(1);
  const [logsLoading, setLogsLoading] = useState(false);

  const [form] = Form.useForm();
  // 用于存储编辑时从详情 API 获取的完整数据
  const [editingDetail, setEditingDetail] = useState<any>(null);

  useEffect(() => {
    loadData();
  }, []);

  // 当编辑详情数据变化时，填充表单
  useEffect(() => {
    if (!editingDetail || !showProviderForm) return;

    let oauthData: any = {};
    try {
      const rawConfig = editingDetail.oauth_config;
      if (typeof rawConfig === 'string') {
        oauthData = JSON.parse(rawConfig);
      } else if (rawConfig && typeof rawConfig === 'object') {
        oauthData = rawConfig;
      }
    } catch { /* ignore */ }

    form.setFieldsValue({
      provider_key: editingDetail.provider_key || '',
      provider_type: editingDetail.provider_type || 'github',
      display_name: editingDetail.display_name || '',
      description: editingDetail.description || '',
      icon: editingDetail.icon || '',
      callback_url: editingDetail.callback_url || '',
      auto_create_user: editingDetail.auto_create_user ?? true,
      default_role: editingDetail.default_role || 'user',
      is_enabled: editingDetail.is_enabled ?? true,
      show_on_login_page: editingDetail.show_on_login_page ?? true,
      display_order: editingDetail.display_order || 0,
      oauth_client_id: oauthData.client_id || '',
      oauth_client_secret: '',
      oauth_domain: oauthData.domain || '',
      oauth_tenant_id: oauthData.tenant_id || '',
      oauth_org_url: oauthData.org_url || '',
      oauth_audience: oauthData.audience || '',
      oauth_scopes: Array.isArray(oauthData.scopes) ? oauthData.scopes.join(', ') : (oauthData.scopes || ''),
    });
  }, [editingDetail, showProviderForm, form]);

  const loadData = async () => {
    try {
      setLoading(true);
      const [providersRes, configRes]: any[] = await Promise.all([
        ssoAdminService.getProviders(),
        ssoAdminService.getConfig(),
      ]);
      setProviders((providersRes.data || providersRes) || []);
      const cfgData = configRes.data || configRes;
      setGlobalConfig(cfgData || { disable_local_login: false });
    } catch (error: any) {
      message.error('加载SSO配置失败');
    } finally {
      setLoading(false);
    }
  };

  const loadLogs = async (page: number = 1) => {
    try {
      setLogsLoading(true);
      const res: any = await ssoAdminService.getLogs(page, 20);
      const data = res.data || res;
      setLogs(data.items || []);
      setLogsTotal(data.total || 0);
      setLogsPage(page);
    } catch (error: any) {
      message.error('加载登录日志失败');
    } finally {
      setLogsLoading(false);
    }
  };

  const handleSaveConfig = async () => {
    try {
      setSavingConfig(true);
      await ssoAdminService.updateConfig(globalConfig);
      message.success('SSO全局配置已更新');
    } catch (error: any) {
      message.error('保存配置失败');
    } finally {
      setSavingConfig(false);
    }
  };

  // 根据 provider_key 自动生成回调 URL（指向前端 SSOCallback 页面）
  const generateCallbackUrl = (_providerKey: string) => {
    return `${window.location.origin}/sso/callback`;
  };

  // 打开添加表单
  const openAddForm = () => {
    setEditingProvider(null);
    form.resetFields();
    form.setFieldsValue({
      provider_type: 'github',
      provider_key: '',
      callback_url: '',
      auto_create_user: true,
      default_role: 'user',
      is_enabled: true,
      show_on_login_page: true,
      display_order: 0,
    });
    setShowProviderForm(true);
  };

  // 打开编辑表单（先调用详情 API 获取完整数据）
  const openEditForm = async (provider: any) => {
    if (!provider.id) return;
    try {
      const res: any = await ssoAdminService.getProvider(provider.id);
      const detail = res.data || res;
      setEditingProvider(detail);
      setEditingDetail(detail);
      setShowProviderForm(true);
    } catch (error: any) {
      message.error('获取 Provider 详情失败');
    }
  };

  // 关闭表单
  const closeForm = () => {
    setShowProviderForm(false);
    setEditingProvider(null);
    setEditingDetail(null);
    form.resetFields();
  };

  // 保存 Provider（将表单中的 oauth_* 字段组装为 oauth_config JSON）
  const handleSaveProvider = async () => {
    try {
      const values = await form.validateFields();
      setSavingProvider(true);

      // 组装 oauth_config JSON
      const oauthConfig: Record<string, any> = {
        client_id: values.oauth_client_id || '',
        client_secret_encrypted: values.oauth_client_secret || '',
      };
      if (values.oauth_domain) oauthConfig.domain = values.oauth_domain;
      if (values.oauth_tenant_id) oauthConfig.tenant_id = values.oauth_tenant_id;
      if (values.oauth_org_url) oauthConfig.org_url = values.oauth_org_url;
      if (values.oauth_audience) oauthConfig.audience = values.oauth_audience;
      if (values.oauth_scopes) {
        oauthConfig.scopes = values.oauth_scopes.split(',').map((s: string) => s.trim()).filter(Boolean);
      }

      // 移除 oauth_* 临时字段，添加 oauth_config
      const submitData = { ...values };
      delete submitData.oauth_client_id;
      delete submitData.oauth_client_secret;
      delete submitData.oauth_domain;
      delete submitData.oauth_tenant_id;
      delete submitData.oauth_org_url;
      delete submitData.oauth_audience;
      delete submitData.oauth_scopes;
      submitData.oauth_config = JSON.stringify(oauthConfig);

      if (editingProvider?.id) {
        await ssoAdminService.updateProvider(editingProvider.id, submitData);
        message.success('Provider 已更新');
      } else {
        await ssoAdminService.createProvider(submitData);
        message.success('Provider 已创建');
      }
      closeForm();
      loadData();
    } catch (error: any) {
      if (error.errorFields) return;
      message.error(error.message || '保存失败');
    } finally {
      setSavingProvider(false);
    }
  };

  const handleDeleteProvider = async (id: number) => {
    try {
      await ssoAdminService.deleteProvider(id);
      message.success('Provider 已删除');
      loadData();
    } catch (error: any) {
      message.error('删除失败');
    }
  };

  const handleToggleProvider = async (id: number, enabled: boolean) => {
    try {
      await ssoAdminService.updateProvider(id, { is_enabled: enabled } as any);
      message.success(enabled ? 'Provider 已启用' : 'Provider 已禁用');
      loadData();
    } catch (error: any) {
      message.error('操作失败');
    }
  };

  const providerTypeOptions = [
    { value: 'github', label: 'GitHub' },
    { value: 'google', label: 'Google' },
    { value: 'auth0', label: 'Auth0' },
    { value: 'azure_ad', label: 'Azure AD' },
    { value: 'okta', label: 'Okta' },
    { value: 'oidc', label: '通用 OIDC' },
  ];

  const providerColumns = [
    {
      title: '名称',
      dataIndex: 'display_name',
      key: 'display_name',
      render: (text: string, record: SSOProviderConfig) => (
        <Space>
          <Text strong>{text}</Text>
          <Tag color="blue">{record.provider_type}</Tag>
        </Space>
      ),
    },
    {
      title: 'Key',
      dataIndex: 'provider_key',
      key: 'provider_key',
      render: (text: string) => <code>{text}</code>,
    },
    {
      title: '状态',
      dataIndex: 'is_enabled',
      key: 'is_enabled',
      render: (enabled: boolean, record: SSOProviderConfig) => (
        <Switch
          checked={enabled}
          onChange={(checked) => handleToggleProvider(record.id!, checked)}
          checkedChildren="启用"
          unCheckedChildren="禁用"
          size="small"
        />
      ),
    },
    {
      title: '自动创建用户',
      dataIndex: 'auto_create_user',
      key: 'auto_create_user',
      render: (v: boolean) => v ? <Tag color="green">是</Tag> : <Tag>否</Tag>,
    },
    {
      title: '排序',
      dataIndex: 'display_order',
      key: 'display_order',
    },
    {
      title: '操作',
      key: 'actions',
      render: (_: any, record: SSOProviderConfig) => (
        <Space>
          <Button type="link" size="small" icon={<EditOutlined />} onClick={() => openEditForm(record)}>
            编辑
          </Button>
          <Popconfirm title="确定删除此 Provider？" onConfirm={() => handleDeleteProvider(record.id!)}>
            <Button type="link" size="small" danger icon={<DeleteOutlined />}>
              删除
            </Button>
          </Popconfirm>
        </Space>
      ),
    },
  ];

  const logColumns = [
    { title: '时间', dataIndex: 'created_at', key: 'created_at', width: 180,
      render: (v: string) => v ? new Date(v).toLocaleString() : '-' },
    { title: 'Provider', dataIndex: 'provider_key', key: 'provider_key', width: 100 },
    { title: '邮箱', dataIndex: 'provider_email', key: 'provider_email', width: 200 },
    { title: '用户ID', dataIndex: 'user_id', key: 'user_id', width: 140 },
    { title: '状态', dataIndex: 'status', key: 'status', width: 120,
      render: (s: string) => {
        const colorMap: Record<string, string> = { success: 'green', failed: 'red', user_created: 'blue', user_linked: 'cyan' };
        return <Tag color={colorMap[s] || 'default'}>{s}</Tag>;
      },
    },
    { title: '错误信息', dataIndex: 'error_message', key: 'error_message', ellipsis: true },
    { title: 'IP', dataIndex: 'ip_address', key: 'ip_address', width: 130 },
  ];

  if (loading) {
    return (
      <div className={styles.container}>
        <div className={styles.loading}><Spin size="large" /><Text>加载中...</Text></div>
      </div>
    );
  }

  return (
    <div className={styles.container}>
      <div className={styles.header}>
        <SafetyCertificateOutlined className={styles.headerIcon} />
        <div>
          <Title level={2} style={{ margin: 0 }}>SSO 单点登录配置</Title>
          <Paragraph type="secondary">
            管理 SSO 身份提供商、全局登录策略和登录日志
          </Paragraph>
        </div>
      </div>

      <Tabs defaultActiveKey="providers" items={[
        {
          key: 'providers',
          label: <><SettingOutlined /> Provider 管理</>,
          children: (
            <>
              {/* 全局配置 */}
              <Card title="全局登录策略" style={{ marginBottom: 16 }}>
                <Space direction="vertical" size="middle" style={{ width: '100%' }}>
                  <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
                    <div>
                      <Text strong>禁用本地密码登录</Text>
                      <br />
                      <Text type="secondary" style={{ fontSize: 12 }}>
                        启用后，普通用户只能通过 SSO 登录。超级管理员（is_system_admin）不受影响，始终可以使用密码登录。
                      </Text>
                    </div>
                    <Switch
                      checked={globalConfig.disable_local_login}
                      onChange={(checked) => setGlobalConfig({ ...globalConfig, disable_local_login: checked })}
                      checkedChildren="已禁用"
                      unCheckedChildren="未禁用"
                    />
                  </div>
                  {globalConfig.disable_local_login && (
                    <Alert
                      message="本地密码登录已禁用"
                      description="普通用户将无法使用密码登录，请确保至少配置了一个可用的 SSO Provider。超级管理员不受此限制。"
                      type="warning"
                      showIcon
                    />
                  )}
                  <Button type="primary" onClick={handleSaveConfig} loading={savingConfig}>
                    保存全局配置
                  </Button>
                </Space>
              </Card>

              {/* Provider 列表 */}
              <Card
                title="SSO Provider 列表"
                extra={
                  !showProviderForm && (
                    <Button type="primary" icon={<PlusOutlined />} onClick={openAddForm}>
                      添加 Provider
                    </Button>
                  )
                }
              >
                {providers.length === 0 && !showProviderForm && (
                  <Alert
                    message="尚未配置任何 SSO Provider"
                    description="点击「添加 Provider」按钮配置 GitHub、Google、Azure AD 等身份提供商。"
                    type="info"
                    showIcon
                    style={{ marginBottom: 16 }}
                  />
                )}

                {providers.length > 0 && (
                  <Table
                    dataSource={providers}
                    columns={providerColumns}
                    rowKey="id"
                    pagination={false}
                    size="middle"
                    style={{ marginBottom: showProviderForm ? 24 : 0 }}
                  />
                )}

                {/* 内联 Provider 表单 */}
                {showProviderForm && (
                  <div className={styles.providerForm}>
                    <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 16 }}>
                      <Title level={5} style={{ margin: 0 }}>
                        {editingProvider ? `编辑 Provider: ${editingProvider.provider_key}` : '添加新 Provider'}
                      </Title>
                      <Button type="text" icon={<CloseOutlined />} onClick={closeForm} />
                    </div>

                    <Form form={form} layout="vertical">
                      <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '0 24px' }}>
                        <Form.Item name="provider_key" label="Provider Key" rules={[{ required: true, message: '请输入唯一标识' }]}
                          extra="唯一标识，用于 API 路由，如 github, google">
                          <Input placeholder="github" disabled={!!editingProvider}
                            onChange={(e) => {
                              const key = e.target.value;
                              if (key && !editingProvider) {
                                form.setFieldValue('callback_url', generateCallbackUrl(key));
                              }
                            }}
                          />
                        </Form.Item>

                        <Form.Item name="provider_type" label="Provider 类型" rules={[{ required: true }]}>
                          <Select options={providerTypeOptions} />
                        </Form.Item>

                        <Form.Item name="display_name" label="显示名称" rules={[{ required: true, message: '请输入显示名称' }]}>
                          <Input placeholder="使用 GitHub 登录" />
                        </Form.Item>

                        <Form.Item name="icon" label="图标" extra="可选值: github, google, microsoft, auth0">
                          <Input placeholder="github" />
                        </Form.Item>

                        <Form.Item name="callback_url" label="回调 URL" rules={[{ required: true, message: '请输入回调 URL' }]}
                          extra="此 URL 需要同时配置在 Provider 端（如 GitHub OAuth App 的 Authorization callback URL）"
                          style={{ gridColumn: '1 / -1' }}>
                          <Input placeholder="http://localhost:8080/api/v1/auth/sso/github/callback" />
                        </Form.Item>

                        <Form.Item name="oauth_client_id" label="Client ID" rules={[{ required: true, message: '请输入 Client ID' }]}>
                          <Input placeholder="OAuth App 的 Client ID" />
                        </Form.Item>

                        <Form.Item name="oauth_client_secret"
                          label={editingProvider ? 'Client Secret（留空不修改）' : 'Client Secret'}
                          rules={editingProvider ? [] : [{ required: true, message: '请输入 Client Secret' }]}
                          extra={editingProvider ? '留空表示保持原有密钥不变' : '输入 OAuth App 的 Client Secret'}>
                          <Input.Password placeholder={editingProvider ? '留空不修改' : 'Client Secret'} />
                        </Form.Item>
                      </div>

                      {/* 根据 Provider 类型显示额外字段 */}
                      <Form.Item noStyle shouldUpdate={(prev, cur) => prev.provider_type !== cur.provider_type}>
                        {({ getFieldValue }) => {
                          const providerType = getFieldValue('provider_type');
                          return (
                            <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '0 24px' }}>
                              {providerType === 'auth0' && (
                                <>
                                  <Form.Item name="oauth_domain" label="Auth0 Domain" rules={[{ required: true, message: '请输入 Auth0 Domain' }]}
                                    extra="如 your-tenant.auth0.com">
                                    <Input placeholder="your-tenant.auth0.com" />
                                  </Form.Item>
                                  <Form.Item name="oauth_audience" label="Audience（可选）"
                                    extra="Auth0 API Audience">
                                    <Input placeholder="https://your-api.example.com" />
                                  </Form.Item>
                                </>
                              )}
                              {providerType === 'azure_ad' && (
                                <Form.Item name="oauth_tenant_id" label="Tenant ID" rules={[{ required: true, message: '请输入 Azure AD Tenant ID' }]}
                                  extra="Azure AD 租户 ID，如 common 或具体的 GUID">
                                  <Input placeholder="common 或 xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx" />
                                </Form.Item>
                              )}
                              {providerType === 'okta' && (
                                <Form.Item name="oauth_org_url" label="Okta Org URL" rules={[{ required: true, message: '请输入 Okta Org URL' }]}
                                  extra="如 your-org.okta.com">
                                  <Input placeholder="your-org.okta.com" />
                                </Form.Item>
                              )}
                              <Form.Item name="oauth_scopes" label="Scopes（可选）"
                                extra="逗号分隔，如 openid, profile, email"
                                style={{ gridColumn: providerType === 'github' || providerType === 'google' ? '1 / -1' : undefined }}>
                                <Input placeholder="openid, profile, email" />
                              </Form.Item>
                            </div>
                          );
                        }}
                      </Form.Item>

                      <Divider style={{ margin: '12px 0' }} />

                      <div style={{ display: 'flex', gap: 32, flexWrap: 'wrap', alignItems: 'center' }}>
                        <Form.Item name="auto_create_user" label="自动创建用户" valuePropName="checked" style={{ marginBottom: 0 }}>
                          <Switch />
                        </Form.Item>
                        <Form.Item name="is_enabled" label="启用" valuePropName="checked" style={{ marginBottom: 0 }}>
                          <Switch />
                        </Form.Item>
                        <Form.Item name="show_on_login_page" label="登录页显示" valuePropName="checked" style={{ marginBottom: 0 }}>
                          <Switch />
                        </Form.Item>
                        <Form.Item name="default_role" label="默认角色" style={{ marginBottom: 0 }}>
                          <Select style={{ width: 120 }}>
                            <Option value="user">user</Option>
                            <Option value="admin">admin</Option>
                          </Select>
                        </Form.Item>
                        <Form.Item name="display_order" label="排序" style={{ marginBottom: 0 }}>
                          <InputNumber min={0} max={100} style={{ width: 80 }} />
                        </Form.Item>
                      </div>

                      <Form.Item name="description" label="描述" style={{ marginTop: 16 }}>
                        <Input placeholder="可选描述" />
                      </Form.Item>

                      <div style={{ display: 'flex', gap: 8, marginTop: 8 }}>
                        <Button type="primary" onClick={handleSaveProvider} loading={savingProvider}>
                          {editingProvider ? '更新' : '创建'}
                        </Button>
                        <Button onClick={closeForm}>取消</Button>
                      </div>
                    </Form>
                  </div>
                )}
              </Card>
            </>
          ),
        },
        {
          key: 'logs',
          label: <><UnorderedListOutlined /> 登录日志</>,
          children: (
            <Card title="SSO 登录日志">
              <Table
                dataSource={logs}
                columns={logColumns}
                rowKey="id"
                loading={logsLoading}
                size="small"
                pagination={{
                  current: logsPage,
                  total: logsTotal,
                  pageSize: 20,
                  onChange: (page) => loadLogs(page),
                  showTotal: (total) => `共 ${total} 条`,
                }}
                locale={{ emptyText: '暂无登录日志' }}
              />
              {logs.length === 0 && !logsLoading && (
                <div style={{ textAlign: 'center', marginTop: 16 }}>
                  <Button onClick={() => loadLogs(1)}>加载日志</Button>
                </div>
              )}
            </Card>
          ),
        },
      ]} />
    </div>
  );
};

export default SSOConfigPage;
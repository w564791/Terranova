import React, { useState, useEffect } from 'react';
import { useNavigate, useSearchParams } from 'react-router-dom';
import {
  Form,
  Input,
  Button,
  Card,
  Typography,
  Space,
  Select,
  Tabs,
  Upload,
  message,
} from 'antd';
import { ArrowLeftOutlined, UploadOutlined, FileTextOutlined, PlusOutlined, ImportOutlined } from '@ant-design/icons';
import type { CreateManifestRequest } from '../../services/manifestApi';
import { createManifest, importManifestHCL, importManifestJSON } from '../../services/manifestApi';
import { iamService } from '../../services/iam';
import { useToast } from '../../contexts/ToastContext';
import styles from './ManifestCreate.module.css';

const { Title, Text } = Typography;
const { TextArea } = Input;

interface Organization {
  id: number;
  name: string;
  display_name?: string;
}

const ManifestCreate: React.FC = () => {
  const navigate = useNavigate();
  const [searchParams] = useSearchParams();
  const [form] = Form.useForm();
  const [importForm] = Form.useForm();
  const [creating, setCreating] = useState(false);
  const [importing, setImporting] = useState(false);
  const [organizations, setOrganizations] = useState<Organization[]>([]);
  const [selectedOrgId, setSelectedOrgId] = useState<number | null>(null);
  const [activeTab, setActiveTab] = useState('create');
  const [hclContent, setHclContent] = useState('');
  const [jsonContent, setJsonContent] = useState<any>(null);
  const [jsonFileName, setJsonFileName] = useState('');
  const [importingJson, setImportingJson] = useState(false);
  const [jsonImportForm] = Form.useForm();
  const toast = useToast();

  // 从 URL 参数获取组织 ID 和 Tab
  useEffect(() => {
    const tabParam = searchParams.get('tab');
    if (tabParam === 'import') {
      setActiveTab('import');
    }
  }, [searchParams]);

  // 加载组织列表
  useEffect(() => {
    const loadOrganizations = async () => {
      try {
        const response = await iamService.listOrganizations(true);
        setOrganizations(response.organizations || []);
        
        // 优先使用 URL 参数中的组织 ID
        const orgParam = searchParams.get('org');
        if (orgParam) {
          setSelectedOrgId(parseInt(orgParam, 10));
        } else if (response.organizations && response.organizations.length > 0) {
          setSelectedOrgId(response.organizations[0].id);
        }
      } catch (error) {
        console.error('加载组织列表失败:', error);
      }
    };
    loadOrganizations();
  }, [searchParams]);

  const orgId = selectedOrgId?.toString() || '';

  const handleSubmit = async (values: CreateManifestRequest) => {
    setCreating(true);
    try {
      await createManifest(orgId, values);
      toast.success('创建成功');
      // 跳转到列表页面
      navigate('/admin/manifests');
    } catch (error: any) {
      toast.error('创建失败: ' + (error.message || '未知错误'));
    } finally {
      setCreating(false);
    }
  };

  const handleImport = async (values: { name: string }) => {
    if (!hclContent.trim()) {
      toast.error('请输入或上传 HCL 内容');
      return;
    }
    setImporting(true);
    try {
      const manifest = await importManifestHCL(orgId, hclContent, values.name);
      toast.success(`导入成功，已解析 Manifest`);
      // 跳转到编辑页面
      navigate(`/admin/manifests/${manifest.id}/edit?org=${orgId}`);
    } catch (error: any) {
      toast.error('导入失败: ' + (error.message || '未知错误'));
    } finally {
      setImporting(false);
    }
  };

  const handleFileUpload = (file: File) => {
    const reader = new FileReader();
    reader.onload = (e) => {
      const content = e.target?.result as string;
      setHclContent(content);
      message.success(`已加载文件: ${file.name}`);
    };
    reader.onerror = () => {
      message.error('读取文件失败');
    };
    reader.readAsText(file);
    return false; // 阻止自动上传
  };

  return (
    <div className={styles.container}>
      <div className={styles.header}>
        <Button
          type="text"
          icon={<ArrowLeftOutlined />}
          onClick={() => navigate('/admin/manifests')}
        >
          返回列表
        </Button>
      </div>

      <Card className={styles.formCard}>
        <Title level={3}>创建 Manifest</Title>
        <Text type="secondary" className={styles.description}>
          Manifest 是一个可视化的基础设施编排模板，可以将多个 Module 组合在一起，
          定义它们之间的依赖关系和变量绑定，然后部署到不同的 Workspace。
        </Text>

        {organizations.length > 1 && (
          <div style={{ marginBottom: 24 }}>
            <label style={{ display: 'block', marginBottom: 8, fontWeight: 500 }}>
              所属组织
            </label>
            <Select
              value={selectedOrgId}
              onChange={(value) => setSelectedOrgId(value)}
              style={{ width: 300 }}
              options={organizations.map(org => ({
                value: org.id,
                label: org.display_name || org.name,
              }))}
            />
          </div>
        )}

        <Tabs
          activeKey={activeTab}
          onChange={setActiveTab}
          items={[
            {
              key: 'create',
              label: (
                <span>
                  <PlusOutlined />
                  新建空白
                </span>
              ),
              children: (
                <Form
                  form={form}
                  layout="vertical"
                  onFinish={handleSubmit}
                  className={styles.form}
                >
                  <Form.Item
                    name="name"
                    label="名称"
                    rules={[
                      { required: true, message: '请输入名称' },
                      { max: 255, message: '名称最多 255 个字符' },
                      {
                        pattern: /^[a-zA-Z0-9_-]+$/,
                        message: '名称只能包含字母、数字、下划线和连字符',
                      },
                    ]}
                    extra="名称用于标识 Manifest，只能包含字母、数字、下划线和连字符"
                  >
                    <Input placeholder="例如: vpc-ec2-stack" size="large" />
                  </Form.Item>

                  <Form.Item
                    name="description"
                    label="描述"
                    extra="描述这个 Manifest 的用途和包含的资源"
                  >
                    <TextArea
                      rows={4}
                      placeholder="例如: 创建一个包含 VPC、子网和 EC2 实例的完整网络架构"
                    />
                  </Form.Item>

                  <Form.Item className={styles.actions}>
                    <Space>
                      <Button onClick={() => navigate('/admin/manifests')}>
                        取消
                      </Button>
                      <Button 
                        type="primary" 
                        htmlType="submit" 
                        loading={creating}
                        disabled={!selectedOrgId}
                      >
                        创建
                      </Button>
                    </Space>
                  </Form.Item>
                </Form>
              ),
            },
            {
              key: 'import',
              label: (
                <span>
                  <FileTextOutlined />
                  从 HCL 导入
                </span>
              ),
              children: (
                <Form
                  form={importForm}
                  layout="vertical"
                  onFinish={handleImport}
                  className={styles.form}
                >
                  <Form.Item
                    name="name"
                    label="Manifest 名称"
                    rules={[
                      { required: true, message: '请输入名称' },
                      { max: 255, message: '名称最多 255 个字符' },
                      {
                        pattern: /^[a-zA-Z0-9_-]+$/,
                        message: '名称只能包含字母、数字、下划线和连字符',
                      },
                    ]}
                  >
                    <Input placeholder="例如: imported-stack" size="large" />
                  </Form.Item>

                  <Form.Item
                    label="HCL 内容"
                    required
                    extra="支持 Terraform HCL 格式，会自动解析 module 和 variable 块"
                  >
                    <Space direction="vertical" style={{ width: '100%' }}>
                      <Upload
                        accept=".tf,.hcl"
                        beforeUpload={handleFileUpload}
                        showUploadList={false}
                      >
                        <Button icon={<UploadOutlined />}>
                          上传 .tf 或 .hcl 文件
                        </Button>
                      </Upload>
                      <TextArea
                        rows={12}
                        value={hclContent}
                        onChange={(e) => setHclContent(e.target.value)}
                        placeholder={`# 粘贴 Terraform HCL 代码，例如:

module "vpc" {
  source  = "terraform-aws-modules/vpc/aws"
  version = "5.0.0"
  
  name = "my-vpc"
  cidr = "10.0.0.0/16"
}

module "ec2" {
  source  = "terraform-aws-modules/ec2-instance/aws"
  version = "5.0.0"
  
  name          = "my-instance"
  instance_type = "t3.micro"
}`}
                        style={{ fontFamily: 'monospace', fontSize: 13 }}
                      />
                      {hclContent && (
                        <Text type="secondary">
                          已输入 {hclContent.length} 字符
                        </Text>
                      )}
                    </Space>
                  </Form.Item>

                  <Form.Item className={styles.actions}>
                    <Space>
                      <Button onClick={() => navigate('/admin/manifests')}>
                        取消
                      </Button>
                      <Button 
                        type="primary" 
                        htmlType="submit" 
                        loading={importing}
                        disabled={!selectedOrgId || !hclContent.trim()}
                      >
                        导入并创建
                      </Button>
                    </Space>
                  </Form.Item>
                </Form>
              ),
            },
            {
              key: 'import-json',
              label: (
                <span>
                  <ImportOutlined />
                  从 ZIP/JSON 导入
                </span>
              ),
              children: (
                <Form
                  form={jsonImportForm}
                  layout="vertical"
                  onFinish={async (values: { name?: string }) => {
                    if (!jsonContent) {
                      toast.error('请上传 ZIP 或 manifest.json 文件');
                      return;
                    }
                    setImportingJson(true);
                    try {
                      const manifest = await importManifestJSON(orgId, jsonContent, values.name);
                      toast.success(`导入成功，已恢复 Manifest 画布数据`);
                      // 跳转到编辑页面
                      navigate(`/admin/manifests/${manifest.id}/edit?org=${orgId}`);
                    } catch (error: any) {
                      toast.error('导入失败: ' + (error.message || '未知错误'));
                    } finally {
                      setImportingJson(false);
                    }
                  }}
                  className={styles.form}
                >
                  <Form.Item
                    name="name"
                    label="Manifest 名称（可选）"
                    extra="留空则使用文件中的名称"
                  >
                    <Input placeholder="留空使用原名称" size="large" />
                  </Form.Item>

                  <Form.Item
                    label="导入文件"
                    required
                    extra="支持上传 .zip 文件（导出的完整包）或 .manifest.json 文件，将完整恢复节点位置、连线和配置"
                  >
                    <Space direction="vertical" style={{ width: '100%' }}>
                      <Upload
                        accept=".zip,.json,.manifest.json"
                        beforeUpload={async (file: File) => {
                          // 处理 ZIP 文件
                          if (file.name.endsWith('.zip')) {
                            try {
                              // 使用 JSZip 解压
                              const JSZip = (await import('jszip')).default;
                              const zip = await JSZip.loadAsync(file);
                              
                              // 查找 manifest.json 文件
                              let manifestFile: any = null;
                              let manifestFileName = '';
                              
                              zip.forEach((relativePath, zipEntry) => {
                                if (relativePath.endsWith('.manifest.json') || relativePath.endsWith('manifest.json')) {
                                  manifestFile = zipEntry;
                                  manifestFileName = relativePath;
                                }
                              });
                              
                              if (!manifestFile) {
                                message.error('ZIP 文件中未找到 manifest.json');
                                return false;
                              }
                              
                              const content = await manifestFile.async('string');
                              const jsonData = JSON.parse(content);
                              setJsonContent(jsonData);
                              setJsonFileName(`${file.name} → ${manifestFileName}`);
                              
                              // 如果 JSON 中有名称，自动填充
                              if (jsonData.name) {
                                jsonImportForm.setFieldsValue({ name: jsonData.name });
                              }
                              message.success(`已从 ZIP 中提取: ${manifestFileName}`);
                            } catch (err: any) {
                              console.error('解压 ZIP 失败:', err);
                              message.error('解压 ZIP 失败: ' + (err.message || '未知错误'));
                            }
                            return false;
                          }
                          
                          // 处理 JSON 文件
                          const reader = new FileReader();
                          reader.onload = (e) => {
                            try {
                              const content = JSON.parse(e.target?.result as string);
                              setJsonContent(content);
                              setJsonFileName(file.name);
                              // 如果 JSON 中有名称，自动填充
                              if (content.name) {
                                jsonImportForm.setFieldsValue({ name: content.name });
                              }
                              message.success(`已加载文件: ${file.name}`);
                            } catch (err) {
                              message.error('JSON 文件格式错误');
                            }
                          };
                          reader.onerror = () => {
                            message.error('读取文件失败');
                          };
                          reader.readAsText(file);
                          return false;
                        }}
                        showUploadList={false}
                      >
                        <Button icon={<UploadOutlined />}>
                          上传 .zip 或 .manifest.json 文件
                        </Button>
                      </Upload>
                      {jsonContent && (
                        <Card size="small" style={{ marginTop: 8 }}>
                          <Text strong>已加载: {jsonFileName}</Text>
                          <div style={{ marginTop: 8, fontSize: 12, color: '#666' }}>
                            <div>名称: {jsonContent.name || '-'}</div>
                            <div>版本: {jsonContent.version || '-'}</div>
                            <div>节点数: {jsonContent.nodes?.length || 0}</div>
                            <div>连线数: {jsonContent.edges?.length || 0}</div>
                            <div>变量数: {jsonContent.variables?.length || 0}</div>
                            {jsonContent.exported_at && (
                              <div>导出时间: {new Date(jsonContent.exported_at).toLocaleString()}</div>
                            )}
                          </div>
                        </Card>
                      )}
                    </Space>
                  </Form.Item>

                  <Form.Item className={styles.actions}>
                    <Space>
                      <Button onClick={() => navigate('/admin/manifests')}>
                        取消
                      </Button>
                      <Button 
                        type="primary" 
                        htmlType="submit" 
                        loading={importingJson}
                        disabled={!selectedOrgId || !jsonContent}
                      >
                        导入并创建
                      </Button>
                    </Space>
                  </Form.Item>
                </Form>
              ),
            },
          ]}
        />
      </Card>
    </div>
  );
};

export default ManifestCreate;

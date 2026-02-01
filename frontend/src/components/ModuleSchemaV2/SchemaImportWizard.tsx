import React, { useState, useCallback } from 'react';
import { Steps, Button, message, Spin, Form, Input, Select, Row, Col } from 'antd';
import { schemaV2Service, type OpenAPISchema, type ParseTFResponse } from '../../services/schemaV2';
import VariablesTfUploader from './VariablesTfUploader';
import AnnotationGuide from './AnnotationGuide';
import FieldConfigPanel from './FieldConfigPanel';
import SchemaPreview from './SchemaPreview';
import styles from './ModuleSchemaV2.module.css';

const { Option } = Select;

interface SchemaImportWizardProps {
  moduleId: number;
  moduleName?: string;
  provider?: string;
  onSuccess?: (schemaId: number) => void;
  onCancel?: () => void;
}

const SchemaImportWizard: React.FC<SchemaImportWizardProps> = ({
  moduleId,
  moduleName = '',
  provider = 'aws',
  onSuccess,
  onCancel,
}) => {
  const [currentStep, setCurrentStep] = useState(0);
  const [loading, setLoading] = useState(false);
  const [variablesTf, setVariablesTf] = useState('');
  const [parseResult, setParseResult] = useState<ParseTFResponse | null>(null);
  const [schema, setSchema] = useState<OpenAPISchema | null>(null);
  const [showGuide, setShowGuide] = useState(false);
  const [form] = Form.useForm();

  // 解析 variables.tf
  const handleParse = useCallback(async () => {
    if (!variablesTf.trim()) {
      message.warning('请先上传或粘贴 variables.tf 内容');
      return;
    }

    setLoading(true);
    try {
      const values = await form.validateFields();
      const response = await schemaV2Service.parseTF({
        variables_tf: variablesTf,
        module_name: values.moduleName || moduleName,
        provider: values.provider || provider,
        version: values.version || '1.0.0',
        layout: values.layout || 'top',
      });

      setParseResult(response);
      setSchema(response.openapi_schema);
      message.success(`解析成功，共 ${response.field_count} 个字段`);
      setCurrentStep(1);
    } catch (error: unknown) {
      const err = error as { response?: { data?: { error?: string } }; message?: string };
      message.error(err.response?.data?.error || err.message || '解析失败');
    } finally {
      setLoading(false);
    }
  }, [variablesTf, form, moduleName, provider]);

  // 更新字段配置
  const handleFieldUpdate = useCallback((fieldName: string, property: string, value: unknown) => {
    if (!schema) return;

    const newSchema = { ...schema };
    const uiConfig = newSchema['x-iac-platform']?.ui;
    if (uiConfig?.fields) {
      if (!uiConfig.fields[fieldName]) {
        uiConfig.fields[fieldName] = {};
      }
      (uiConfig.fields[fieldName] as Record<string, unknown>)[property] = value;
    }
    setSchema(newSchema);
  }, [schema]);

  // 保存 Schema
  const handleSave = useCallback(async () => {
    if (!schema) {
      message.error('Schema 数据为空');
      return;
    }

    setLoading(true);
    try {
      const values = await form.validateFields();
      const response = await schemaV2Service.createSchemaV2(moduleId, {
        version: values.version || '1.0.0',
        openapi_schema: schema,
        variables_tf: variablesTf,
        status: 'active',
        source_type: 'tf_parse',
      });

      message.success('Schema 创建成功');
      onSuccess?.(response.id);
    } catch (error: unknown) {
      const err = error as { response?: { data?: { error?: string } }; message?: string };
      message.error(err.response?.data?.error || err.message || '保存失败');
    } finally {
      setLoading(false);
    }
  }, [schema, variablesTf, moduleId, form, onSuccess]);

  // 步骤配置
  const steps = [
    {
      title: '上传文件',
      description: '上传 variables.tf',
    },
    {
      title: '配置字段',
      description: '自定义字段显示',
    },
    {
      title: '预览确认',
      description: '确认并保存',
    },
  ];

  // 渲染步骤内容
  const renderStepContent = () => {
    switch (currentStep) {
      case 0:
        return (
          <div>
            <Form form={form} layout="vertical" initialValues={{ moduleName, provider, version: '1.0.0', layout: 'top' }}>
              <Row gutter={16}>
                <Col span={8}>
                  <Form.Item name="moduleName" label="模块名称">
                    <Input placeholder="模块名称" />
                  </Form.Item>
                </Col>
                <Col span={8}>
                  <Form.Item name="provider" label="Provider">
                    <Select>
                      <Option value="aws">AWS</Option>
                      <Option value="azure">Azure</Option>
                      <Option value="gcp">GCP</Option>
                      <Option value="alicloud">阿里云</Option>
                      <Option value="other">其他</Option>
                    </Select>
                  </Form.Item>
                </Col>
                <Col span={8}>
                  <Form.Item name="version" label="版本">
                    <Input placeholder="1.0.0" />
                  </Form.Item>
                </Col>
              </Row>
            </Form>
            
            <VariablesTfUploader
              value={variablesTf}
              onChange={setVariablesTf}
              onShowGuide={() => setShowGuide(true)}
            />
          </div>
        );

      case 1:
        return schema ? (
          <FieldConfigPanel
            schema={schema}
            onFieldUpdate={handleFieldUpdate}
          />
        ) : (
          <div>请先解析 variables.tf</div>
        );

      case 2:
        return (
          <SchemaPreview
            schema={schema}
            parseResult={parseResult}
          />
        );

      default:
        return null;
    }
  };

  // 渲染操作按钮
  const renderActions = () => {
    return (
      <div className={styles.wizardActions}>
        <div>
          {currentStep > 0 && (
            <Button onClick={() => setCurrentStep(currentStep - 1)}>
              上一步
            </Button>
          )}
        </div>
        <div>
          <Button onClick={onCancel} style={{ marginRight: 8 }}>
            取消
          </Button>
          {currentStep === 0 && (
            <Button
              type="primary"
              onClick={handleParse}
              disabled={!variablesTf.trim()}
              loading={loading}
            >
              解析并继续
            </Button>
          )}
          {currentStep === 1 && (
            <Button
              type="primary"
              onClick={() => setCurrentStep(2)}
            >
              下一步
            </Button>
          )}
          {currentStep === 2 && (
            <Button
              type="primary"
              onClick={handleSave}
              loading={loading}
            >
              保存 Schema
            </Button>
          )}
        </div>
      </div>
    );
  };

  return (
    <div className={styles.wizardContainer}>
      <Steps
        current={currentStep}
        items={steps}
        className={styles.wizardSteps}
      />

      <Spin spinning={loading}>
        <div className={styles.wizardContent}>
          {renderStepContent()}
        </div>
      </Spin>

      {renderActions()}

      <AnnotationGuide
        visible={showGuide}
        onClose={() => setShowGuide(false)}
      />
    </div>
  );
};

export default SchemaImportWizard;

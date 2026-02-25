import React, { useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { MonacoJsonEditor } from '../components/DynamicForm/MonacoJsonEditor';
import { OpenAPISchemaEditor } from '../components/OpenAPISchemaEditor';
import { moduleService } from '../services/modules';
import api from '../services/api';
import { schemaV2Service, type OpenAPISchema } from '../services/schemaV2';
import { useToast } from '../contexts/ToastContext';
import styles from './ImportModule.module.css';

type ImportMethod = 'json' | 'tf-file' | 'tar' | 'git';

const ImportModule: React.FC = () => {
  const navigate = useNavigate();
  const { success, error } = useToast();
  const [importMethod, setImportMethod] = useState<ImportMethod>('json');
  const [loading, setLoading] = useState(false);

  // JSON导入状态
  const [jsonConfig, setJsonConfig] = useState('');

  // TF文件导入状态
  const [tfFiles, setTfFiles] = useState<File[]>([]);
  const [tfContent, setTfContent] = useState('');
  const [outputsContent, setOutputsContent] = useState('');
  const [showSchemaEditor, setShowSchemaEditor] = useState(false);
  const [parsedOpenAPISchema, setParsedOpenAPISchema] = useState<OpenAPISchema | null>(null);

  // TAR包导入状态
  const [tarFile, setTarFile] = useState<File | null>(null);

  // Git导入状态
  const [gitUrl, setGitUrl] = useState('');
  const [gitBranch, setGitBranch] = useState('main');

  // 通用字段
  const [moduleName, setModuleName] = useState('');
  const [provider, setProvider] = useState('AWS');
  const [moduleSource, setModuleSource] = useState('');
  const [moduleVersion, setModuleVersion] = useState('');
  const [description, setDescription] = useState('');
  
  // 模块名称检查
  const [nameCheckStatus, setNameCheckStatus] = useState<'idle' | 'checking' | 'available' | 'exists'>('idle');
  const [checkTimeout, setCheckTimeout] = useState<number | null>(null);

  const handleJsonImport = async () => {
    if (!jsonConfig.trim()) {
      error('请输入JSON配置');
      return;
    }

    let config;
    try {
      config = JSON.parse(jsonConfig);
    } catch (e) {
      error('JSON格式错误，请检查后重试');
      return;
    }

    if (!moduleName.trim()) {
      error('请输入模块名称');
      return;
    }

    try {
      setLoading(true);
      
      const moduleData = {
        name: moduleName,
        provider: provider,
        module_source: moduleSource,
        version: moduleVersion || config.version || '',
        description: description || config.description || '',
        repository_url: 'json-import',
        branch: config.version || '1.0.0'
      };

      const moduleResponse = await moduleService.createModule(moduleData);
      const moduleId = moduleResponse.data.id;

      if (config.schema || config.openapi) {
        // 判断是 OpenAPI Schema 还是旧格式
        const schemaData = config.openapi ? config : { schema_data: config.schema };

        await api.post(`/modules/${moduleId}/schemas`, {
          ...schemaData,
          version: config.schema_version || config.info?.version || '1.0.0',
          status: 'active',
          source_type: config.openapi ? 'openapi' : 'manual'
        });

        success('模块和Schema导入成功！');
        navigate(`/modules/${moduleId}/schemas`);
      } else {
        success('模块导入成功！请添加Schema配置。');
        navigate(`/modules/${moduleId}/schemas`);
      }
    } catch (err: any) {
      const errorMessage = typeof err === 'string' ? err : (err.message || '未知错误');
      if (errorMessage.includes('duplicate key') || errorMessage.includes('unique constraint')) {
        error('模块名称已存在！请使用不同的模块名称或删除已存在的模块。');
      } else {
        error('导入失败: ' + errorMessage);
      }
    } finally {
      setLoading(false);
    }
  };

  const handleTfFileImport = async () => {
    const hasFiles = tfFiles.length > 0;
    const hasContent = tfContent.trim() || outputsContent.trim();
    
    if (!hasFiles && !hasContent) {
      error('请上传.tf文件或粘贴内容');
      return;
    }

    if (!moduleName.trim()) {
      error('请输入模块名称');
      return;
    }

    try {
      setLoading(true);

      let variablesTf = tfContent;
      let outputsTf = outputsContent;
      
      // 如果有上传的文件，读取文件内容
      if (hasFiles) {
        for (const file of tfFiles) {
          const content = await file.text();
          const fileName = file.name.toLowerCase();
          
          if (fileName.includes('variable') || fileName === 'variables.tf') {
            variablesTf += content + '\n';
          } else if (fileName.includes('output') || fileName === 'outputs.tf') {
            outputsTf += content + '\n';
          } else {
            // 默认当作 variables.tf 处理
            variablesTf += content + '\n';
          }
        }
      }

      // 调用 V2 解析 API，直接获取 OpenAPI Schema
      const parseResult = await schemaV2Service.parseTF({
        variables_tf: variablesTf || '',
        outputs_tf: outputsTf || '',
        module_name: moduleName,
        provider: provider.toLowerCase(),
        version: '1.0.0',
      });
      
      // 直接使用 OpenAPI Schema，不做转换
      setParsedOpenAPISchema(parseResult.openapi_schema);
      setShowSchemaEditor(true);
      
      const fieldCount = parseResult.field_count || 0;
      const outputCount = (parseResult.openapi_schema as any)?.['x-iac-platform']?.outputs?.items?.length || 0;
      
      let message = `解析成功！共 ${fieldCount} 个变量`;
      if (outputCount > 0) {
        message += `，${outputCount} 个输出`;
      }
      success(message);
      
    } catch (err: any) {
      error('解析失败: ' + (err.message || '未知错误'));
    } finally {
      setLoading(false);
    }
  };

  // Schema编辑完成后保存（直接保存 OpenAPI Schema）
  const handleSchemaSave = async (openAPISchema: OpenAPISchema) => {
    try {
      setLoading(true);

      const moduleData = {
        name: moduleName,
        provider: provider,
        module_source: moduleSource,
        version: moduleVersion,
        description: description,
        repository_url: 'tf-file-import',
        branch: '1.0.0'
      };

      let moduleId: number | string;
      let isExistingModule = false;
      
      try {
        const moduleResponse = await moduleService.createModule(moduleData);
        moduleId = moduleResponse.data.id;
      } catch (moduleErr: any) {
        // API interceptor rejects with a string message, not an Error object
        const errMsg = typeof moduleErr === 'string' ? moduleErr : (moduleErr.message || '');
        if (errMsg.includes('duplicate key') || errMsg.includes('unique constraint') || errMsg.includes('已存在')) {
          // 模块已存在，尝试查找已存在的模块
          try {
            const modulesResponse = await moduleService.getModules();
            const allModules = (modulesResponse as any).data?.items || (modulesResponse as any).data || [];
            const existingModule = (Array.isArray(allModules) ? allModules : []).find((m: any) => m.name === moduleName && m.provider === provider);
            if (existingModule) {
              moduleId = existingModule.id;
              isExistingModule = true;
            } else {
              error(`模块 "${moduleName}" 已存在但无法找到，请尝试使用不同的名称。`);
              return;
            }
          } catch {
            error(`模块 "${moduleName}" 已存在！请使用不同的模块名称，或者先删除已存在的模块。`);
            return;
          }
        } else {
          throw moduleErr;
        }
      }

      // 直接保存 OpenAPI Schema
      await api.post(`/modules/${moduleId}/schemas/v2`, {
        openapi_schema: openAPISchema,
        version: '1.0.0',
        status: 'active',
        source_type: 'tf_parse'
      });

      if (isExistingModule) {
        success(`Schema 已添加到已存在的模块 "${moduleName}"！`);
      } else {
        success('模块和Schema创建成功！');
      }
      navigate(`/modules/${moduleId}/schemas`);
      
    } catch (err: any) {
      const errMsg = typeof err === 'string' ? err : (err.message || '未知错误');
      if (errMsg.includes('duplicate key') || errMsg.includes('unique constraint')) {
        error(`模块 "${moduleName}" 已存在！请使用不同的模块名称。`);
      } else {
        error('保存失败: ' + errMsg);
      }
    } finally {
      setLoading(false);
    }
  };

  // 检查模块名称是否存在
  const checkModuleName = async (name: string) => {
    if (!name.trim()) {
      setNameCheckStatus('idle');
      return;
    }
    
    setNameCheckStatus('checking');
    try {
      const response = await moduleService.getModules();
      const modules = response.data || [];
      const exists = modules.some((m: any) => m.name === name);
      if (exists) {
        setNameCheckStatus('exists');
      } else {
        setNameCheckStatus('available');
      }
    } catch {
      setNameCheckStatus('idle');
    }
  };

  // 处理模块名称变化
  const handleModuleNameChange = (name: string) => {
    setModuleName(name);
    
    if (checkTimeout) {
      clearTimeout(checkTimeout);
    }
    
    const timeout = window.setTimeout(() => {
      checkModuleName(name);
    }, 500);
    
    setCheckTimeout(timeout);
  };

  // 如果正在显示 Schema 编辑器
  if (showSchemaEditor && parsedOpenAPISchema) {
    return (
      <div className={styles.container}>
        <OpenAPISchemaEditor
          schema={parsedOpenAPISchema}
          onSave={handleSchemaSave}
          onCancel={() => {
            setShowSchemaEditor(false);
            setParsedOpenAPISchema(null);
          }}
        />
      </div>
    );
  }

  return (
    <div className={styles.container}>
      <div className={styles.header}>
        <h1>导入模块</h1>
        <p>选择导入方式来添加新的Terraform模块</p>
      </div>

      {/* 导入方式选择 */}
      <div className={styles.methodSelector}>
        <button
          className={`${styles.methodButton} ${importMethod === 'json' ? styles.active : ''}`}
          onClick={() => setImportMethod('json')}
        >
          📋 JSON配置
        </button>
        <button
          className={`${styles.methodButton} ${importMethod === 'tf-file' ? styles.active : ''}`}
          onClick={() => setImportMethod('tf-file')}
        >
          📄 TF文件
        </button>
        <button
          className={`${styles.methodButton} ${importMethod === 'tar' ? styles.active : ''}`}
          onClick={() => setImportMethod('tar')}
          disabled
        >
          📦 TAR包 (开发中)
        </button>
        <button
          className={`${styles.methodButton} ${importMethod === 'git' ? styles.active : ''}`}
          onClick={() => setImportMethod('git')}
          disabled
        >
          🔗 Git仓库 (开发中)
        </button>
      </div>

      {/* 通用字段 */}
      <div className={styles.commonFields}>
        <div className={styles.formGroup}>
          <label>模块名称 *</label>
          <div className={styles.inputWithStatus}>
            <input
              type="text"
              value={moduleName}
              onChange={(e) => handleModuleNameChange(e.target.value)}
              placeholder="例如：aws-s3-bucket"
              className={styles.input}
            />
            {nameCheckStatus === 'checking' && <span className={styles.checking}>检查中...</span>}
            {nameCheckStatus === 'available' && <span className={styles.available}>✓ 可用</span>}
            {nameCheckStatus === 'exists' && <span className={styles.exists}>✗ 已存在</span>}
          </div>
        </div>

        <div className={styles.formRow}>
          <div className={styles.formGroup}>
            <label>Provider</label>
            <select
              value={provider}
              onChange={(e) => setProvider(e.target.value)}
              className={styles.select}
            >
              <option value="AWS">AWS</option>
              <option value="Azure">Azure</option>
              <option value="GCP">GCP</option>
              <option value="Kubernetes">Kubernetes</option>
              <option value="Other">其他</option>
            </select>
          </div>

          <div className={styles.formGroup}>
            <label>模块源</label>
            <input
              type="text"
              value={moduleSource}
              onChange={(e) => setModuleSource(e.target.value)}
              placeholder="例如：terraform-aws-modules/s3-bucket/aws"
              className={styles.input}
            />
          </div>
        </div>

        <div className={styles.formGroup}>
          <label>版本</label>
          <input
            type="text"
            value={moduleVersion}
            onChange={(e) => setModuleVersion(e.target.value)}
            placeholder="例如：5.0.0"
            className={styles.input}
          />
          <small className={styles.fieldHint}>
            Terraform Registry 模块版本号，将在生成的 tf_code 中使用
          </small>
        </div>

        <div className={styles.formGroup}>
          <label>描述</label>
          <textarea
            value={description}
            onChange={(e) => setDescription(e.target.value)}
            placeholder="模块描述..."
            className={styles.textarea}
            rows={2}
          />
        </div>
      </div>

      {/* JSON导入 */}
      {importMethod === 'json' && (
        <div className={styles.importSection}>
          <h3>JSON配置导入</h3>
          <p className={styles.hint}>粘贴包含模块配置的JSON（支持OpenAPI Schema格式）</p>
          <div className={styles.jsonEditorWrapper}>
            <MonacoJsonEditor
              value={jsonConfig}
              onChange={setJsonConfig}
              minHeight={300}
            />
          </div>
          <button
            onClick={handleJsonImport}
            disabled={loading || !moduleName.trim()}
            className={styles.importButton}
          >
            {loading ? '导入中...' : '导入模块'}
          </button>
        </div>
      )}

      {/* TF文件导入 */}
      {importMethod === 'tf-file' && (
        <div className={styles.importSection}>
          <h3>Terraform文件导入</h3>
          <p className={styles.hint}>上传 TF 文件（支持多选 variables.tf + outputs.tf）或粘贴内容，系统将自动解析并生成 OpenAPI Schema</p>
          
          <div className={styles.fileUpload}>
            <input
              type="file"
              accept=".tf,.hcl,text/plain"
              multiple
              onChange={(e) => {
                const files = e.target.files;
                if (files && files.length > 0) {
                  setTfFiles(Array.from(files));
                  setTfContent('');
                  setOutputsContent('');
                }
              }}
              id="tf-file-input"
              className={styles.fileInput}
            />
            <label htmlFor="tf-file-input" className={styles.fileLabel}>
              {tfFiles.length > 0 
                ? `已选择: ${tfFiles.map(f => f.name).join(', ')}` 
                : '选择 .tf 文件（可多选）'}
            </label>
          </div>

          <div className={styles.orDivider}>或</div>

          <div className={styles.formGroup}>
            <label>粘贴 variables.tf 内容</label>
            <textarea
              value={tfContent}
              onChange={(e) => {
                setTfContent(e.target.value);
                setTfFiles([]);
              }}
              placeholder={`variable "bucket_name" {
  description = "S3 bucket name"  # @level:basic @alias:存储桶名称
  type        = string
}

variable "tags" {
  description = "Resource tags"
  type        = map(string)
  default     = {}
}`}
              className={styles.codeTextarea}
              rows={10}
            />
          </div>

          <div className={styles.formGroup}>
            <label>粘贴 outputs.tf 内容（可选）</label>
            <textarea
              value={outputsContent}
              onChange={(e) => {
                setOutputsContent(e.target.value);
                setTfFiles([]);
              }}
              placeholder={`output "bucket_id" {
  description = "The ID of the S3 bucket"  # @alias:存储桶ID
  value       = aws_s3_bucket.this.id
}

output "bucket_arn" {
  description = "The ARN of the S3 bucket"
  value       = aws_s3_bucket.this.arn
}`}
              className={styles.codeTextarea}
              rows={8}
            />
          </div>

          <button
            onClick={handleTfFileImport}
            disabled={loading || !moduleName.trim() || (tfFiles.length === 0 && !tfContent.trim() && !outputsContent.trim())}
            className={styles.importButton}
          >
            {loading ? '解析中...' : '解析并导入'}
          </button>
        </div>
      )}

      {/* TAR包导入 */}
      {importMethod === 'tar' && (
        <div className={styles.importSection}>
          <h3>TAR包导入</h3>
          <p className={styles.hint}>上传包含Terraform模块的TAR包</p>
          <div className={styles.fileUpload}>
            <input
              type="file"
              accept=".tar,.tar.gz,.tgz"
              onChange={(e) => setTarFile(e.target.files?.[0] || null)}
              id="tar-file-input"
              className={styles.fileInput}
            />
            <label htmlFor="tar-file-input" className={styles.fileLabel}>
              {tarFile ? `已选择: ${tarFile.name}` : '选择 TAR 文件'}
            </label>
          </div>
          <button disabled className={styles.importButton}>
            功能开发中...
          </button>
        </div>
      )}

      {/* Git导入 */}
      {importMethod === 'git' && (
        <div className={styles.importSection}>
          <h3>Git仓库导入</h3>
          <p className={styles.hint}>从Git仓库导入Terraform模块</p>
          <div className={styles.formGroup}>
            <label>Git URL</label>
            <input
              type="text"
              value={gitUrl}
              onChange={(e) => setGitUrl(e.target.value)}
              placeholder="https://github.com/user/repo.git"
              className={styles.input}
            />
          </div>
          <div className={styles.formGroup}>
            <label>分支</label>
            <input
              type="text"
              value={gitBranch}
              onChange={(e) => setGitBranch(e.target.value)}
              placeholder="main"
              className={styles.input}
            />
          </div>
          <button disabled className={styles.importButton}>
            功能开发中...
          </button>
        </div>
      )}
    </div>
  );
};

export default ImportModule;

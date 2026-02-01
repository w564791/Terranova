import React, { useState } from 'react';
import { MonacoJsonEditor } from './MonacoJsonEditor';
import styles from './SchemaImportDialog.module.css';

interface SchemaImportDialogProps {
  onClose: () => void;
  onImport: (schemaData: any, version: string) => Promise<void>;
}

export const SchemaImportDialog: React.FC<SchemaImportDialogProps> = ({
  onClose,
  onImport
}) => {
  const [jsonValue, setJsonValue] = useState('');
  const [version, setVersion] = useState('1.0.0');
  const [importing, setImporting] = useState(false);
  const [error, setError] = useState('');

  const handleImport = async () => {
    setError('');
    
    // 验证JSON格式
    if (!jsonValue.trim()) {
      setError('请输入Schema JSON配置');
      return;
    }

    let schemaData;
    try {
      schemaData = JSON.parse(jsonValue);
    } catch (e) {
      setError('JSON格式错误，请检查后重试');
      return;
    }

    // 验证版本号
    if (!version.trim()) {
      setError('请输入版本号');
      return;
    }

    try {
      setImporting(true);
      await onImport(schemaData, version);
      onClose();
    } catch (e: any) {
      setError(e.message || '导入失败，请重试');
    } finally {
      setImporting(false);
    }
  };

  const handleOverlayClick = (e: React.MouseEvent) => {
    if (e.target === e.currentTarget) {
      onClose();
    }
  };

  const loadExample = () => {
    const exampleSchema = {
      "bucket_name": {
        "type": "string",
        "required": true,
        "description": "S3存储桶名称",
        "placeholder": "my-bucket-name"
      },
      "region": {
        "type": "string",
        "required": true,
        "description": "AWS区域",
        "default": "us-west-2",
        "options": ["us-east-1", "us-west-2", "eu-west-1", "ap-southeast-1"]
      },
      "versioning_enabled": {
        "type": "boolean",
        "required": false,
        "description": "是否启用版本控制",
        "default": false
      },
      "tags": {
        "type": "map",
        "required": false,
        "description": "资源标签",
        "default": {}
      }
    };
    setJsonValue(JSON.stringify(exampleSchema, null, 2));
  };

  return (
    <div className={styles.overlay} onClick={handleOverlayClick}>
      <div className={styles.dialog}>
        <div className={styles.header}>
          <h2 className={styles.title}>导入JSON Schema</h2>
          <button
            className={styles.closeButton}
            onClick={onClose}
            type="button"
            aria-label="关闭"
          >
            ×
          </button>
        </div>

        <div className={styles.content}>
          <div className={styles.description}>
            <p>直接粘贴或上传JSON格式的Schema配置，系统将自动保存到数据库。</p>
            <button
              type="button"
              onClick={loadExample}
              className={styles.exampleButton}
            >
              加载示例
            </button>
          </div>

          <div className={styles.formGroup}>
            <label className={styles.label}>
              版本号 <span className={styles.required}>*</span>
            </label>
            <input
              type="text"
              value={version}
              onChange={(e) => setVersion(e.target.value)}
              placeholder="1.0.0"
              className={styles.input}
            />
          </div>

          <div className={styles.formGroup}>
            <label className={styles.label}>
              Schema JSON <span className={styles.required}>*</span>
            </label>
            <MonacoJsonEditor
              value={jsonValue}
              onChange={setJsonValue}
              minHeight={400}
            />
          </div>

          {error && (
            <div className={styles.error}>
              <span className={styles.errorIcon}></span>
              {error}
            </div>
          )}
        </div>

        <div className={styles.footer}>
          <button
            type="button"
            onClick={onClose}
            className={styles.cancelButton}
            disabled={importing}
          >
            取消
          </button>
          <button
            type="button"
            onClick={handleImport}
            className={styles.importButton}
            disabled={importing}
          >
            {importing ? '导入中...' : '导入Schema'}
          </button>
        </div>
      </div>
    </div>
  );
};

import React, { useState, useCallback, useRef } from 'react';
import { Upload, Button, Input, message, Spin, Alert } from 'antd';
import { InboxOutlined, FileTextOutlined, CopyOutlined, QuestionCircleOutlined } from '@ant-design/icons';
import type { UploadProps } from 'antd';
import type { RcFile } from 'antd/es/upload';
import styles from './ModuleSchemaV2.module.css';

const { TextArea } = Input;
const { Dragger } = Upload;

interface VariablesTfUploaderProps {
  value?: string;
  onChange?: (value: string) => void;
  onShowGuide?: () => void;
  disabled?: boolean;
}

const VariablesTfUploader: React.FC<VariablesTfUploaderProps> = ({
  value = '',
  onChange,
  onShowGuide,
  disabled = false,
}) => {
  const [loading, setLoading] = useState(false);
  const [inputMode, setInputMode] = useState<'upload' | 'paste'>('upload');
  const textAreaRef = useRef<HTMLTextAreaElement>(null);

  // 处理文件上传
  const handleUpload: UploadProps['customRequest'] = useCallback(async (options: {
    file: RcFile | string | Blob;
    onSuccess?: (body: unknown) => void;
    onError?: (err: Error) => void;
  }) => {
    const { file, onSuccess, onError } = options;
    
    setLoading(true);
    try {
      const reader = new FileReader();
      reader.onload = (e) => {
        const content = e.target?.result as string;
        onChange?.(content);
        onSuccess?.(null);
        message.success('文件上传成功');
        setLoading(false);
      };
      reader.onerror = () => {
        onError?.(new Error('文件读取失败'));
        message.error('文件读取失败');
        setLoading(false);
      };
      reader.readAsText(file as Blob);
    } catch (error) {
      onError?.(error as Error);
      message.error('文件上传失败');
      setLoading(false);
    }
  }, [onChange]);

  // 处理粘贴
  const handlePaste = useCallback((e: React.ClipboardEvent) => {
    const pastedText = e.clipboardData.getData('text');
    if (pastedText && pastedText.includes('variable')) {
      onChange?.(pastedText);
      message.success('内容已粘贴');
    }
  }, [onChange]);

  // 处理文本变化
  const handleTextChange = useCallback((e: React.ChangeEvent<HTMLTextAreaElement>) => {
    onChange?.(e.target.value);
  }, [onChange]);

  // 清空内容
  const handleClear = useCallback(() => {
    onChange?.('');
  }, [onChange]);

  // 加载示例
  const handleLoadExample = useCallback(() => {
    const example = `variable "name" {
  description = "The name of the resource" # @level:basic @alias:资源名称
  type        = string
}

variable "instance_type" {
  description = "EC2 instance type" # @level:basic @source:instance_types @widget:select
  type        = string
  default     = "t3.micro"
}

variable "tags" {
  description = "Tags to apply to resources" # @level:advanced @widget:key-value
  type        = map(string)
  default     = {}
}

variable "enable_monitoring" {
  description = "Enable detailed monitoring" # @level:advanced
  type        = bool
  default     = false
}`;
    onChange?.(example);
    message.info('已加载示例内容');
  }, [onChange]);

  const uploadProps: UploadProps = {
    name: 'file',
    multiple: false,
    accept: '.tf',
    showUploadList: false,
    customRequest: handleUpload,
    disabled: disabled || loading,
  };

  return (
    <div className={styles.uploaderContainer}>
      {/* 模式切换 */}
      <div className={styles.modeSwitch}>
        <Button
          type={inputMode === 'upload' ? 'primary' : 'default'}
          onClick={() => setInputMode('upload')}
          icon={<InboxOutlined />}
        >
          文件上传
        </Button>
        <Button
          type={inputMode === 'paste' ? 'primary' : 'default'}
          onClick={() => setInputMode('paste')}
          icon={<CopyOutlined />}
        >
          粘贴内容
        </Button>
        <Button
          type="link"
          icon={<QuestionCircleOutlined />}
          onClick={onShowGuide}
        >
          注释规范说明
        </Button>
      </div>

      <Spin spinning={loading}>
        {inputMode === 'upload' ? (
          <Dragger {...uploadProps} className={styles.dragger}>
            <p className="ant-upload-drag-icon">
              <InboxOutlined />
            </p>
            <p className="ant-upload-text">点击或拖拽 variables.tf 文件到此区域</p>
            <p className="ant-upload-hint">
              支持 Terraform 变量定义文件，系统将自动解析并生成 Schema
            </p>
          </Dragger>
        ) : (
          <div className={styles.pasteArea}>
            <TextArea
              ref={textAreaRef as any}
              value={value}
              onChange={handleTextChange}
              onPaste={handlePaste}
              placeholder={`粘贴 variables.tf 内容到此处...

示例:
variable "name" {
  description = "The name of the resource" # @level:basic
  type        = string
}
`}
              rows={12}
              disabled={disabled}
              className={styles.codeTextArea}
            />
          </div>
        )}
      </Spin>

      {/* 操作按钮 */}
      <div className={styles.uploaderActions}>
        <Button onClick={handleLoadExample} disabled={disabled}>
          <FileTextOutlined /> 加载示例
        </Button>
        {value && (
          <Button onClick={handleClear} danger disabled={disabled}>
            清空
          </Button>
        )}
      </div>

      {/* 内容预览 */}
      {value && (
        <Alert
          type="success"
          message={
            <span>
              已加载内容，共 {value.split('\n').length} 行，
              约 {(value.match(/variable\s+"/g) || []).length} 个变量定义
            </span>
          }
          className={styles.contentAlert}
        />
      )}
    </div>
  );
};

export default VariablesTfUploader;

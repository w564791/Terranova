import React, { useState } from 'react';
import { Upload, Button, Checkbox, Alert, Input } from 'antd';
import { UploadOutlined, WarningOutlined } from '@ant-design/icons';
import type { UploadFile } from 'antd/es/upload/interface';
import { stateAPI } from '../services/state';
import { useToast } from '../contexts/ToastContext';
import styles from './StateUpload.module.css';

const { TextArea } = Input;

interface StateUploadProps {
  workspaceId: string;
  onUploadSuccess?: () => void;
}

export const StateUpload: React.FC<StateUploadProps> = ({ workspaceId, onUploadSuccess }) => {
  const { showToast } = useToast();
  const [fileList, setFileList] = useState<UploadFile[]>([]);
  const [forceUpload, setForceUpload] = useState(false);
  const [showForceWarning, setShowForceWarning] = useState(false);
  const [description, setDescription] = useState('');
  const [uploading, setUploading] = useState(false);

  // 处理文件选择
  const handleFileChange = (info: any) => {
    let newFileList = [...info.fileList];
    // 只保留最新的一个文件
    newFileList = newFileList.slice(-1);
    setFileList(newFileList);
  };

  // 处理上传
  const handleUpload = async () => {
    if (fileList.length === 0) {
      showToast('请选择要上传的 State 文件', 'warning');
      return;
    }

    const file = fileList[0].originFileObj as File;
    if (!file) {
      showToast('文件读取失败', 'error');
      return;
    }

    setUploading(true);
    try {
      const response = await stateAPI.uploadStateFile(
        workspaceId,
        file,
        forceUpload,
        description
      );

      // 显示成功消息
      let successMsg = `State 上传成功！新版本: #${response.version}`;
      if (response.warnings && response.warnings.length > 0) {
        successMsg += ` (${response.warnings.length} 条警告)`;
      }
      showToast(successMsg, 'success');

      // 重置表单
      setFileList([]);
      setForceUpload(false);
      setShowForceWarning(false);
      setDescription('');

      // 触发回调
      if (onUploadSuccess) {
        onUploadSuccess();
      }
    } catch (error: any) {
      // API 拦截器返回的是字符串错误消息
      const errText = typeof error === 'string' ? error : (error?.message || '');
      
      // 格式化错误消息
      let errorMsg = '上传失败';
      
      if (errText) {
        if (errText.includes('serial must be greater')) {
          const match = errText.match(/current \((\d+)\), got (\d+)/);
          if (match) {
            errorMsg = `版本号冲突：当前 ${match[1]}，上传 ${match[2]}，请勾选"强制上传"`;
          } else {
            errorMsg = '版本号冲突，请勾选"强制上传"跳过校验';
          }
        } else if (errText.includes('lineage mismatch')) {
          errorMsg = 'Lineage 不匹配，请勾选"强制上传"跳过校验';
        } else if (errText.includes('missing required field')) {
          errorMsg = 'State 文件格式错误：缺少 lineage 或 serial';
        } else if (errText.includes('workspace is already locked')) {
          errorMsg = 'Workspace 已被锁定，请先解锁';
        } else {
          errorMsg = `上传失败: ${errText}`;
        }
      }
      
      showToast(errorMsg, 'error');
    } finally {
      setUploading(false);
    }
  };

  return (
    <div className={styles.container}>
      <h3>Upload Terraform State</h3>

      <div className={styles.uploadSection}>
        <Upload
          fileList={fileList}
          onChange={handleFileChange}
          beforeUpload={() => false}
          accept=".tfstate,.json"
          maxCount={1}
        >
          <Button icon={<UploadOutlined />}>Select State File</Button>
        </Upload>

        <div className={styles.description}>
          <TextArea
            placeholder="上传说明（可选）"
            value={description}
            onChange={(e) => setDescription(e.target.value)}
            rows={2}
            maxLength={200}
            showCount
          />
        </div>

        <div className={styles.options}>
          <Checkbox
            checked={forceUpload}
            onChange={(e) => {
              const checked = e.target.checked;
              setForceUpload(checked);
              setShowForceWarning(checked);
            }}
          >
            Force Upload (跳过校验)
          </Checkbox>
        </div>

        {showForceWarning && (
          <Alert
            type="error"
            className={styles.forceWarning}
            icon={<WarningOutlined />}
            message="⚠️ 危险操作警告"
            description={
              <div>
                <p>
                  <strong>强制上传将跳过所有安全校验，可能导致：</strong>
                </p>
                <ul>
                  <li>覆盖其他用户的更改</li>
                  <li>State 不一致</li>
                  <li>资源管理混乱</li>
                </ul>
                <p>
                  <strong>请确认：</strong>
                </p>
                <ul>
                  <li>✓ 已备份当前 state</li>
                  <li>✓ 了解此操作的风险</li>
                  <li>✓ 已与团队成员确认</li>
                </ul>
              </div>
            }
          />
        )}

        <Button
          type="primary"
          onClick={handleUpload}
          disabled={fileList.length === 0}
          loading={uploading}
          danger={forceUpload}
          className={styles.uploadButton}
        >
          {forceUpload ? '强制上传 State' : '上传 State'}
        </Button>
      </div>
    </div>
  );
};

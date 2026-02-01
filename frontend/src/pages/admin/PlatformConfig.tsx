import React, { useState, useEffect } from 'react';
import { useToast } from '../../hooks/useToast';
import api from '../../services/api';
import styles from './PlatformConfig.module.css';

interface PlatformConfigData {
  base_url: string;
  protocol: string;
  host: string;
  api_port: string;
  cc_port: string;
}

const PlatformConfig: React.FC = () => {
  const [config, setConfig] = useState<PlatformConfigData>({
    base_url: '',
    protocol: 'http',
    host: 'localhost',
    api_port: '8080',
    cc_port: '8081',
  });
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [useBaseUrl, setUseBaseUrl] = useState(true);
  const [isEditing, setIsEditing] = useState(false);
  const { showToast } = useToast();

  useEffect(() => {
    loadConfig();
  }, []);

  const loadConfig = async () => {
    try {
      setLoading(true);
      const data: PlatformConfigData = await api.get('/global/settings/platform-config');
      setConfig(data);
      // 如果 base_url 存在且不为空，使用 base_url 模式
      setUseBaseUrl(!!data.base_url);
    } catch (error: any) {
      showToast(error.response?.data?.error || '加载配置失败', 'error');
    } finally {
      setLoading(false);
    }
  };

  const handleSave = async () => {
    try {
      setSaving(true);
      
      // 始终同步更新所有字段，确保数据一致性
      const updateData: Partial<PlatformConfigData> = {
        protocol: config.protocol,
        host: config.host,
        api_port: config.api_port,
        cc_port: config.cc_port,
      };
      
      if (useBaseUrl) {
        // 使用完整 URL 模式 - base_url 已经通过 handleBaseUrlChange 解析并同步到分离字段
        updateData.base_url = config.base_url;
      } else {
        // 使用分离配置模式 - 同时更新 base_url
        updateData.base_url = `${config.protocol}://${config.host}:${config.api_port}`;
      }
      
      await api.put('/global/settings/platform-config', updateData);
      showToast('配置保存成功，新创建的 Pod 将使用新配置', 'success');
      setIsEditing(false);
    } catch (error: any) {
      showToast(error.response?.data?.error || '保存配置失败', 'error');
    } finally {
      setSaving(false);
    }
  };

  const handleCancel = () => {
    loadConfig();
    setIsEditing(false);
  };

  const handleBaseUrlChange = (value: string) => {
    setConfig({ ...config, base_url: value });
    
    // 尝试解析 URL 并更新分离字段
    try {
      const url = new URL(value);
      setConfig({
        ...config,
        base_url: value,
        protocol: url.protocol.replace(':', ''),
        host: url.hostname,
        api_port: url.port || (url.protocol === 'https:' ? '443' : '80'),
      });
    } catch {
      // URL 解析失败，只更新 base_url
      setConfig({ ...config, base_url: value });
    }
  };

  const buildBaseUrl = () => {
    return `${config.protocol}://${config.host}:${config.api_port}`;
  };

  if (loading) {
    return (
      <div className={styles.container}>
        <div className={styles.loading}>加载中...</div>
      </div>
    );
  }

  return (
    <div className={styles.container}>
      <div className={styles.header}>
        <h1 className={styles.title}>平台配置</h1>
        <p className={styles.description}>
          配置平台的网络地址，用于 Run Task 回调和 Agent 连接。修改后需要重启后端服务才能生效。
        </p>
      </div>

      <div className={styles.content}>
        <div className={styles.section}>
          <h2 className={styles.sectionTitle}>平台地址配置</h2>
          
          {isEditing ? (
            <>
              <div className={styles.modeSwitch}>
                <label className={styles.radioLabel}>
                  <input
                    type="radio"
                    checked={useBaseUrl}
                    onChange={() => setUseBaseUrl(true)}
                  />
                  <span>使用完整 URL</span>
                </label>
                <label className={styles.radioLabel}>
                  <input
                    type="radio"
                    checked={!useBaseUrl}
                    onChange={() => setUseBaseUrl(false)}
                  />
                  <span>分离配置</span>
                </label>
              </div>

              {useBaseUrl ? (
                <div className={styles.formGroup}>
                  <label className={styles.label}>平台基础 URL</label>
                  <input
                    type="text"
                    className={styles.input}
                    value={config.base_url}
                    onChange={(e) => handleBaseUrlChange(e.target.value)}
                    placeholder="http://your-server:8080"
                  />
                  <span className={styles.hint}>
                    完整的平台访问地址，例如：http://192.168.1.100:8080 或 https://iac.example.com
                  </span>
                </div>
              ) : (
                <>
                  <div className={styles.formRow}>
                    <div className={styles.formGroup}>
                      <label className={styles.label}>协议</label>
                      <select
                        className={styles.select}
                        value={config.protocol}
                        onChange={(e) => setConfig({ ...config, protocol: e.target.value })}
                      >
                        <option value="http">HTTP</option>
                        <option value="https">HTTPS</option>
                      </select>
                    </div>
                    
                    <div className={styles.formGroup} style={{ flex: 2 }}>
                      <label className={styles.label}>主机地址</label>
                      <input
                        type="text"
                        className={styles.input}
                        value={config.host}
                        onChange={(e) => setConfig({ ...config, host: e.target.value })}
                        placeholder="localhost 或 192.168.1.100"
                      />
                    </div>
                    
                    <div className={styles.formGroup}>
                      <label className={styles.label}>API 端口</label>
                      <input
                        type="text"
                        className={styles.input}
                        value={config.api_port}
                        onChange={(e) => setConfig({ ...config, api_port: e.target.value })}
                        placeholder="8080"
                      />
                    </div>
                  </div>
                  
                  <div className={styles.preview}>
                    <span className={styles.previewLabel}>预览：</span>
                    <code className={styles.previewUrl}>{buildBaseUrl()}</code>
                  </div>
                </>
              )}
            </>
          ) : (
            <div className={styles.readOnlyValue}>
              <label className={styles.label}>平台基础 URL</label>
              <code className={styles.valueCode}>{config.base_url || buildBaseUrl()}</code>
            </div>
          )}
        </div>

        <div className={styles.section}>
          <h2 className={styles.sectionTitle}>Agent 控制通道配置</h2>
          
          {isEditing ? (
            <div className={styles.formGroup}>
              <label className={styles.label}>CC 端口</label>
              <input
                type="text"
                className={styles.input}
                value={config.cc_port}
                onChange={(e) => setConfig({ ...config, cc_port: e.target.value })}
                placeholder="8081"
                style={{ maxWidth: '200px' }}
              />
              <span className={styles.hint}>
                Agent 控制通道（WebSocket）端口，默认为 API 端口 + 10
              </span>
            </div>
          ) : (
            <div className={styles.readOnlyValue}>
              <label className={styles.label}>CC 端口</label>
              <code className={styles.valueCode}>{config.cc_port}</code>
            </div>
          )}
        </div>

        <div className={styles.section}>
          <h2 className={styles.sectionTitle}>当前配置摘要</h2>
          <div className={styles.summary}>
            <div className={styles.summaryItem}>
              <span className={styles.summaryLabel}>Run Task 回调地址：</span>
              <code>{useBaseUrl ? config.base_url : buildBaseUrl()}/api/v1/run-task-results/...</code>
            </div>
            <div className={styles.summaryItem}>
              <span className={styles.summaryLabel}>Agent API 地址：</span>
              <code>{useBaseUrl ? config.base_url : buildBaseUrl()}/api/v1/agents/...</code>
            </div>
            <div className={styles.summaryItem}>
              <span className={styles.summaryLabel}>Agent CC 地址：</span>
              <code>ws://{config.host}:{config.cc_port}/ws/agent</code>
            </div>
          </div>
        </div>

        <div className={styles.actions}>
          {isEditing ? (
            <>
              <button
                className={styles.saveButton}
                onClick={handleSave}
                disabled={saving}
              >
                {saving ? '保存中...' : '保存配置'}
              </button>
              <button
                className={styles.resetButton}
                onClick={handleCancel}
                disabled={saving}
              >
                取消
              </button>
            </>
          ) : (
            <button
              className={styles.editButton}
              onClick={() => setIsEditing(true)}
            >
              编辑
            </button>
          )}
        </div>

        <div className={styles.notice}>
          <strong>⚠️ 注意：</strong>
          修改配置后需要重启后端服务才能生效。如果您使用的是 Docker 部署，请重启相关容器。
        </div>
      </div>
    </div>
  );
};

export default PlatformConfig;

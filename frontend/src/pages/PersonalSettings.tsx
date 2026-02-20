import React, { useState, useEffect } from 'react';
import { useNavigate, useSearchParams } from 'react-router-dom';
import api from '../services/api';
import { getMFAStatus, getMFAConfig } from '../services/mfaService';
import type { MFAStatus, MFAConfig } from '../services/mfaService';
import styles from './PersonalSettings.module.css';

interface UserToken {
  token_name: string;
  is_active: boolean;
  created_at: string;
  revoked_at?: string;
  last_used_at?: string;
  expires_at?: string;
}

interface TokenCreateResponse {
  token_id: string;
  token_name: string;
  token: string;
  created_at: string;
  expires_at?: string;
}

const PersonalSettings: React.FC = () => {
  const navigate = useNavigate();
  const [searchParams, setSearchParams] = useSearchParams();
  const [activeTab, setActiveTab] = useState<'password' | 'tokens' | 'mfa'>(() => {
    const tab = searchParams.get('tab');
    return (tab === 'tokens' || tab === 'password' || tab === 'mfa') ? tab : 'password';
  });
  
  // Password change state
  const [oldPassword, setOldPassword] = useState('');
  const [newPassword, setNewPassword] = useState('');
  const [confirmPassword, setConfirmPassword] = useState('');
  const [passwordMessage, setPasswordMessage] = useState('');
  const [passwordError, setPasswordError] = useState('');
  
  // Token management state
  const [tokens, setTokens] = useState<UserToken[]>([]);
  const [showCreateToken, setShowCreateToken] = useState(false);
  const [tokenName, setTokenName] = useState('');
  const [expiresInDays, setExpiresInDays] = useState(90);
  const [createdToken, setCreatedToken] = useState<TokenCreateResponse | null>(null);
  const [tokenMessage, setTokenMessage] = useState('');
  const [tokenError, setTokenError] = useState('');
  const [loading, setLoading] = useState(false);
  
  // MFA state
  const [mfaStatus, setMfaStatus] = useState<MFAStatus | null>(null);
  const [mfaConfig, setMfaConfig] = useState<MFAConfig | null>(null);
  const [mfaLoading, setMfaLoading] = useState(false);

  useEffect(() => {
    if (activeTab === 'tokens') {
      loadTokens();
    }
    if (activeTab === 'mfa') {
      loadMFAStatus();
      loadMFAConfig();
    }
  }, [activeTab]);

  const loadMFAStatus = async () => {
    try {
      setMfaLoading(true);
      const response: any = await getMFAStatus();
      setMfaStatus(response.data);
    } catch (error) {
      console.error('加载MFA状态失败:', error);
    } finally {
      setMfaLoading(false);
    }
  };

  const loadMFAConfig = async () => {
    try {
      const response: any = await getMFAConfig();
      setMfaConfig(response.data?.config);
    } catch (error) {
      console.error('加载MFA配置失败:', error);
    }
  };

  // 判断备用码是否启用
  const isBackupCodesEnabled = mfaConfig?.required_backup_codes !== 0;

  const loadTokens = async () => {
    try {
      const response = await api.get('/user/tokens');
      setTokens(response.data || []);
      setTokenError(''); // 清除之前的错误
    } catch (error: any) {
      console.error('加载Token列表失败:', error);
      setTokenError(error.error || '加载Token列表失败，请稍后重试');
    }
  };

  const handlePasswordChange = async (e: React.FormEvent) => {
    e.preventDefault();
    setPasswordMessage('');
    setPasswordError('');

    if (newPassword !== confirmPassword) {
      setPasswordError('新密码和确认密码不匹配');
      return;
    }

    if (newPassword.length < 6) {
      setPasswordError('新密码长度至少为6个字符');
      return;
    }

    setLoading(true);
    try {
      await api.post('/user/change-password', {
        old_password: oldPassword,
        new_password: newPassword,
      });
      setPasswordMessage('密码修改成功');
      setOldPassword('');
      setNewPassword('');
      setConfirmPassword('');
    } catch (error: any) {
      setPasswordError(error.error || '密码修改失败，请检查当前密码是否正确');
    } finally {
      setLoading(false);
    }
  };

  const handleCreateToken = async (e: React.FormEvent) => {
    e.preventDefault();
    setTokenMessage('');
    setTokenError('');

    if (!tokenName.trim()) {
      setTokenError('请输入Token名称');
      return;
    }

    setLoading(true);
    try {
      const response = await api.post('/user/tokens', {
        token_name: tokenName,
        expires_in_days: expiresInDays,
      });
      setCreatedToken(response.data);
      setTokenName('');
      setShowCreateToken(false);
      loadTokens();
    } catch (error: any) {
      setTokenError(error.error || '创建Token失败，请稍后重试');
    } finally {
      setLoading(false);
    }
  };

  const [revokeConfirm, setRevokeConfirm] = useState<string | null>(null);

  const handleRevokeToken = async (tokenName: string) => {
    try {
      await api.delete(`/user/tokens/${encodeURIComponent(tokenName)}`);
      setTokenMessage('Token已成功吊销');
      setRevokeConfirm(null);
      loadTokens();
    } catch (error: any) {
      setTokenError(error.error || '吊销Token失败，请稍后重试');
      setRevokeConfirm(null);
    }
  };

  const copyToClipboard = (text: string) => {
    navigator.clipboard.writeText(text);
    setTokenMessage('Token已复制到剪贴板');
    setTimeout(() => setTokenMessage(''), 3000);
  };

  const formatDate = (dateString?: string) => {
    if (!dateString) return '-';
    return new Date(dateString).toLocaleString('zh-CN');
  };

  return (
    <div className={styles.container}>
      <div className={styles.header}>
        <h1>个人设置</h1>
        <button onClick={() => navigate(-1)} className={styles.backButton}>
          返回
        </button>
      </div>

      <div className={styles.tabs}>
        <button
          className={`${styles.tab} ${activeTab === 'password' ? styles.activeTab : ''}`}
          onClick={() => {
            setActiveTab('password');
            setSearchParams({ tab: 'password' });
          }}
        >
          修改密码
        </button>
        <button
          className={`${styles.tab} ${activeTab === 'tokens' ? styles.activeTab : ''}`}
          onClick={() => {
            setActiveTab('tokens');
            setSearchParams({ tab: 'tokens' });
          }}
        >
          访问Token
        </button>
        <button
          className={`${styles.tab} ${activeTab === 'mfa' ? styles.activeTab : ''}`}
          onClick={() => {
            setActiveTab('mfa');
            setSearchParams({ tab: 'mfa' });
          }}
        >
          多因素认证
        </button>
      </div>

      <div className={styles.content}>
        {activeTab === 'password' && (
          <div className={styles.section}>
            <h2>修改密码</h2>
            <form onSubmit={handlePasswordChange} className={styles.form}>
              <div className={styles.formGroup}>
                <label htmlFor="oldPassword">当前密码</label>
                <input
                  type="password"
                  id="oldPassword"
                  value={oldPassword}
                  onChange={(e) => setOldPassword(e.target.value)}
                  required
                  className={styles.input}
                />
              </div>

              <div className={styles.formGroup}>
                <label htmlFor="newPassword">新密码</label>
                <input
                  type="password"
                  id="newPassword"
                  value={newPassword}
                  onChange={(e) => setNewPassword(e.target.value)}
                  required
                  minLength={6}
                  className={styles.input}
                />
                <small className={styles.hint}>密码长度至少6个字符</small>
              </div>

              <div className={styles.formGroup}>
                <label htmlFor="confirmPassword">确认新密码</label>
                <input
                  type="password"
                  id="confirmPassword"
                  value={confirmPassword}
                  onChange={(e) => setConfirmPassword(e.target.value)}
                  required
                  className={styles.input}
                />
              </div>

              {passwordMessage && (
                <div className={styles.successMessage}>{passwordMessage}</div>
              )}
              {passwordError && (
                <div className={styles.errorMessage}>{passwordError}</div>
              )}

              <button
                type="submit"
                disabled={loading}
                className={styles.submitButton}
              >
                {loading ? '处理中...' : '修改密码'}
              </button>
            </form>
          </div>
        )}

        {activeTab === 'tokens' && (
          <div className={styles.section}>
            <div className={styles.sectionHeader}>
              <h2>访问Token管理</h2>
              <button
                onClick={() => setShowCreateToken(true)}
                className={styles.createButton}
                disabled={tokens.filter(t => t.is_active).length >= 2}
              >
                创建新Token
              </button>
            </div>

            <p className={styles.description}>
              访问Token可用于API调用和自动化脚本。每个用户最多可以创建2个有效Token。
            </p>

            {tokenMessage && (
              <div className={styles.successMessage}>{tokenMessage}</div>
            )}

            {createdToken && (
              <div className={styles.tokenCreated}>
                <h3>Token创建成功</h3>
                <p className={styles.warning}>
                   请立即复制并保存此Token，关闭后将无法再次查看！
                </p>
                <div className={styles.tokenDisplay}>
                  <code>{createdToken.token}</code>
                  <button
                    onClick={() => copyToClipboard(createdToken.token)}
                    className={styles.copyButton}
                  >
                    复制
                  </button>
                </div>
                <button
                  onClick={() => setCreatedToken(null)}
                  className={styles.closeButton}
                >
                  我已保存，关闭
                </button>
              </div>
            )}

            {showCreateToken && (
              <div className={styles.createTokenForm}>
                <h3>创建新Token</h3>
                <form onSubmit={handleCreateToken} className={styles.form}>
                  <div className={styles.formGroup}>
                    <label htmlFor="tokenName">Token名称</label>
                    <input
                      type="text"
                      id="tokenName"
                      value={tokenName}
                      onChange={(e) => setTokenName(e.target.value)}
                      required
                      placeholder="例如：CI/CD Pipeline"
                      className={styles.input}
                    />
                  </div>

                  <div className={styles.formGroup}>
                    <label htmlFor="expiresInDays">过期时间</label>
                    <select
                      id="expiresInDays"
                      value={expiresInDays}
                      onChange={(e) => setExpiresInDays(Number(e.target.value))}
                      className={styles.select}
                    >
                      <option value={30}>30天</option>
                      <option value={60}>60天</option>
                      <option value={90}>90天</option>
                      <option value={180}>180天</option>
                      <option value={365}>365天</option>
                      <option value={0}>永不过期</option>
                    </select>
                  </div>

                  <div className={styles.formActions}>
                    <button
                      type="button"
                      onClick={() => {
                        setShowCreateToken(false);
                        setTokenName('');
                      }}
                      className={styles.cancelButton}
                    >
                      取消
                    </button>
                    <button
                      type="submit"
                      disabled={loading}
                      className={styles.submitButton}
                    >
                      {loading ? '创建中...' : '创建Token'}
                    </button>
                  </div>
                </form>
              </div>
            )}

            <div className={styles.tokenList}>
              {tokenError ? (
                <div className={styles.errorState}>
                  <p className={styles.errorIcon}></p>
                  <p className={styles.errorText}>{tokenError}</p>
                  <button onClick={loadTokens} className={styles.retryButton}>
                    重试
                  </button>
                </div>
              ) : tokens.length === 0 ? (
                <div className={styles.emptyState}>
                  <p className={styles.emptyIcon}></p>
                  <p className={styles.emptyText}>您还没有创建任何Token</p>
                  <p className={styles.emptyHint}>点击上方"创建新Token"按钮开始创建</p>
                </div>
              ) : (
                <table className={styles.table}>
                  <thead>
                    <tr>
                      <th>名称</th>
                      <th>状态</th>
                      <th>创建时间</th>
                      <th>最后使用</th>
                      <th>过期时间</th>
                      <th>操作</th>
                    </tr>
                  </thead>
                  <tbody>
                    {tokens.map((token) => (
                      <tr key={token.token_name}>
                        <td>{token.token_name}</td>
                        <td>
                          <span
                            className={`${styles.status} ${
                              token.is_active ? styles.active : styles.revoked
                            }`}
                          >
                            {token.is_active ? '有效' : '已吊销'}
                          </span>
                        </td>
                        <td>{formatDate(token.created_at)}</td>
                        <td>{formatDate(token.last_used_at)}</td>
                        <td>{formatDate(token.expires_at)}</td>
                        <td>
                          {token.is_active && (
                            <>
                              {revokeConfirm === token.token_name ? (
                                <div className={styles.confirmActions}>
                                  <button
                                    onClick={() => handleRevokeToken(token.token_name)}
                                    className={styles.confirmButton}
                                  >
                                    确认吊销
                                  </button>
                                  <button
                                    onClick={() => setRevokeConfirm(null)}
                                    className={styles.cancelSmallButton}
                                  >
                                    取消
                                  </button>
                                </div>
                              ) : (
                                <button
                                  onClick={() => setRevokeConfirm(token.token_name)}
                                  className={styles.revokeButton}
                                >
                                  吊销
                                </button>
                              )}
                            </>
                          )}
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              )}
            </div>
          </div>
        )}

        {activeTab === 'mfa' && (
          <div className={styles.section}>
            <h2>多因素认证（MFA）</h2>
            <p className={styles.description}>
              多因素认证通过要求额外的验证码来增强账户安全性。启用后，登录时除了密码外，还需要输入Authenticator应用生成的验证码。
            </p>

            {mfaLoading ? (
              <div className={styles.loadingState}>
                <p>加载中...</p>
              </div>
            ) : mfaStatus ? (
              <div className={styles.mfaContent}>
                <div className={styles.mfaStatusCard}>
                  <div className={styles.mfaStatusHeader}>
                    <span className={`${styles.mfaStatusBadge} ${mfaStatus.mfa_enabled ? styles.mfaEnabled : styles.mfaDisabled}`}>
                      {mfaStatus.mfa_enabled ? '已启用' : '未启用'}
                    </span>
                  </div>
                  {mfaStatus.mfa_enabled && mfaStatus.mfa_verified_at && (
                    <p className={styles.mfaStatusTime}>
                      启用时间：{formatDate(mfaStatus.mfa_verified_at)}
                    </p>
                  )}

                  {mfaStatus.mfa_enabled && (
                    <div className={styles.mfaInfo}>
                      {isBackupCodesEnabled && (
                        <div className={styles.mfaInfoItem}>
                          <span className={styles.mfaInfoLabel}>剩余备用恢复码</span>
                          <span className={styles.mfaInfoValue}>{mfaStatus.backup_codes_count} 个</span>
                        </div>
                      )}
                      <div className={styles.mfaInfoItem}>
                        <span className={styles.mfaInfoLabel}>强制策略</span>
                        <span className={styles.mfaInfoValue}>
                          {mfaStatus.enforcement_policy === 'optional' && '可选'}
                          {mfaStatus.enforcement_policy === 'required_new' && '新用户必须'}
                          {mfaStatus.enforcement_policy === 'required_all' && '所有用户必须'}
                        </span>
                      </div>
                    </div>
                  )}

                  {mfaStatus.is_required && !mfaStatus.mfa_enabled && (
                    <div className={styles.mfaWarning}>
                      根据安全策略，您需要启用多因素认证
                    </div>
                  )}
                </div>

                <div className={styles.mfaActions}>
                  {mfaStatus.mfa_enabled ? (
                    <button
                      onClick={() => navigate('/settings/mfa')}
                      className={styles.mfaManageButton}
                    >
                      管理多因素认证
                    </button>
                  ) : (
                    <button
                      onClick={() => navigate('/settings/mfa')}
                      className={styles.mfaEnableButton}
                    >
                      启用多因素认证
                    </button>
                  )}
                </div>

                <div className={styles.mfaTips}>
                  <h4>支持的Authenticator应用</h4>
                  <ul>
                    <li>Google Authenticator</li>
                    <li>Microsoft Authenticator</li>
                    <li>Authy</li>
                    <li>其他支持TOTP标准的应用</li>
                  </ul>
                </div>
              </div>
            ) : (
              <div className={styles.errorState}>
                <p className={styles.errorText}>加载MFA状态失败</p>
                <button onClick={loadMFAStatus} className={styles.retryButton}>
                  重试
                </button>
              </div>
            )}
          </div>
        )}
      </div>
    </div>
  );
};

export default PersonalSettings;

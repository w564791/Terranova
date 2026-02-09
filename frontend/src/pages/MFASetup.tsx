import React, { useState, useEffect, useRef } from 'react';
import { Card, Steps, Button, Input, message, Alert, Typography, Space, Spin, Modal, Divider } from 'antd';
import type { InputRef } from 'antd';
import { SafetyCertificateOutlined, QrcodeOutlined, KeyOutlined, CheckCircleOutlined, CopyOutlined, DownloadOutlined, ReloadOutlined, LockOutlined, UnlockOutlined } from '@ant-design/icons';
import { useNavigate } from 'react-router-dom';
import { getMFAStatus, setupMFA, verifyAndEnableMFA, disableMFA, regenerateBackupCodes, getMFAConfig } from '../services/mfaService';
import type { MFAStatus, MFASetupResponse, MFAConfig } from '../services/mfaService';
import styles from './MFASetup.module.css';

const { Title, Text, Paragraph } = Typography;
const { Step } = Steps;

const MFASetup: React.FC = () => {
  const navigate = useNavigate();
  const [loading, setLoading] = useState(true);
  const [mfaStatus, setMfaStatus] = useState<MFAStatus | null>(null);
  const [mfaConfig, setMfaConfig] = useState<MFAConfig | null>(null);
  const [setupData, setSetupData] = useState<MFASetupResponse | null>(null);
  const [currentStep, setCurrentStep] = useState(0);
  const [verifyCode, setVerifyCode] = useState('');
  const [verifying, setVerifying] = useState(false);
  const [showDisableModal, setShowDisableModal] = useState(false);
  const [disableCode, setDisableCode] = useState('');
  const [disablePassword, setDisablePassword] = useState('');
  const [disabling, setDisabling] = useState(false);
  const [showRegenerateModal, setShowRegenerateModal] = useState(false);
  const [regenerateCode, setRegenerateCode] = useState('');
  const [regenerating, setRegenerating] = useState(false);
  const [newBackupCodes, setNewBackupCodes] = useState<string[]>([]);
  const inputRefs = useRef<(InputRef | null)[]>([]);

  useEffect(() => {
    loadMFAStatus();
    loadMFAConfig();
  }, []);

  const loadMFAStatus = async () => {
    try {
      setLoading(true);
      const response: any = await getMFAStatus();
      setMfaStatus(response.data);
    } catch (error) {
      message.error('获取MFA状态失败');
    } finally {
      setLoading(false);
    }
  };

  const loadMFAConfig = async () => {
    try {
      const response: any = await getMFAConfig();
      setMfaConfig(response.data?.config);
    } catch (error) {
      // 获取配置失败，使用默认值
      console.error('获取MFA配置失败:', error);
    }
  };

  // 判断备用码是否启用
  const isBackupCodesEnabled = mfaConfig?.required_backup_codes !== 0;

  const handleStartSetup = async () => {
    try {
      setLoading(true);
      const response: any = await setupMFA();
      setSetupData(response.data);
      setCurrentStep(1);
    } catch (error: any) {
      message.error(error || '初始化MFA设置失败');
    } finally {
      setLoading(false);
    }
  };

  const handleVerify = async () => {
    if (verifyCode.length !== 6) {
      message.error('请输入6位验证码');
      return;
    }

    try {
      setVerifying(true);
      await verifyAndEnableMFA(verifyCode);
      message.success('MFA已成功启用！');
      setCurrentStep(2);
      loadMFAStatus();
    } catch (error: any) {
      message.error(error || '验证失败，请检查验证码');
    } finally {
      setVerifying(false);
    }
  };

  const handleDisableMFA = async () => {
    if (disableCode.length !== 6) {
      message.error('请输入6位验证码');
      return;
    }
    if (!disablePassword) {
      message.error('请输入密码');
      return;
    }

    try {
      setDisabling(true);
      await disableMFA(disableCode, disablePassword);
      message.success('MFA已禁用');
      setShowDisableModal(false);
      setDisableCode('');
      setDisablePassword('');
      loadMFAStatus();
      setSetupData(null);
      setCurrentStep(0);
    } catch (error: any) {
      message.error(error || '禁用MFA失败');
    } finally {
      setDisabling(false);
    }
  };

  const handleRegenerateBackupCodes = async () => {
    if (regenerateCode.length !== 6) {
      message.error('请输入6位验证码');
      return;
    }

    try {
      setRegenerating(true);
      const response: any = await regenerateBackupCodes(regenerateCode);
      setNewBackupCodes(response.data.backup_codes);
      message.success('备用恢复码已重新生成');
      loadMFAStatus();
    } catch (error: any) {
      message.error(error || '重新生成备用恢复码失败');
    } finally {
      setRegenerating(false);
    }
  };

  const copyToClipboard = (text: string) => {
    navigator.clipboard.writeText(text);
    message.success('已复制到剪贴板');
  };

  const downloadBackupCodes = (codes: string[]) => {
    const content = `IaC Platform 备用恢复码\n生成时间: ${new Date().toLocaleString()}\n\n每个恢复码只能使用一次，请妥善保管。\n\n${codes.join('\n')}`;
    const blob = new Blob([content], { type: 'text/plain' });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = 'iac-platform-backup-codes.txt';
    a.click();
    URL.revokeObjectURL(url);
  };

  const handleCodeInput = (value: string, index: number) => {
    const newCode = verifyCode.split('');
    newCode[index] = value;
    setVerifyCode(newCode.join(''));
    
    // 自动跳转到下一个输入框
    if (value && index < 5) {
      inputRefs.current[index + 1]?.focus();
    }
  };

  if (loading && !setupData) {
    return (
      <div className={styles.container}>
        <Card className={styles.card}>
          <div className={styles.loading}>
            <Spin size="large" />
            <Text>加载中...</Text>
          </div>
        </Card>
      </div>
    );
  }

  // 已启用MFA的状态页面
  if (mfaStatus?.mfa_enabled && currentStep === 0) {
    return (
      <div className={styles.container}>
        <Card className={styles.card}>
          <div className={styles.header}>
            <SafetyCertificateOutlined className={styles.headerIcon} style={{ color: '#52c41a' }} />
            <Title level={3}>多因素认证已启用</Title>
          </div>

          <Alert
            message="您的账户已受到多因素认证保护"
            description={`启用时间: ${mfaStatus.mfa_verified_at ? new Date(mfaStatus.mfa_verified_at).toLocaleString() : '未知'}`}
            type="success"
            showIcon
            className={styles.alert}
          />

          <div className={styles.statusInfo}>
            {isBackupCodesEnabled && (
              <div className={styles.statusItem}>
                <Text type="secondary">剩余备用恢复码</Text>
                <Text strong>{mfaStatus.backup_codes_count} 个</Text>
              </div>
            )}
            <div className={styles.statusItem}>
              <Text type="secondary">强制策略</Text>
              <Text strong>
                {mfaStatus.enforcement_policy === 'optional' && '可选'}
                {mfaStatus.enforcement_policy === 'required_new' && '新用户必须'}
                {mfaStatus.enforcement_policy === 'required_all' && '所有用户必须'}
              </Text>
            </div>
          </div>

          <Divider />

          <Space direction="vertical" style={{ width: '100%' }}>
            {isBackupCodesEnabled && (
              <Button
                icon={<ReloadOutlined />}
                onClick={() => setShowRegenerateModal(true)}
                block
              >
                重新生成备用恢复码
              </Button>
            )}
            
            {mfaStatus.enforcement_policy !== 'required_all' && (
              <Button
                danger
                icon={<UnlockOutlined />}
                onClick={() => setShowDisableModal(true)}
                block
              >
                禁用多因素认证
              </Button>
            )}
          </Space>
        </Card>

        {/* 禁用MFA弹窗 */}
        <Modal
          title="禁用多因素认证"
          open={showDisableModal}
          onCancel={() => {
            setShowDisableModal(false);
            setDisableCode('');
            setDisablePassword('');
          }}
          footer={null}
        >
          <Alert
            message="警告"
            description="禁用多因素认证会降低您账户的安全性。"
            type="warning"
            showIcon
            className={styles.alert}
          />
          <div className={styles.formItem}>
            <Text>验证码</Text>
            <Input
              placeholder="请输入Authenticator中的6位验证码"
              value={disableCode}
              onChange={(e) => setDisableCode(e.target.value.replace(/\D/g, '').slice(0, 6))}
              maxLength={6}
            />
          </div>
          <div className={styles.formItem}>
            <Text>密码</Text>
            <Input.Password
              placeholder="请输入您的登录密码"
              value={disablePassword}
              onChange={(e) => setDisablePassword(e.target.value)}
            />
          </div>
          <Button
            type="primary"
            danger
            loading={disabling}
            onClick={handleDisableMFA}
            block
          >
            确认禁用
          </Button>
        </Modal>

        {/* 重新生成备用恢复码弹窗 */}
        <Modal
          title="重新生成备用恢复码"
          open={showRegenerateModal}
          onCancel={() => {
            setShowRegenerateModal(false);
            setRegenerateCode('');
            setNewBackupCodes([]);
          }}
          footer={null}
          width={500}
        >
          {newBackupCodes.length === 0 ? (
            <>
              <Alert
                message="注意"
                description="重新生成后，旧的备用恢复码将失效。"
                type="warning"
                showIcon
                className={styles.alert}
              />
              <div className={styles.formItem}>
                <Text>验证码</Text>
                <Input
                  placeholder="请输入Authenticator中的6位验证码"
                  value={regenerateCode}
                  onChange={(e) => setRegenerateCode(e.target.value.replace(/\D/g, '').slice(0, 6))}
                  maxLength={6}
                />
              </div>
              <Button
                type="primary"
                loading={regenerating}
                onClick={handleRegenerateBackupCodes}
                block
              >
                生成新的恢复码
              </Button>
            </>
          ) : (
            <>
              <Alert
                message="请保存您的新备用恢复码"
                description="每个恢复码只能使用一次，请妥善保管。"
                type="success"
                showIcon
                className={styles.alert}
              />
              <div className={styles.backupCodes}>
                {newBackupCodes.map((code, index) => (
                  <div key={index} className={styles.backupCode}>
                    <code>{code}</code>
                  </div>
                ))}
              </div>
              <Space style={{ width: '100%', justifyContent: 'center' }}>
                <Button
                  icon={<CopyOutlined />}
                  onClick={() => copyToClipboard(newBackupCodes.join('\n'))}
                >
                  复制全部
                </Button>
                <Button
                  icon={<DownloadOutlined />}
                  onClick={() => downloadBackupCodes(newBackupCodes)}
                >
                  下载
                </Button>
              </Space>
            </>
          )}
        </Modal>
      </div>
    );
  }

  // MFA设置流程
  return (
    <div className={styles.container}>
      <Card className={styles.card}>
        <div className={styles.header}>
          <SafetyCertificateOutlined className={styles.headerIcon} />
          <Title level={3}>设置多因素认证</Title>
          <Paragraph type="secondary">
            使用Google Authenticator或其他TOTP应用增强账户安全性
          </Paragraph>
        </div>

        <Steps current={currentStep} className={styles.steps}>
          <Step title="开始" icon={<LockOutlined />} />
          <Step title="扫描二维码" icon={<QrcodeOutlined />} />
          <Step title="保存恢复码" icon={<KeyOutlined />} />
        </Steps>

        {currentStep === 0 && (
          <div className={styles.stepContent}>
            <Alert
              message="什么是多因素认证？"
              description="多因素认证在您登录时需要额外输入一个动态验证码，即使密码泄露，攻击者也无法登录您的账户。"
              type="info"
              showIcon
              className={styles.alert}
            />
            <div className={styles.instructions}>
              <Title level={5}>准备工作</Title>
              <ol>
                <li>在手机上安装 Google Authenticator 或其他 TOTP 应用</li>
                <li>确保手机时间准确（建议开启自动同步）</li>
              </ol>
            </div>
            <Button
              type="primary"
              size="large"
              onClick={handleStartSetup}
              loading={loading}
              block
            >
              开始设置
            </Button>
          </div>
        )}

        {currentStep === 1 && setupData && (
          <div className={styles.stepContent}>
            <div className={styles.qrSection}>
              <div className={styles.qrCode}>
                <img src={setupData.qr_code} alt="MFA QR Code" />
              </div>
              <div className={styles.qrInstructions}>
                <Title level={5}>扫描二维码</Title>
                <Paragraph>
                  打开 Authenticator 应用，点击添加账户，然后扫描左侧二维码。
                </Paragraph>
                <Divider>或手动输入密钥</Divider>
                <div className={styles.secretKey}>
                  <code>{setupData.secret}</code>
                  <Button
                    icon={<CopyOutlined />}
                    size="small"
                    onClick={() => copyToClipboard(setupData.secret)}
                  >
                    复制
                  </Button>
                </div>
              </div>
            </div>

            <Divider />

            <div className={styles.verifySection}>
              <Title level={5}>输入验证码</Title>
              <Paragraph type="secondary">
                输入 Authenticator 应用中显示的 6 位验证码
              </Paragraph>
              <div className={styles.codeInputs}>
                {[0, 1, 2, 3, 4, 5].map((index) => (
                  <Input
                    key={index}
                    ref={(el) => { inputRefs.current[index] = el; }}
                    className={styles.codeInput}
                    maxLength={1}
                    value={verifyCode[index] || ''}
                    onChange={(e) => handleCodeInput(e.target.value.replace(/\D/g, ''), index)}
                    onKeyDown={(e) => {
                      if (e.key === 'Backspace' && !verifyCode[index] && index > 0) {
                        inputRefs.current[index - 1]?.focus();
                      }
                    }}
                  />
                ))}
              </div>
              <Button
                type="primary"
                size="large"
                onClick={handleVerify}
                loading={verifying}
                disabled={verifyCode.length !== 6}
                block
              >
                验证并启用
              </Button>
            </div>
          </div>
        )}

        {currentStep === 2 && setupData && (
          <div className={styles.stepContent}>
            <Alert
              message="MFA已成功启用！"
              description={isBackupCodesEnabled ? "请保存以下备用恢复码，当您无法访问Authenticator应用时可以使用。" : "您的账户现在受到多因素认证保护。"}
              type="success"
              showIcon
              icon={<CheckCircleOutlined />}
              className={styles.alert}
            />

            {isBackupCodesEnabled && (
              <div className={styles.backupCodesSection}>
                <Title level={5}>备用恢复码</Title>
                <Paragraph type="secondary">
                  每个恢复码只能使用一次，请妥善保管。
                </Paragraph>
                <div className={styles.backupCodes}>
                  {setupData.backup_codes.map((code, index) => (
                    <div key={index} className={styles.backupCode}>
                      <code>{code}</code>
                    </div>
                  ))}
                </div>
                <Space className={styles.backupActions}>
                  <Button
                    icon={<CopyOutlined />}
                    onClick={() => copyToClipboard(setupData.backup_codes.join('\n'))}
                  >
                    复制全部
                  </Button>
                  <Button
                    icon={<DownloadOutlined />}
                    onClick={() => downloadBackupCodes(setupData.backup_codes)}
                  >
                    下载
                  </Button>
                </Space>
              </div>
            )}

            <Button
              type="primary"
              size="large"
              onClick={() => navigate('/settings')}
              block
            >
              完成
            </Button>
          </div>
        )}
      </Card>
    </div>
  );
};

export default MFASetup;
import React, { useState, useRef, useEffect } from 'react';
import { Card, Button, Input, message, Alert, Typography, Space, Divider, Spin } from 'antd';
import type { InputRef } from 'antd';
import { SafetyCertificateOutlined, KeyOutlined } from '@ant-design/icons';
import { useNavigate, useLocation } from 'react-router-dom';
import { useDispatch } from 'react-redux';
import { verifyMFALogin } from '../services/mfaService';
import { loginSuccess } from '../store/slices/authSlice';
import api from '../services/api';
import styles from './MFAVerify.module.css';

const { Title, Text, Paragraph } = Typography;

interface LocationState {
  mfa_token: string;
  username: string;
  mfa_setup_required?: boolean;
}

const MFAVerify: React.FC = () => {
  const navigate = useNavigate();
  const location = useLocation();
  const dispatch = useDispatch();
  const state = location.state as LocationState;
  
  const [verifyCode, setVerifyCode] = useState('');
  const [verifying, setVerifying] = useState(false);
  const [useBackupCode, setUseBackupCode] = useState(false);
  const [backupCode, setBackupCode] = useState('');
  const [errorMessage, setErrorMessage] = useState('');
  const [redirecting, setRedirecting] = useState(false);
  const inputRefs = useRef<(InputRef | null)[]>([]);

  useEffect(() => {
    // 如果没有mfa_token，重定向到登录页
    if (!state?.mfa_token) {
      navigate('/login');
    }
    // 如果需要设置MFA，重定向到MFA设置页面
    if (state?.mfa_setup_required) {
      navigate('/setup/mfa', { state: { mfa_token: state.mfa_token } });
    }
  }, [state, navigate]);

  const handleCodeInput = (value: string, index: number) => {
    const newCode = verifyCode.split('');
    newCode[index] = value;
    const newCodeStr = newCode.join('');
    setVerifyCode(newCodeStr);
    setErrorMessage(''); // 清除错误信息
    
    // 自动跳转到下一个输入框
    if (value && index < 5) {
      inputRefs.current[index + 1]?.focus();
    }
    
    // 输入完6位后自动提交
    if (newCodeStr.length === 6 && value) {
      // 使用setTimeout确保状态更新后再提交
      setTimeout(() => {
        handleVerifyWithCode(newCodeStr);
      }, 100);
    }
  };

  const handleVerifyWithCode = async (code: string) => {
    if (verifying) return;
    
    try {
      setVerifying(true);
      setErrorMessage('');
      const response: any = await verifyMFALogin(state.mfa_token, code);
      
      console.log('[MFA Debug] Response:', response);
      
      const token = response?.data?.token;
      const user = response?.data?.user;
      if (token && user) {
        localStorage.setItem('token', token);
        console.log('[MFA Debug] Token saved to localStorage:', token.substring(0, 20) + '...');
        
        setRedirecting(true);
        message.success('验证成功，正在跳转...');
        
        try {
          const meResponse: any = await api.get('/auth/me', {
            headers: {
              'Authorization': `Bearer ${token}`
            }
          });
          console.log('[MFA Debug] /auth/me response:', meResponse);
          
          const fullUser = meResponse.data || meResponse;
          dispatch(loginSuccess({ token, user: fullUser }));
          console.log('[MFA Debug] Token verified and saved via Redux');
          
          navigate('/');
        } catch (meError) {
          console.error('[MFA Debug] /auth/me failed:', meError);
          dispatch(loginSuccess({ token, user }));
          navigate('/');
        }
      } else {
        console.error('[MFA Debug] No token or user in response:', response);
        throw new Error('服务器未返回有效的token');
      }
    } catch (error: any) {
      setErrorMessage('认证失败');
      // 不清空验证码，让用户可以看到输入的内容
    } finally {
      setVerifying(false);
    }
  };

  const handleVerify = async () => {
    if (useBackupCode) {
      if (backupCode.length !== 8) {
        message.error('请输入8位恢复码');
        return;
      }
      handleVerifyWithCode(backupCode);
    } else {
      if (verifyCode.length !== 6) {
        message.error('请输入6位验证码');
        return;
      }
      handleVerifyWithCode(verifyCode);
    }
  };

  const handleKeyPress = (e: React.KeyboardEvent) => {
    if (e.key === 'Enter') {
      handleVerify();
    }
  };

  if (!state?.mfa_token) {
    return null;
  }

  // 正在跳转时显示加载状态
  if (redirecting) {
    return (
      <div className={styles.container}>
        <Card className={styles.card}>
          <div className={styles.redirecting}>
            <Spin size="large" />
            <Title level={4} style={{ marginTop: 16 }}>验证成功</Title>
            <Paragraph type="secondary">正在跳转到首页...</Paragraph>
          </div>
        </Card>
      </div>
    );
  }

  return (
    <div className={styles.container}>
      <Card className={styles.card}>
        <div className={styles.header}>
          <SafetyCertificateOutlined className={styles.headerIcon} />
          <Title level={3}>多因素认证</Title>
          <Paragraph type="secondary">
            {state.username ? `欢迎回来，${state.username}` : '请输入验证码完成登录'}
          </Paragraph>
        </div>

        {!useBackupCode ? (
          <div className={styles.verifySection}>
            <Text>请输入 Authenticator 应用中显示的 6 位验证码</Text>
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
                  autoFocus={index === 0}
                  disabled={verifying}
                />
              ))}
            </div>
            {errorMessage && (
              <div className={styles.errorMessage}>
                <Text type="danger">{errorMessage}</Text>
              </div>
            )}
            <Button
              type="primary"
              size="large"
              onClick={handleVerify}
              loading={verifying}
              disabled={verifyCode.length !== 6}
              block
            >
              验证
            </Button>
          </div>
        ) : (
          <div className={styles.verifySection}>
            <Alert
              message="使用备用恢复码"
              description="每个恢复码只能使用一次。使用后建议重新生成新的恢复码。"
              type="info"
              showIcon
              className={styles.alert}
            />
            <Input
              placeholder="请输入8位恢复码"
              value={backupCode}
              onChange={(e) => {
                setBackupCode(e.target.value.replace(/\D/g, '').slice(0, 8));
                setErrorMessage('');
              }}
              maxLength={8}
              size="large"
              className={styles.backupInput}
              onKeyPress={handleKeyPress}
              disabled={verifying}
            />
            {errorMessage && (
              <div className={styles.errorMessage}>
                <Text type="danger">{errorMessage}</Text>
              </div>
            )}
            <Button
              type="primary"
              size="large"
              onClick={handleVerify}
              loading={verifying}
              disabled={backupCode.length !== 8}
              block
            >
              验证
            </Button>
          </div>
        )}

        <Divider />

        <div className={styles.footer}>
          {!useBackupCode ? (
            <Button
              type="link"
              icon={<KeyOutlined />}
              onClick={() => setUseBackupCode(true)}
            >
              无法访问 Authenticator？使用备用恢复码
            </Button>
          ) : (
            <Button
              type="link"
              onClick={() => {
                setUseBackupCode(false);
                setBackupCode('');
              }}
            >
              返回使用验证码
            </Button>
          )}
        </div>

        <div className={styles.backLink}>
          <Button type="link" onClick={() => navigate('/login')}>
            返回登录
          </Button>
        </div>
      </Card>
    </div>
  );
};

export default MFAVerify;
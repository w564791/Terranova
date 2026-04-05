import React, { useState, useRef, useEffect } from 'react';
import { useNavigate, useLocation } from 'react-router-dom';
import { useDispatch } from 'react-redux';
import { verifyMFALogin } from '../services/mfaService';
import { loginSuccess } from '../store/slices/authSlice';
import api from '../services/api';
import styles from './MFAVerify.module.css';

interface LocationState {
  mfa_token: string;
  username: string;
  mfa_setup_required?: boolean;
  required_backup_codes?: number;
}

const MFAVerify: React.FC = () => {
  const navigate = useNavigate();
  const location = useLocation();
  const dispatch = useDispatch();
  const state = location.state as LocationState;

  const [verifyCode, setVerifyCode] = useState('');
  const [verifying, setVerifying] = useState(false);
  const [useBackupCode, setUseBackupCode] = useState(false);
  const [backupCodes, setBackupCodes] = useState<string[]>(['']);
  const [requiredBackupCodes, setRequiredBackupCodes] = useState(() => {
    return state?.required_backup_codes !== undefined ? state.required_backup_codes : 1;
  });
  const [errorMessage, setErrorMessage] = useState('');
  const [redirecting, setRedirecting] = useState(false);
  const inputRefs = useRef<(HTMLInputElement | null)[]>([]);

  useEffect(() => {
    if (!state?.mfa_token) {
      navigate('/login');
      return;
    }
    if (state?.mfa_setup_required) {
      navigate('/setup/mfa', { state: { mfa_token: state.mfa_token } });
      return;
    }
    if (state?.required_backup_codes !== undefined) {
      setRequiredBackupCodes(state.required_backup_codes);
      if (state.required_backup_codes > 0) {
        setBackupCodes(Array(state.required_backup_codes).fill(''));
      }
    }
  }, [state, navigate]);

  const handleCodeInput = (value: string, index: number) => {
    const digit = value.replace(/\D/g, '');
    const newCode = verifyCode.split('');
    newCode[index] = digit;
    const newCodeStr = newCode.join('');
    setVerifyCode(newCodeStr);
    setErrorMessage('');

    if (digit && index < 5) {
      inputRefs.current[index + 1]?.focus();
    }

    if (newCodeStr.length === 6 && digit) {
      setTimeout(() => handleVerifyWithCode(newCodeStr), 100);
    }
  };

  const handleKeyDown = (e: React.KeyboardEvent, index: number) => {
    if (e.key === 'Backspace') {
      e.preventDefault();
      if (verifyCode[index]) {
        const newCode = verifyCode.split('');
        newCode[index] = '';
        setVerifyCode(newCode.join(''));
        setErrorMessage('');
      } else if (index > 0) {
        const newCode = verifyCode.split('');
        newCode[index - 1] = '';
        setVerifyCode(newCode.join(''));
        setErrorMessage('');
        inputRefs.current[index - 1]?.focus();
      }
    }
  };

  const handleVerifyWithCode = async (code: string) => {
    if (verifying) return;
    try {
      setVerifying(true);
      setErrorMessage('');
      const response: any = await verifyMFALogin(state.mfa_token, code);
      const token = response?.data?.token;
      const user = response?.data?.user;
      if (token && user) {
        localStorage.setItem('token', token);
        setRedirecting(true);
        try {
          const meResponse: any = await api.get('/auth/me', {
            headers: { 'Authorization': `Bearer ${token}` }
          });
          const fullUser = meResponse.data || meResponse;
          dispatch(loginSuccess({ token, user: fullUser }));
          navigate('/');
        } catch {
          dispatch(loginSuccess({ token, user }));
          navigate('/');
        }
      } else {
        throw new Error('Invalid response');
      }
    } catch {
      setErrorMessage('验证码错误，请重试');
    } finally {
      setVerifying(false);
    }
  };

  const handleBackupCodeChange = (index: number, value: string) => {
    const newCodes = [...backupCodes];
    newCodes[index] = value.replace(/\D/g, '').slice(0, 8);
    setBackupCodes(newCodes);
    setErrorMessage('');
  };

  const isBackupCodesComplete = () => backupCodes.every(code => code.length === 8);

  const handleVerify = () => {
    if (useBackupCode) {
      if (!isBackupCodesComplete()) return;
      handleVerifyWithCode(backupCodes.join(','));
    } else {
      if (verifyCode.length !== 6) return;
      handleVerifyWithCode(verifyCode);
    }
  };

  if (!state?.mfa_token) return null;

  if (redirecting) {
    return (
      <div className={styles.container}>
        <div className={styles.brandSection}>
          <h1 className={styles.brandTitle}>
            MFA
            <br />
            <span className={styles.brandHighlight}>Verified</span>
          </h1>
        </div>
        <div className={styles.formSection}>
          <div className={styles.formHeader}>
            <h2 className={styles.formTitle}>Redirecting...</h2>
          </div>
        </div>
      </div>
    );
  }

  return (
    <div className={styles.container}>
      <div className={styles.brandSection}>
        <h1 className={styles.brandTitle}>
          Multi-Factor
          <br />
          <span className={styles.brandHighlight}>Authentication</span>
          <br />
          {state.username ? `${state.username}` : ''}
        </h1>
        <p className={styles.brandSubtitle}>
          {useBackupCode
            ? 'Enter your backup recovery codes to complete login.'
            : 'Enter the 6-digit code from your authenticator app to verify your identity.'}
        </p>
      </div>

      <div className={styles.formSection}>
        <div className={styles.formHeader}>
          <h2 className={styles.formTitle}>
            {useBackupCode ? 'Recovery Code' : 'Verification Code'}
          </h2>
        </div>

        {!useBackupCode ? (
          <div className={styles.form}>
            <p className={styles.hint}>Enter the 6-digit code from your Authenticator app</p>
            <div className={styles.codeInputs}>
              {[0, 1, 2, 3, 4, 5].map((index) => (
                <input
                  key={index}
                  ref={(el) => { inputRefs.current[index] = el; }}
                  className={`${styles.codeInput} ${errorMessage ? styles.codeInputError : ''}`}
                  type="text"
                  inputMode="numeric"
                  maxLength={1}
                  value={verifyCode[index] || ''}
                  onChange={(e) => handleCodeInput(e.target.value, index)}
                  onKeyDown={(e) => handleKeyDown(e, index)}
                  autoFocus={index === 0}
                  disabled={verifying}
                />
              ))}
            </div>

            {errorMessage && (
              <div className={styles.errorMessage}>{errorMessage}</div>
            )}

            <button
              className={styles.button}
              onClick={handleVerify}
              disabled={verifying || verifyCode.length !== 6}
            >
              {verifying ? 'Verifying...' : 'Verify'}
            </button>
          </div>
        ) : (
          <div className={styles.form}>
            <div className={styles.infoBox}>
              Enter {requiredBackupCodes} recovery code(s). Each code can only be used once.
            </div>
            {backupCodes.map((code, index) => (
              <div className={styles.inputGroup} key={index}>
                <label className={styles.label}>Recovery Code {index + 1}</label>
                <input
                  className={`${styles.input} ${errorMessage ? styles.inputError : ''}`}
                  type="text"
                  inputMode="numeric"
                  placeholder="Enter 8-digit code"
                  value={code}
                  onChange={(e) => handleBackupCodeChange(index, e.target.value)}
                  onKeyDown={(e) => e.key === 'Enter' && handleVerify()}
                  maxLength={8}
                  disabled={verifying}
                />
              </div>
            ))}

            {errorMessage && (
              <div className={styles.errorMessage}>{errorMessage}</div>
            )}

            <button
              className={styles.button}
              onClick={handleVerify}
              disabled={verifying || !isBackupCodesComplete()}
            >
              {verifying ? 'Verifying...' : 'Verify'}
            </button>
          </div>
        )}

        <div className={styles.divider}>
          <span className={styles.dividerText}>
            {useBackupCode ? 'or' : 'lost access?'}
          </span>
        </div>

        <div className={styles.footer}>
          {requiredBackupCodes > 0 && (
            !useBackupCode ? (
              <button className={styles.linkButton} onClick={() => setUseBackupCode(true)}>
                Use Recovery Code
              </button>
            ) : (
              <button
                className={styles.linkButton}
                onClick={() => {
                  setUseBackupCode(false);
                  setBackupCodes(Array(requiredBackupCodes).fill(''));
                  setErrorMessage('');
                }}
              >
                Use Authenticator Code
              </button>
            )
          )}
          <button className={styles.linkButton} onClick={() => navigate('/login')}>
            Back to Login
          </button>
        </div>
      </div>
    </div>
  );
};

export default MFAVerify;

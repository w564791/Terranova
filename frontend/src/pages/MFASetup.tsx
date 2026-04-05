import React, { useState, useEffect, useRef } from 'react';
import { useNavigate, useLocation } from 'react-router-dom';
import { useDispatch } from 'react-redux';
import { loginSuccess } from '../store/slices/authSlice';
import { getMFAStatus, setupMFA, verifyAndEnableMFA, setupMFAWithToken, verifyAndEnableMFAWithToken, disableMFA, regenerateBackupCodes, getMFAConfig } from '../services/mfaService';
import type { MFAStatus, MFASetupResponse, MFAConfig } from '../services/mfaService';
import styles from './MFASetup.module.css';

const MFASetup: React.FC = () => {
  const navigate = useNavigate();
  const location = useLocation();
  const dispatch = useDispatch();
  const mfaToken = (location.state as any)?.mfa_token as string | undefined;
  const [loading, setLoading] = useState(true);
  const [mfaStatus, setMfaStatus] = useState<MFAStatus | null>(null);
  const [mfaConfig, setMfaConfig] = useState<MFAConfig | null>(null);
  const [setupData, setSetupData] = useState<MFASetupResponse | null>(null);
  const [currentStep, setCurrentStep] = useState(0);
  const [verifyCode, setVerifyCode] = useState('');
  const [verifying, setVerifying] = useState(false);
  const [errorMessage, setErrorMessage] = useState('');
  const [showDisableModal, setShowDisableModal] = useState(false);
  const [disableCode, setDisableCode] = useState('');
  const [disablePassword, setDisablePassword] = useState('');
  const [disabling, setDisabling] = useState(false);
  const [disableError, setDisableError] = useState('');
  const [showRegenerateModal, setShowRegenerateModal] = useState(false);
  const [regenerateCode, setRegenerateCode] = useState('');
  const [regenerating, setRegenerating] = useState(false);
  const [regenerateError, setRegenerateError] = useState('');
  const [newBackupCodes, setNewBackupCodes] = useState<string[]>([]);
  const [toast, setToast] = useState('');
  const inputRefs = useRef<(HTMLInputElement | null)[]>([]);

  const showToast = (msg: string) => {
    setToast(msg);
    setTimeout(() => setToast(''), 2500);
  };

  useEffect(() => {
    if (mfaToken) {
      setLoading(false);
    } else {
      loadMFAStatus();
      loadMFAConfig();
    }
  }, []);

  const loadMFAStatus = async () => {
    try {
      setLoading(true);
      const response: any = await getMFAStatus();
      setMfaStatus(response.data);
    } catch {
      setErrorMessage('Failed to load MFA status');
    } finally {
      setLoading(false);
    }
  };

  const loadMFAConfig = async () => {
    try {
      const response: any = await getMFAConfig();
      setMfaConfig(response.data?.config);
    } catch {
      // use defaults
    }
  };

  const isBackupCodesEnabled = mfaConfig?.required_backup_codes !== 0;

  const handleStartSetup = async () => {
    try {
      setLoading(true);
      setErrorMessage('');
      const response: any = mfaToken
        ? await setupMFAWithToken(mfaToken)
        : await setupMFA();
      setSetupData(response.data);
      setCurrentStep(1);
    } catch (error: any) {
      setErrorMessage(error || 'Failed to initialize MFA setup');
    } finally {
      setLoading(false);
    }
  };

  const handleVerify = async () => {
    if (verifyCode.length !== 6) return;
    try {
      setVerifying(true);
      setErrorMessage('');
      if (mfaToken) {
        const response: any = await verifyAndEnableMFAWithToken(mfaToken, verifyCode);
        setCurrentStep(2);
        const { token, user } = response.data;
        localStorage.setItem('token', token);
        dispatch(loginSuccess({ user, token }));
      } else {
        await verifyAndEnableMFA(verifyCode);
        setCurrentStep(2);
        loadMFAStatus();
      }
    } catch (error: any) {
      setErrorMessage(error || 'Verification failed, please check the code');
    } finally {
      setVerifying(false);
    }
  };

  const handleDisableMFA = async () => {
    if (disableCode.length !== 6 || !disablePassword) {
      setDisableError('Please enter both code and password');
      return;
    }
    try {
      setDisabling(true);
      setDisableError('');
      await disableMFA(disableCode, disablePassword);
      showToast('MFA disabled');
      setShowDisableModal(false);
      setDisableCode('');
      setDisablePassword('');
      loadMFAStatus();
      setSetupData(null);
      setCurrentStep(0);
    } catch (error: any) {
      setDisableError(error || 'Failed to disable MFA');
    } finally {
      setDisabling(false);
    }
  };

  const handleRegenerateBackupCodes = async () => {
    if (regenerateCode.length !== 6) {
      setRegenerateError('Please enter 6-digit code');
      return;
    }
    try {
      setRegenerating(true);
      setRegenerateError('');
      const response: any = await regenerateBackupCodes(regenerateCode);
      setNewBackupCodes(response.data.backup_codes);
      showToast('Backup codes regenerated');
      loadMFAStatus();
    } catch (error: any) {
      setRegenerateError(error || 'Failed to regenerate backup codes');
    } finally {
      setRegenerating(false);
    }
  };

  const copyToClipboard = (text: string) => {
    navigator.clipboard.writeText(text);
    showToast('Copied to clipboard');
  };

  const downloadBackupCodes = (codes: string[]) => {
    const content = `IaC Platform Backup Recovery Codes\nGenerated: ${new Date().toLocaleString()}\n\nEach code can only be used once.\n\n${codes.join('\n')}`;
    const blob = new Blob([content], { type: 'text/plain' });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = 'iac-platform-backup-codes.txt';
    a.click();
    URL.revokeObjectURL(url);
  };

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
      setTimeout(() => handleVerify(), 100);
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

  // --- Brand section content varies by state ---
  const getBrandContent = () => {
    if (mfaStatus?.mfa_enabled && currentStep === 0) {
      return {
        title: <>MFA<br /><span className={styles.brandHighlight}>Enabled</span></>,
        subtitle: 'Your account is protected by multi-factor authentication.',
      };
    }
    switch (currentStep) {
      case 0:
        return {
          title: <>Setup<br /><span className={styles.brandHighlight}>MFA</span></>,
          subtitle: 'Add an extra layer of security to your account using an authenticator app.',
        };
      case 1:
        return {
          title: <>Scan<br /><span className={styles.brandHighlight}>QR Code</span></>,
          subtitle: 'Open your authenticator app, add a new account, and scan the QR code.',
        };
      case 2:
        return {
          title: <>Setup<br /><span className={styles.brandHighlight}>Complete</span></>,
          subtitle: 'MFA is now enabled. Save your backup codes in a safe place.',
        };
      default:
        return { title: <>MFA</>, subtitle: '' };
    }
  };

  const brand = getBrandContent();

  // --- Loading ---
  if (loading && !setupData) {
    return (
      <div className={styles.container}>
        <div className={styles.brandSection}>
          <h1 className={styles.brandTitle}>{brand.title}</h1>
          <p className={styles.brandSubtitle}>{brand.subtitle}</p>
        </div>
        <div className={styles.formSection}>
          <div className={styles.formHeader}>
            <h2 className={styles.formTitle}>Loading...</h2>
          </div>
        </div>
      </div>
    );
  }

  // --- MFA already enabled ---
  if (mfaStatus?.mfa_enabled && currentStep === 0) {
    return (
      <div className={styles.container}>
        <div className={styles.brandSection}>
          <h1 className={styles.brandTitle}>{brand.title}</h1>
          <p className={styles.brandSubtitle}>{brand.subtitle}</p>
        </div>
        <div className={styles.formSection}>
          <div className={styles.formHeader}>
            <h2 className={styles.formTitle}>MFA Status</h2>
          </div>

          <div className={styles.successBox}>
            Your account is protected by multi-factor authentication.
            {mfaStatus.mfa_verified_at && (
              <span className={styles.statusMeta}>
                Enabled: {new Date(mfaStatus.mfa_verified_at).toLocaleString()}
              </span>
            )}
          </div>

          <div className={styles.statusGrid}>
            {isBackupCodesEnabled && (
              <div className={styles.statusItem}>
                <span className={styles.statusLabel}>Backup Codes</span>
                <span className={styles.statusValue}>{mfaStatus.backup_codes_count} remaining</span>
              </div>
            )}
            <div className={styles.statusItem}>
              <span className={styles.statusLabel}>Policy</span>
              <span className={styles.statusValue}>
                {mfaStatus.enforcement_policy === 'optional' && 'Optional'}
                {mfaStatus.enforcement_policy === 'required_new' && 'Required (new users)'}
                {mfaStatus.enforcement_policy === 'required_all' && 'Required (all users)'}
              </span>
            </div>
          </div>

          <div className={styles.actionButtons}>
            {isBackupCodesEnabled && (
              <button className={styles.buttonOutline} onClick={() => setShowRegenerateModal(true)}>
                Regenerate Backup Codes
              </button>
            )}
            {mfaStatus.enforcement_policy !== 'required_all' && (
              <button className={styles.buttonDanger} onClick={() => setShowDisableModal(true)}>
                Disable MFA
              </button>
            )}
          </div>
        </div>

        {toast && <div className={styles.toast}>{toast}</div>}

        {/* Disable MFA Modal */}
        {showDisableModal && (
          <div className={styles.modalOverlay} onClick={() => { setShowDisableModal(false); setDisableError(''); }}>
            <div className={styles.modal} onClick={e => e.stopPropagation()}>
              <h3 className={styles.modalTitle}>Disable MFA</h3>
              <div className={styles.warningBox}>
                Disabling MFA will reduce the security of your account.
              </div>
              <div className={styles.inputGroup}>
                <label className={styles.label}>Verification Code</label>
                <input
                  className={styles.input}
                  type="text"
                  inputMode="numeric"
                  placeholder="6-digit code from Authenticator"
                  value={disableCode}
                  onChange={e => { setDisableCode(e.target.value.replace(/\D/g, '').slice(0, 6)); setDisableError(''); }}
                  maxLength={6}
                />
              </div>
              <div className={styles.inputGroup}>
                <label className={styles.label}>Password</label>
                <input
                  className={styles.input}
                  type="password"
                  placeholder="Enter your login password"
                  value={disablePassword}
                  onChange={e => { setDisablePassword(e.target.value); setDisableError(''); }}
                />
              </div>
              {disableError && <div className={styles.errorMessage}>{disableError}</div>}
              <div className={styles.modalActions}>
                <button className={styles.buttonSecondary} onClick={() => { setShowDisableModal(false); setDisableCode(''); setDisablePassword(''); setDisableError(''); }}>
                  Cancel
                </button>
                <button className={styles.buttonDanger} onClick={handleDisableMFA} disabled={disabling}>
                  {disabling ? 'Disabling...' : 'Confirm Disable'}
                </button>
              </div>
            </div>
          </div>
        )}

        {/* Regenerate Backup Codes Modal */}
        {showRegenerateModal && (
          <div className={styles.modalOverlay} onClick={() => { setShowRegenerateModal(false); setRegenerateError(''); setNewBackupCodes([]); }}>
            <div className={styles.modal} onClick={e => e.stopPropagation()}>
              <h3 className={styles.modalTitle}>
                {newBackupCodes.length === 0 ? 'Regenerate Backup Codes' : 'New Backup Codes'}
              </h3>
              {newBackupCodes.length === 0 ? (
                <>
                  <div className={styles.warningBox}>
                    Old backup codes will be invalidated after regeneration.
                  </div>
                  <div className={styles.inputGroup}>
                    <label className={styles.label}>Verification Code</label>
                    <input
                      className={styles.input}
                      type="text"
                      inputMode="numeric"
                      placeholder="6-digit code from Authenticator"
                      value={regenerateCode}
                      onChange={e => { setRegenerateCode(e.target.value.replace(/\D/g, '').slice(0, 6)); setRegenerateError(''); }}
                      maxLength={6}
                    />
                  </div>
                  {regenerateError && <div className={styles.errorMessage}>{regenerateError}</div>}
                  <div className={styles.modalActions}>
                    <button className={styles.buttonSecondary} onClick={() => { setShowRegenerateModal(false); setRegenerateCode(''); setRegenerateError(''); }}>
                      Cancel
                    </button>
                    <button className={styles.button} onClick={handleRegenerateBackupCodes} disabled={regenerating}>
                      {regenerating ? 'Generating...' : 'Generate'}
                    </button>
                  </div>
                </>
              ) : (
                <>
                  <div className={styles.successBox}>
                    Save these codes. Each code can only be used once.
                  </div>
                  <div className={styles.backupCodes}>
                    {newBackupCodes.map((code, index) => (
                      <div key={index} className={styles.backupCode}>
                        <code>{code}</code>
                      </div>
                    ))}
                  </div>
                  <div className={styles.modalActions}>
                    <button className={styles.buttonOutline} onClick={() => copyToClipboard(newBackupCodes.join('\n'))}>
                      Copy All
                    </button>
                    <button className={styles.buttonOutline} onClick={() => downloadBackupCodes(newBackupCodes)}>
                      Download
                    </button>
                    <button className={styles.button} onClick={() => { setShowRegenerateModal(false); setNewBackupCodes([]); setRegenerateCode(''); }}>
                      Done
                    </button>
                  </div>
                </>
              )}
            </div>
          </div>
        )}
      </div>
    );
  }

  // --- MFA Setup Flow ---
  return (
    <div className={styles.container}>
      <div className={styles.brandSection}>
        <h1 className={styles.brandTitle}>{brand.title}</h1>
        <p className={styles.brandSubtitle}>{brand.subtitle}</p>
      </div>

      <div className={styles.formSection}>
        {/* Step indicator */}
        <div className={styles.steps}>
          {['Prepare', 'Scan & Verify', 'Backup Codes'].map((label, i) => (
            <div key={i} className={`${styles.step} ${i <= currentStep ? styles.stepActive : ''} ${i < currentStep ? styles.stepDone : ''}`}>
              <div className={styles.stepDot}>{i < currentStep ? '\u2713' : i + 1}</div>
              <span className={styles.stepLabel}>{label}</span>
            </div>
          ))}
        </div>

        {/* Step 0: Prepare */}
        {currentStep === 0 && (
          <>
            <div className={styles.formHeader}>
              <h2 className={styles.formTitle}>Setup MFA</h2>
            </div>
            <div className={styles.infoBox}>
              MFA adds a dynamic verification code to your login. Even if your password is compromised, your account stays protected.
            </div>
            <div className={styles.instructions}>
              <p className={styles.label}>Before you begin:</p>
              <ol>
                <li>Install Google Authenticator or another TOTP app on your phone</li>
                <li>Make sure your phone's clock is accurate (enable auto-sync)</li>
              </ol>
            </div>
            {errorMessage && <div className={styles.errorMessage}>{errorMessage}</div>}
            <button className={styles.button} onClick={handleStartSetup} disabled={loading}>
              {loading ? 'Setting up...' : 'Start Setup'}
            </button>
          </>
        )}

        {/* Step 1: Scan QR + Verify */}
        {currentStep === 1 && setupData && (
          <>
            <div className={styles.formHeader}>
              <h2 className={styles.formTitle}>Scan & Verify</h2>
            </div>

            <div className={styles.qrSection}>
              <div className={styles.qrCode}>
                <img src={setupData.qr_code} alt="MFA QR Code" />
              </div>
              <div className={styles.qrMeta}>
                <p className={styles.hint}>Scan this QR code with your authenticator app.</p>
                <div className={styles.divider}><span className={styles.dividerText}>or enter manually</span></div>
                <div className={styles.secretKey}>
                  <code>{setupData.secret}</code>
                  <button className={styles.copyBtn} onClick={() => copyToClipboard(setupData.secret)}>Copy</button>
                </div>
              </div>
            </div>

            <div className={styles.dividerFull} />

            <p className={styles.hint} style={{ marginBottom: 8 }}>Enter the 6-digit code from your authenticator app</p>
            <div className={styles.codeInputs}>
              {[0, 1, 2, 3, 4, 5].map((index) => (
                <input
                  key={index}
                  ref={el => { inputRefs.current[index] = el; }}
                  className={`${styles.codeInput} ${errorMessage ? styles.codeInputError : ''}`}
                  type="text"
                  inputMode="numeric"
                  maxLength={1}
                  value={verifyCode[index] || ''}
                  onChange={e => handleCodeInput(e.target.value, index)}
                  onKeyDown={e => handleKeyDown(e, index)}
                  autoFocus={index === 0}
                  disabled={verifying}
                />
              ))}
            </div>

            {errorMessage && <div className={styles.errorMessage}>{errorMessage}</div>}

            <button className={styles.button} onClick={handleVerify} disabled={verifying || verifyCode.length !== 6}>
              {verifying ? 'Verifying...' : 'Verify & Enable'}
            </button>
          </>
        )}

        {/* Step 2: Backup Codes */}
        {currentStep === 2 && setupData && (
          <>
            <div className={styles.formHeader}>
              <h2 className={styles.formTitle}>Save Backup Codes</h2>
            </div>

            <div className={styles.successBox}>
              MFA is now enabled.
              {isBackupCodesEnabled && ' Save these backup codes -- each can only be used once.'}
            </div>

            {isBackupCodesEnabled && (
              <>
                <div className={styles.backupCodes}>
                  {setupData.backup_codes.map((code, index) => (
                    <div key={index} className={styles.backupCode}>
                      <code>{code}</code>
                    </div>
                  ))}
                </div>
                <div className={styles.backupActions}>
                  <button className={styles.buttonOutline} onClick={() => copyToClipboard(setupData.backup_codes.join('\n'))}>
                    Copy All
                  </button>
                  <button className={styles.buttonOutline} onClick={() => downloadBackupCodes(setupData.backup_codes)}>
                    Download
                  </button>
                </div>
              </>
            )}

            <button className={styles.button} onClick={() => navigate('/settings')} style={{ marginTop: 16 }}>
              Done
            </button>
          </>
        )}
      </div>

      {toast && <div className={styles.toast}>{toast}</div>}
    </div>
  );
};

export default MFASetup;

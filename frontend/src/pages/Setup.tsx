import React, { useState, useEffect } from 'react';
import { useNavigate } from 'react-router-dom';
import { setupService } from '../services/auth';
import styles from './Setup.module.css';

const Setup: React.FC = () => {
  const navigate = useNavigate();
  const [loading, setLoading] = useState(true);
  const [submitting, setSubmitting] = useState(false);
  const [success, setSuccess] = useState(false);
  const [createdUser, setCreatedUser] = useState<{ username: string; email: string } | null>(null);

  const [formData, setFormData] = useState({
    username: '',
    email: '',
    password: '',
    confirmPassword: '',
  });

  const [errors, setErrors] = useState<Record<string, string>>({});

  // 检查系统是否已初始化
  useEffect(() => {
    const checkStatus = async () => {
      try {
        const response: any = await setupService.getStatus();
        const statusData = response.data || response;
        if (statusData.initialized) {
          navigate('/login', { replace: true });
          return;
        }
      } catch (error) {
        console.error('Failed to check setup status:', error);
      } finally {
        setLoading(false);
      }
    };
    checkStatus();
  }, [navigate]);

  const validate = (): boolean => {
    const newErrors: Record<string, string> = {};

    if (!formData.username.trim()) {
      newErrors.username = '请输入用户名';
    } else if (formData.username.length < 3) {
      newErrors.username = '用户名至少 3 个字符';
    } else if (formData.username.length > 50) {
      newErrors.username = '用户名不能超过 50 个字符';
    } else if (!/^[a-zA-Z0-9_-]+$/.test(formData.username)) {
      newErrors.username = '用户名只能包含字母、数字、下划线和连字符';
    }

    if (!formData.email.trim()) {
      newErrors.email = '请输入邮箱';
    } else if (!/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(formData.email)) {
      newErrors.email = '请输入有效的邮箱地址';
    }

    if (!formData.password) {
      newErrors.password = '请输入密码';
    } else if (formData.password.length < 8) {
      newErrors.password = '密码至少 8 个字符';
    }

    if (!formData.confirmPassword) {
      newErrors.confirmPassword = '请确认密码';
    } else if (formData.password !== formData.confirmPassword) {
      newErrors.confirmPassword = '两次输入的密码不一致';
    }

    setErrors(newErrors);
    return Object.keys(newErrors).length === 0;
  };

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();

    if (!validate()) return;

    setSubmitting(true);
    setErrors({});

    try {
      const response = await setupService.initAdmin({
        username: formData.username,
        email: formData.email,
        password: formData.password,
      });

      setCreatedUser({
        username: response.data?.username || formData.username,
        email: response.data?.email || formData.email,
      });
      setSuccess(true);
    } catch (error: any) {
      const message = error.message || '初始化失败，请重试';
      if (message.includes('已初始化')) {
        navigate('/login', { replace: true });
      } else if (message.includes('用户名') || message.includes('邮箱')) {
        setErrors({ username: message });
      } else {
        setErrors({ submit: message });
      }
    } finally {
      setSubmitting(false);
    }
  };

  const handleInputChange = (field: string, value: string) => {
    setFormData(prev => ({ ...prev, [field]: value }));
    if (errors[field]) {
      setErrors(prev => {
        const next = { ...prev };
        delete next[field];
        return next;
      });
    }
  };

  const handleGoToLogin = () => {
    navigate('/login', { replace: true });
  };

  // 加载状态
  if (loading) {
    return (
      <div className={styles.loadingContainer}>
        <div className={styles.loadingBrand}>
          <h1 className={styles.brandTitle}>
            强大。安全。卓越。
            <br />
            <span className={styles.brandHighlight}>IaC Platform</span>
          </h1>
        </div>
        <div className={styles.loadingContent}>
          <div className={styles.spinner} />
          <p className={styles.loadingText}>正在检查系统状态...</p>
        </div>
      </div>
    );
  }

  // 成功状态
  if (success) {
    return (
      <div className={styles.container}>
        <div className={styles.brandSection}>
          <h1 className={styles.brandTitle}>
            强大。安全。卓越。
            <br />
            <span className={styles.brandHighlight}>IaC Platform</span>
            <br />
            基础设施即代码管理平台。
          </h1>
          <p className={styles.brandSubtitle}>
            统一管理您的云基础设施，实现自动化部署、版本控制和团队协作。
            让基础设施管理变得简单、可靠、高效。
          </p>
        </div>

        <div className={styles.formSection}>
          <div className={styles.successCard}>
            <div className={styles.successIcon}>✓</div>
            <h2 className={styles.successTitle}>系统初始化完成</h2>
            <p className={styles.successMessage}>
              管理员账号已创建成功，请使用以下信息登录系统
            </p>

            {createdUser && (
              <div className={styles.adminInfo}>
                <div className={styles.adminInfoItem}>
                  <span className={styles.adminInfoLabel}>用户名</span>
                  <span className={styles.adminInfoValue}>{createdUser.username}</span>
                </div>
                <div className={styles.adminInfoItem}>
                  <span className={styles.adminInfoLabel}>邮箱</span>
                  <span className={styles.adminInfoValue}>{createdUser.email}</span>
                </div>
                <div className={styles.adminInfoItem}>
                  <span className={styles.adminInfoLabel}>角色</span>
                  <span className={styles.adminInfoValue}>系统管理员</span>
                </div>
              </div>
            )}

            <button className={styles.button} onClick={handleGoToLogin}>
              前往登录
            </button>
          </div>
        </div>
      </div>
    );
  }

  // 表单状态
  return (
    <div className={styles.container}>
      {/* 左侧品牌区域 */}
      <div className={styles.brandSection}>
        <h1 className={styles.brandTitle}>
          强大。安全。卓越。
          <br />
          <span className={styles.brandHighlight}>IaC Platform</span>
          <br />
          基础设施即代码管理平台。
        </h1>
        <p className={styles.brandSubtitle}>
          统一管理您的云基础设施，实现自动化部署、版本控制和团队协作。
          让基础设施管理变得简单、可靠、高效。
        </p>
      </div>

      {/* 右侧表单区域 */}
      <div className={styles.formSection}>
        <div className={styles.formHeader}>
          <h2 className={styles.formTitle}>创建系统管理员账号以完成初始化</h2>
        </div>

        {/* 步骤指示器 */}
        <div className={styles.steps}>
          <div className={`${styles.step} ${styles.stepActive}`}>
            <div className={`${styles.stepNumber} ${styles.stepNumberActive}`}>1</div>
            <span>创建管理员</span>
          </div>
          <div className={styles.stepDivider} />
          <div className={styles.step}>
            <div className={styles.stepNumber}>2</div>
            <span>开始使用</span>
          </div>
        </div>

        <form className={styles.form} onSubmit={handleSubmit}>
          {errors.submit && (
            <div className={styles.errorMessage}>
              {errors.submit}
            </div>
          )}

          <div className={styles.inputGroup}>
            <label className={styles.label}>用户名</label>
            <input
              type="text"
              placeholder="请输入管理员用户名"
              className={`${styles.input} ${errors.username ? styles.inputError : ''}`}
              value={formData.username}
              onChange={(e) => handleInputChange('username', e.target.value)}
              autoFocus
            />
            {errors.username && <span className={styles.errorText}>{errors.username}</span>}
            <span className={styles.hint}>3-50 个字符，支持字母、数字、下划线和连字符</span>
          </div>

          <div className={styles.inputGroup}>
            <label className={styles.label}>邮箱</label>
            <input
              type="email"
              placeholder="请输入管理员邮箱"
              className={`${styles.input} ${errors.email ? styles.inputError : ''}`}
              value={formData.email}
              onChange={(e) => handleInputChange('email', e.target.value)}
            />
            {errors.email && <span className={styles.errorText}>{errors.email}</span>}
          </div>

          <div className={styles.inputGroup}>
            <label className={styles.label}>密码</label>
            <input
              type="password"
              placeholder="请输入密码（至少 8 个字符）"
              className={`${styles.input} ${errors.password ? styles.inputError : ''}`}
              value={formData.password}
              onChange={(e) => handleInputChange('password', e.target.value)}
            />
            {errors.password && <span className={styles.errorText}>{errors.password}</span>}
          </div>

          <div className={styles.inputGroup}>
            <label className={styles.label}>确认密码</label>
            <input
              type="password"
              placeholder="请再次输入密码"
              className={`${styles.input} ${errors.confirmPassword ? styles.inputError : ''}`}
              value={formData.confirmPassword}
              onChange={(e) => handleInputChange('confirmPassword', e.target.value)}
            />
            {errors.confirmPassword && <span className={styles.errorText}>{errors.confirmPassword}</span>}
          </div>

          <button
            type="submit"
            className={styles.button}
            disabled={submitting}
          >
            {submitting ? '正在初始化...' : '创建管理员并完成初始化'}
          </button>
        </form>
      </div>
    </div>
  );
};

export default Setup;
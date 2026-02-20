import React, { useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { useSelector } from 'react-redux';
import type { RootState } from '../store';
import { authService } from '../services/auth';
import styles from './ResetPassword.module.css';

const ResetPassword: React.FC = () => {
  const navigate = useNavigate();
  const { user } = useSelector((state: RootState) => state.auth);
  const [loading, setLoading] = useState(false);
  const [success, setSuccess] = useState(false);
  const [formData, setFormData] = useState({
    currentPassword: '',
    newPassword: '',
    confirmPassword: ''
  });
  const [errors, setErrors] = useState<{
    currentPassword?: string;
    newPassword?: string;
    confirmPassword?: string;
  }>({});

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    
    // 验证表单
    const newErrors: typeof errors = {};
    if (!formData.currentPassword) newErrors.currentPassword = '请输入当前密码';
    if (!formData.newPassword) newErrors.newPassword = '请输入新密码';
    if (formData.newPassword.length < 6) newErrors.newPassword = '新密码至少6位';
    if (!formData.confirmPassword) newErrors.confirmPassword = '请确认新密码';
    if (formData.newPassword !== formData.confirmPassword) {
      newErrors.confirmPassword = '两次输入的密码不一致';
    }
    
    if (Object.keys(newErrors).length > 0) {
      setErrors(newErrors);
      return;
    }
    
    setErrors({});
    setLoading(true);
    
    try {
      await authService.resetPassword({
        current_password: formData.currentPassword,
        new_password: formData.newPassword
      });
      setSuccess(true);
      // 3秒后返回
      setTimeout(() => navigate('/'), 3000);
    } catch (error: any) {
      setErrors({ 
        currentPassword: error.message || '密码重置失败，请重试' 
      });
    } finally {
      setLoading(false);
    }
  };

  const handleInputChange = (field: string, value: string) => {
    setFormData(prev => ({ ...prev, [field]: value }));
    // 清除错误但保留用户输入
    if (errors[field as keyof typeof errors]) {
      setErrors(prev => ({ ...prev, [field]: undefined }));
    }
  };

  if (success) {
    return (
      <div className={styles.container}>
        <div className={styles.card}>
          <div className={styles.header}>
            <h1 className={styles.title}>密码重置成功</h1>
            <p className={styles.subtitle}>3秒后自动返回首页</p>
          </div>
        </div>
      </div>
    );
  }

  return (
    <div className={styles.container}>
      <div className={styles.card}>
        <div className={styles.header}>
          <h1 className={styles.title}>重置密码</h1>
          <p className={styles.subtitle}>为 {user?.username} 设置新密码</p>
        </div>
        
        <form className={styles.form} onSubmit={handleSubmit}>
          <div className={styles.inputGroup}>
            <label className={styles.label}>当前密码</label>
            <input
              type="password"
              className={styles.input}
              value={formData.currentPassword}
              onChange={(e) => handleInputChange('currentPassword', e.target.value)}
              style={{ borderColor: errors.currentPassword ? '#ef4444' : undefined }}
            />
            {errors.currentPassword && (
              <div className={styles.error}>{errors.currentPassword}</div>
            )}
          </div>
          
          <div className={styles.inputGroup}>
            <label className={styles.label}>新密码</label>
            <input
              type="password"
              className={styles.input}
              value={formData.newPassword}
              onChange={(e) => handleInputChange('newPassword', e.target.value)}
              style={{ borderColor: errors.newPassword ? '#ef4444' : undefined }}
            />
            {errors.newPassword && (
              <div className={styles.error}>{errors.newPassword}</div>
            )}
          </div>
          
          <div className={styles.inputGroup}>
            <label className={styles.label}>确认新密码</label>
            <input
              type="password"
              className={styles.input}
              value={formData.confirmPassword}
              onChange={(e) => handleInputChange('confirmPassword', e.target.value)}
              style={{ borderColor: errors.confirmPassword ? '#ef4444' : undefined }}
            />
            {errors.confirmPassword && (
              <div className={styles.error}>{errors.confirmPassword}</div>
            )}
          </div>
          
          <button
            type="submit"
            className={styles.button}
            disabled={loading}
          >
            {loading ? '重置中...' : '重置密码'}
          </button>
          
          <button
            type="button"
            className={styles.cancelButton}
            onClick={() => navigate('/')}
          >
            取消
          </button>
        </form>
      </div>
    </div>
  );
};

export default ResetPassword;
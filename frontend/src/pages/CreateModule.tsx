import React, { useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { moduleService } from '../services/modules';
import { useToast } from '../contexts/ToastContext';
import styles from './CreateModule.module.css';

const CreateModule: React.FC = () => {
  const navigate = useNavigate();
  const [loading, setLoading] = useState(false);
  const { success, error } = useToast();
  const [formData, setFormData] = useState({
    name: '',
    description: '',
    provider: 'AWS',
    module_source: '',
    repository_url: '',
    branch: 'main'
  });

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setLoading(true);

    try {
      await moduleService.createModule(formData);
      success('模块创建成功！');
      navigate('/modules');
    } catch (err: any) {
      error('创建模块失败: ' + (err.message || '未知错误'));
    } finally {
      setLoading(false);
    }
  };

  const handleChange = (e: React.ChangeEvent<HTMLInputElement | HTMLTextAreaElement | HTMLSelectElement>) => {
    setFormData({
      ...formData,
      [e.target.name]: e.target.value
    });
  };

  return (
    <div className={styles.container}>
      <div className={styles.header}>
        <button onClick={() => navigate('/modules')} className={styles.backButton}>
          ← 返回
        </button>
        <h1 className={styles.title}>创建模块</h1>
      </div>

      <div className={styles.formContainer}>
        <form onSubmit={handleSubmit} className={styles.form}>
          <div className={styles.formGroup}>
            <label className={styles.label}>模块名称 *</label>
            <input
              type="text"
              name="name"
              value={formData.name}
              onChange={handleChange}
              className={styles.input}
              placeholder="输入模块名称"
              required
            />
          </div>

          <div className={styles.formGroup}>
            <label className={styles.label}>提供商 *</label>
            <select
              name="provider"
              value={formData.provider}
              onChange={handleChange}
              className={styles.select}
              required
            >
              <option value="AWS">AWS</option>
              <option value="Azure">Azure</option>
              <option value="GCP">Google Cloud</option>
              <option value="Alibaba">阿里云</option>
            </select>
          </div>

          <div className={styles.formGroup}>
            <label className={styles.label}>Module Source *</label>
            <input
              type="text"
              name="module_source"
              value={formData.module_source}
              onChange={handleChange}
              className={styles.input}
              placeholder="例如: terraform-aws-modules/vpc/aws 或 git::https://..."
              required
            />
            <div className={styles.hint}>
              Terraform Module的source地址，用于在main.tf.json中引用此Module
            </div>
          </div>

          <div className={styles.formGroup}>
            <label className={styles.label}>仓库地址 *</label>
            <input
              type="url"
              name="repository_url"
              value={formData.repository_url}
              onChange={handleChange}
              className={styles.input}
              placeholder="https://github.com/user/repo"
              required
            />
          </div>

          <div className={styles.formGroup}>
            <label className={styles.label}>分支</label>
            <input
              type="text"
              name="branch"
              value={formData.branch}
              onChange={handleChange}
              className={styles.input}
              placeholder="main"
            />
          </div>

          <div className={styles.formGroup}>
            <label className={styles.label}>描述</label>
            <textarea
              name="description"
              value={formData.description}
              onChange={handleChange}
              className={styles.textarea}
              placeholder="输入模块描述"
              rows={4}
            />
          </div>

          <div className={styles.actions}>
            <button
              type="button"
              onClick={() => navigate('/modules')}
              className={styles.cancelButton}
            >
              取消
            </button>
            <button
              type="submit"
              disabled={loading}
              className={styles.submitButton}
            >
              {loading ? '创建中...' : '创建模块'}
            </button>
          </div>
        </form>
      </div>
      
      {/* 全局Toast由ToastProvider管理 */}
    </div>
  );
};

export default CreateModule;

import React, { useState, useEffect } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import { moduleService } from '../services/modules';
import styles from './EditModule.module.css';

const EditModule: React.FC = () => {
  const { id } = useParams<{ id: string }>();
  const navigate = useNavigate();
  const [loading, setLoading] = useState(false);
  const [initialLoading, setInitialLoading] = useState(true);
  const [formData, setFormData] = useState({
    module_source: '',
    description: '',
    version: '',
    branch: ''
  });

  const loadModule = async () => {
    if (!id) return;
    
    try {
      const response = await moduleService.getModule(parseInt(id));
      const module = response.data;
      setFormData({
        module_source: module.module_source || '',
        description: module.description || '',
        version: module.version || '',
        branch: module.branch || 'main'
      });
    } catch (error) {
      console.error('加载模块失败:', error);
    } finally {
      setInitialLoading(false);
    }
  };

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!id) return;
    
    setLoading(true);
    try {
      await moduleService.updateModule(parseInt(id), formData);
      navigate(`/modules/${id}`);
    } catch (error) {
      console.error('更新模块失败:', error);
    } finally {
      setLoading(false);
    }
  };

  const handleChange = (e: React.ChangeEvent<HTMLInputElement | HTMLTextAreaElement>) => {
    setFormData({
      ...formData,
      [e.target.name]: e.target.value
    });
  };

  useEffect(() => {
    loadModule();
  }, [id]);

  if (initialLoading) {
    return <div className={styles.loading}>加载中...</div>;
  }

  return (
    <div className={styles.container}>
      <div className={styles.header}>
        <button onClick={() => navigate(`/modules/${id}`)} className={styles.backButton}>
          ← 返回
        </button>
        <h1 className={styles.title}>编辑模块</h1>
      </div>

      <div className={styles.formContainer}>
        <form onSubmit={handleSubmit} className={styles.form}>
          <div className={styles.formGroup}>
            <label className={styles.label}>Module Source</label>
            <input
              type="text"
              name="module_source"
              value={formData.module_source}
              onChange={handleChange}
              className={styles.input}
              placeholder="例如: terraform-aws-modules/vpc/aws"
            />
            <div className={styles.hint}>
              Terraform Module的source地址，用于在main.tf.json中引用此Module
            </div>
          </div>

          <div className={styles.formGroup}>
            <label className={styles.label}>版本</label>
            <input
              type="text"
              name="version"
              value={formData.version}
              onChange={handleChange}
              className={styles.input}
              placeholder="输入版本号"
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
              onClick={() => navigate(`/modules/${id}`)}
              className={styles.cancelButton}
            >
              取消
            </button>
            <button
              type="submit"
              disabled={loading}
              className={styles.submitButton}
            >
              {loading ? '保存中...' : '保存更改'}
            </button>
          </div>
        </form>
      </div>
    </div>
  );
};

export default EditModule;

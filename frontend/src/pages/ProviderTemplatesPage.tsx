import React from 'react';
import ProviderTemplatesAdmin from './ProviderTemplatesAdmin';
import styles from './Admin.module.css';

const ProviderTemplatesPage: React.FC = () => {
  return (
    <div className={styles.container}>
      <div className={styles.header}>
        <h1 className={styles.title}>Provider Templates</h1>
        <p className={styles.description}>
          管理全局 Provider 配置模板，Workspace 可以引用这些模板并选择性地覆盖部分配置。
          支持任意 Terraform Provider 类型。
        </p>
      </div>
      <ProviderTemplatesAdmin />
    </div>
  );
};

export default ProviderTemplatesPage;

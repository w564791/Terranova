import React from 'react';
import { useNavigate, Link } from 'react-router-dom';
import styles from './IAMManagement.module.css';

const IAMManagement: React.FC = () => {
  const navigate = useNavigate();

  const modules = [
    {
      id: 'organizations',
      title: '组织管理',
      description: '管理平台中的组织，组织是权限管理的顶层单位',
      path: '/admin/organizations',
      icon: 'ORG',
      stats: '管理组织结构',
    },
    {
      id: 'projects',
      title: '项目管理',
      description: '管理组织下的项目，项目是工作空间的容器',
      path: '/admin/projects',
      icon: 'PRJ',
      stats: '组织项目资源',
    },
    {
      id: 'teams',
      title: '团队管理',
      description: '管理团队和成员，统一授予权限简化管理',
      path: '/admin/teams',
      icon: 'TEAM',
      stats: '协作团队管理',
    },
    {
      id: 'permissions',
      title: '权限管理',
      description: '管理用户、团队和应用的权限授予',
      path: '/iam/permissions',
      icon: 'PERM',
      stats: '细粒度权限控制',
    },
  ];

  return (
    <div className={styles.container}>
      {/* 页面头部 */}
      <div className={styles.header}>
        <h1 className={styles.title}>IAM 权限管理系统</h1>
        <p className={styles.description}>
          Identity and Access Management - 统一的身份认证和访问控制管理平台
        </p>
      </div>

      {/* 模块卡片网格 */}
      <div className={styles.modulesGrid}>
        {modules.map((module) => (
          <Link
            key={module.id}
            to={module.path}
            className={styles.moduleCard}
          >
            <div className={styles.cardIcon}>{module.icon}</div>
            <div className={styles.cardContent}>
              <h3 className={styles.cardTitle}>{module.title}</h3>
              <p className={styles.cardDescription}>{module.description}</p>
              <div className={styles.cardStats}>{module.stats}</div>
            </div>
            <div className={styles.cardArrow}>→</div>
          </Link>
        ))}
      </div>

      {/* 快速操作区域 */}
      <div className={styles.quickActions}>
        <h2 className={styles.sectionTitle}>快速操作</h2>
        <div className={styles.actionsGrid}>
          <button
            className={styles.actionButton}
            onClick={() => navigate('/admin/organizations')}
          >
            <span className={styles.actionIcon}>+</span>
            <span>创建组织</span>
          </button>
          <button
            className={styles.actionButton}
            onClick={() => navigate('/admin/projects')}
          >
            <span className={styles.actionIcon}>+</span>
            <span>创建项目</span>
          </button>
          <button
            className={styles.actionButton}
            onClick={() => navigate('/admin/teams')}
          >
            <span className={styles.actionIcon}>+</span>
            <span>创建团队</span>
          </button>
          <button
            className={styles.actionButton}
            onClick={() => navigate('/iam/permissions')}
          >
            <span className={styles.actionIcon}>+</span>
            <span>授予权限</span>
          </button>
        </div>
      </div>

      {/* 系统信息 */}
      <div className={styles.systemInfo}>
        <div className={styles.infoCard}>
          <div className={styles.infoLabel}>权限模型</div>
          <div className={styles.infoValue}>RBAC + ABAC</div>
        </div>
        <div className={styles.infoCard}>
          <div className={styles.infoLabel}>作用域层级</div>
          <div className={styles.infoValue}>组织 / 项目 / 工作空间</div>
        </div>
        <div className={styles.infoCard}>
          <div className={styles.infoLabel}>权限级别</div>
          <div className={styles.infoValue}>None / Read / Write / Admin</div>
        </div>
        <div className={styles.infoCard}>
          <div className={styles.infoLabel}>主体类型</div>
          <div className={styles.infoValue}>用户 / 团队 / 应用</div>
        </div>
      </div>
    </div>
  );
};

export default IAMManagement;

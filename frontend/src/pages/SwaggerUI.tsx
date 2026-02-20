import React, { useEffect } from 'react';
import { useSelector } from 'react-redux';
import { useNavigate } from 'react-router-dom';
import type { RootState } from '../store';
import styles from './SwaggerUI.module.css';

const SwaggerUI: React.FC = () => {
  const { user } = useSelector((state: RootState) => state.auth);
  const navigate = useNavigate();

  // 检测页面刷新，重定向到首页
  useEffect(() => {
    // 使用sessionStorage标记正常导航
    const navigationMark = sessionStorage.getItem('swagger-navigation');
    
    if (!navigationMark) {
      // 没有标记，说明是刷新或直接访问，重定向到首页
      navigate('/', { replace: true });
      return;
    }
    
    // 检查标记的时间戳（5秒内有效）
    const timestamp = parseInt(navigationMark);
    const now = Date.now();
    if (now - timestamp > 5000) {
      // 标记过期，说明是刷新，重定向到首页
      sessionStorage.removeItem('swagger-navigation');
      navigate('/', { replace: true });
      return;
    }
    
    // 延迟清除标记，确保组件完全加载
    setTimeout(() => {
      sessionStorage.removeItem('swagger-navigation');
    }, 1000);
  }, [navigate]);

  useEffect(() => {
    let link: HTMLLinkElement | null = null;
    let bundleScript: HTMLScriptElement | null = null;
    let standaloneScript: HTMLScriptElement | null = null;

    // 动态加载Swagger UI的CSS和JS
    const loadSwaggerUI = () => {
      // 检查是否已经加载过
      // @ts-ignore
      if (window.SwaggerUIBundle && window.SwaggerUIStandalonePreset) {
        initSwaggerUI();
        return;
      }

      // 加载Swagger UI CSS
      link = document.createElement('link');
      link.rel = 'stylesheet';
      link.href = 'https://cdn.jsdelivr.net/npm/swagger-ui-dist@5/swagger-ui.css';
      link.id = 'swagger-ui-css';
      document.head.appendChild(link);

      // 加载自定义修复CSS
      const fixLink = document.createElement('link');
      fixLink.rel = 'stylesheet';
      fixLink.href = '/swagger-fix.css';
      fixLink.id = 'swagger-fix-css';
      document.head.appendChild(fixLink);

      // 加载Swagger UI Bundle
      bundleScript = document.createElement('script');
      bundleScript.src = 'https://cdn.jsdelivr.net/npm/swagger-ui-dist@5/swagger-ui-bundle.js';
      bundleScript.id = 'swagger-ui-bundle';
      bundleScript.onload = () => {
        // 加载Swagger UI Standalone Preset
        standaloneScript = document.createElement('script');
        standaloneScript.src = 'https://cdn.jsdelivr.net/npm/swagger-ui-dist@5/swagger-ui-standalone-preset.js';
        standaloneScript.id = 'swagger-ui-standalone';
        standaloneScript.onload = () => {
          initSwaggerUI();
        };
        document.body.appendChild(standaloneScript);
      };
      document.body.appendChild(bundleScript);
    };

    const initSwaggerUI = () => {
      // 获取当前用户的token
      const token = localStorage.getItem('token');

      // @ts-ignore
      const ui = window.SwaggerUIBundle({
        url: 'http://localhost:8080/swagger/doc.json',
        dom_id: '#swagger-ui',
        deepLinking: true,
        filter: true, // 启用API搜索过滤功能
        presets: [
          // @ts-ignore
          window.SwaggerUIBundle.presets.apis,
          // @ts-ignore
          window.SwaggerUIStandalonePreset
        ],
        plugins: [
          // @ts-ignore
          window.SwaggerUIBundle.plugins.DownloadUrl
        ],
        layout: 'StandaloneLayout',
        // 自动添加Bearer token
        requestInterceptor: (request: any) => {
          if (token) {
            request.headers['Authorization'] = `Bearer ${token}`;
          }
          return request;
        },
      });

      // 预设认证信息
      if (token && ui) {
        setTimeout(() => {
          ui.preauthorizeApiKey('BearerAuth', `Bearer ${token}`);
        }, 100);
      }

      // @ts-ignore
      window.ui = ui;
    };

    loadSwaggerUI();

    // 清理函数
    return () => {
      // 清理Swagger UI实例
      // @ts-ignore
      if (window.ui) {
        // @ts-ignore
        window.ui = null;
      }

      // 清理DOM元素
      const swaggerContainer = document.getElementById('swagger-ui');
      if (swaggerContainer) {
        swaggerContainer.innerHTML = '';
      }
    };
  }, []);

  return (
    <div className={styles.container}>
      <div className={styles.header}>
        <h1>API 文档</h1>
        <div className={styles.userInfo}>
          <span className={styles.badge}>超级管理员</span>
          <span>{user?.username}</span>
        </div>
      </div>
      <div className={styles.info}>
        <p>
          <strong>说明：</strong>
          API文档已自动配置您的认证信息，可以直接测试所有API。
        </p>
      </div>
      <div id="swagger-ui" className={styles.swaggerContainer}></div>
    </div>
  );
};

export default SwaggerUI;

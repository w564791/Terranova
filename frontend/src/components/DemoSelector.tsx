import React, { useState, useEffect, useRef } from 'react';
import ReactDOM from 'react-dom';
import { moduleDemoService, type ModuleDemo } from '../services/moduleDemos';
import { useToast } from '../contexts/ToastContext';
import { extractErrorMessage } from '../utils/errorHandler';
import styles from './DemoSelector.module.css';

interface DemoSelectorProps {
  moduleId: number;
  onSelectDemo: (demoData: any, demoName: string) => void;
  hasFormData: boolean;
}

const DemoSelector: React.FC<DemoSelectorProps> = ({ moduleId, onSelectDemo, hasFormData }) => {
  const [demos, setDemos] = useState<ModuleDemo[]>([]);
  const [loading, setLoading] = useState(true);
  const [isOpen, setIsOpen] = useState(false);
  const [dropdownPosition, setDropdownPosition] = useState({ top: 0, left: 0, width: 0 });
  const { showToast } = useToast();
  const containerRef = useRef<HTMLDivElement>(null);
  const buttonRef = useRef<HTMLButtonElement>(null);

  useEffect(() => {
    loadDemos();
  }, [moduleId]);

  useEffect(() => {
    // 点击外部关闭下拉菜单
    const handleClickOutside = (event: MouseEvent) => {
      const target = event.target as Node;
      
      // 检查是否点击在容器内
      if (containerRef.current && containerRef.current.contains(target)) {
        return;
      }
      
      // 检查是否点击在 Portal 渲染的下拉菜单内
      // 通过检查 data-demo-selector-dropdown 属性
      const dropdownElement = document.querySelector('[data-demo-selector-dropdown]');
      if (dropdownElement && dropdownElement.contains(target)) {
        return;
      }
      
      setIsOpen(false);
    };

    if (isOpen) {
      // 使用 setTimeout 延迟添加事件监听，避免立即触发
      const timer = setTimeout(() => {
        document.addEventListener('mousedown', handleClickOutside);
      }, 0);
      
      return () => {
        clearTimeout(timer);
        document.removeEventListener('mousedown', handleClickOutside);
      };
    }
    
    return () => {
      document.removeEventListener('mousedown', handleClickOutside);
    };
  }, [isOpen]);

  // 计算下拉菜单位置
  useEffect(() => {
    if (isOpen && buttonRef.current) {
      const rect = buttonRef.current.getBoundingClientRect();
      setDropdownPosition({
        top: rect.bottom + 4,
        left: rect.left,
        width: Math.max(rect.width, 350),
      });
    }
  }, [isOpen]);

  const loadDemos = async () => {
    try {
      setLoading(true);
      const data = await moduleDemoService.getDemosByModuleId(moduleId);
      setDemos(data);
    } catch (error: any) {
      console.error('加载Demo列表失败:', error);
      showToast(extractErrorMessage(error), 'error');
    } finally {
      setLoading(false);
    }
  };

  const handleDemoClick = async (demo: ModuleDemo) => {
    setIsOpen(false);
    
    if (!demo.current_version?.config_data) {
      showToast('该Demo没有配置数据', 'warning');
      return;
    }

    onSelectDemo(demo.current_version.config_data, demo.name);
  };

  const formatDate = (dateString: string) => {
    const date = new Date(dateString);
    return date.toLocaleString('zh-CN', {
      year: 'numeric',
      month: '2-digit',
      day: '2-digit',
      hour: '2-digit',
      minute: '2-digit'
    });
  };

  const getButtonText = () => {
    if (loading) return '加载中...';
    if (demos.length === 0) return '该module暂无可用Demo';
    return `可用Demo ${demos.length}个`;
  };

  const isDisabled = loading || demos.length === 0;

  // 使用 Portal 渲染下拉菜单，避免被父容器的 overflow 截断
  const renderDropdown = () => {
    if (!isOpen || demos.length === 0) return null;

    return ReactDOM.createPortal(
      <div 
        className={styles.dropdown}
        data-demo-selector-dropdown="true"
        style={{
          position: 'fixed',
          top: dropdownPosition.top,
          left: dropdownPosition.left,
          minWidth: dropdownPosition.width,
        }}
      >
        {demos.map((demo) => (
          <div
            key={demo.id}
            className={styles.dropdownItem}
            onClick={() => handleDemoClick(demo)}
          >
            <div className={styles.demoName}>{demo.name}</div>
            <div className={styles.demoMeta}>
              {demo.description && (
                <span className={styles.demoDesc}>描述: {demo.description}</span>
              )}
              {demo.description && demo.updated_at && (
                <span className={styles.separator}> | </span>
              )}
              {demo.updated_at && (
                <span className={styles.demoTime}>更新: {formatDate(demo.updated_at)}</span>
              )}
            </div>
          </div>
        ))}
      </div>,
      document.body
    );
  };

  return (
    <div className={styles.container} ref={containerRef}>
      <button
        ref={buttonRef}
        className={styles.selectorButton}
        onClick={() => !isDisabled && setIsOpen(!isOpen)}
        disabled={isDisabled}
      >
        {getButtonText()}
        {!isDisabled && (
          <span className={styles.arrow}>{isOpen ? '▲' : '▼'}</span>
        )}
      </button>

      {renderDropdown()}
    </div>
  );
};

export default DemoSelector;

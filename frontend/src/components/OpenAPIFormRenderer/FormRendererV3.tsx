import React from 'react';
import { FormRenderer } from './index';
import type { FormRendererProps } from './types';
import { UIVersionProvider } from '../../contexts/UIVersionContext';
import styles from './FormRendererV3.module.css';

/**
 * FormRendererV3 — UI v3 包装组件
 *
 * 包裹原始 FormRenderer，通过 UIVersionProvider 上下文
 * 和 CSS 变量覆盖实现视觉升级，不修改 FormRenderer 核心逻辑。
 */
const FormRendererV3: React.FC<FormRendererProps> = (props) => {
  return (
    <UIVersionProvider value={{ version: 'v3', isV3: true }}>
      <div className={styles.v3Wrapper}>
        <FormRenderer {...props} />
      </div>
    </UIVersionProvider>
  );
};

export default FormRendererV3;

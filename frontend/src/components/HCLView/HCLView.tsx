import React, { useMemo, useState } from 'react';
import { jsonToHCL, highlightHCL } from '../../utils/hclFormatter';
import styles from './HCLView.module.css';

interface HCLViewProps {
  data: Record<string, unknown>;
  moduleSource?: string;
  moduleVersion?: string;
  moduleName?: string;
}

const HCLView: React.FC<HCLViewProps> = ({
  data,
  moduleSource,
  moduleVersion,
  moduleName,
}) => {
  const [copied, setCopied] = useState(false);

  const hclText = useMemo(
    () =>
      jsonToHCL(data, {
        moduleName: moduleName || 'resource',
        moduleSource,
        moduleVersion,
      }),
    [data, moduleSource, moduleVersion, moduleName]
  );

  const highlightedHTML = useMemo(() => highlightHCL(hclText), [hclText]);

  const handleCopy = async () => {
    try {
      await navigator.clipboard.writeText(hclText);
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    } catch {
      const textarea = document.createElement('textarea');
      textarea.value = hclText;
      document.body.appendChild(textarea);
      textarea.select();
      document.execCommand('copy');
      document.body.removeChild(textarea);
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    }
  };

  return (
    <div className={styles.container}>
      <div className={styles.toolbar}>
        <span className={styles.lang}>HCL · Terraform</span>
        <button
          className={`${styles.copyBtn} ${copied ? styles.copied : ''}`}
          onClick={handleCopy}
        >
          {copied ? '已复制 ✓' : '复制'}
        </button>
      </div>
      <div className={styles.body}>
        <pre dangerouslySetInnerHTML={{ __html: highlightedHTML }} />
      </div>
    </div>
  );
};

export default HCLView;

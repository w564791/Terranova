import React from 'react';
import styles from './SourceVersionCard.module.css';

interface SourceVersionCardProps {
  source?: string;
  version?: string;
}

const SourceVersionCard: React.FC<SourceVersionCardProps> = ({ source, version }) => {
  if (!source && !version) return null;

  return (
    <div className={styles.card}>
      {source && (
        <div className={styles.item}>
          <span className={styles.key}>Source</span>
          <span className={styles.val}>{source}</span>
        </div>
      )}
      {source && version && <div className={styles.divider} />}
      {version && (
        <div className={styles.item}>
          <span className={styles.key}>Version</span>
          <span className={styles.val}>{version}</span>
        </div>
      )}
    </div>
  );
};

export default SourceVersionCard;

import React, { useState, useCallback } from 'react';
import { StateUpload } from '../components/StateUpload';
import { StateVersionHistory } from '../components/StateVersionHistory';
import styles from './WorkspaceDetail.module.css';

interface StatesTabProps {
  workspaceId: string;
}

const StatesTab: React.FC<StatesTabProps> = ({ workspaceId }) => {
  // 用于触发 StateVersionHistory 刷新的 key
  const [refreshKey, setRefreshKey] = useState(0);

  // 上传成功后刷新列表
  const handleUploadSuccess = useCallback(() => {
    setRefreshKey(prev => prev + 1);
  }, []);

  return (
    <div className={styles.statesContainer}>
      {/* State Version History - 放在上面 */}
      <StateVersionHistory 
        key={refreshKey}
        workspaceId={workspaceId} 
      />

      {/* State Upload Section - 放在下面 */}
      <StateUpload 
        workspaceId={workspaceId} 
        onUploadSuccess={handleUploadSuccess}
      />
    </div>
  );
};

export default StatesTab;

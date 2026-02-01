import React from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import SchemaEditor from '../components/ModuleSchemaV2/SchemaEditor';
import styles from './SchemaManagement.module.css';

const SchemaEditorPage: React.FC = () => {
  const { moduleId, schemaId } = useParams<{ moduleId: string; schemaId: string }>();
  const navigate = useNavigate();

  const handleSave = () => {
    // 保存成功后返回 Schema 管理页面
    navigate(`/modules/${moduleId}/schemas`);
  };

  const handleCancel = () => {
    navigate(-1);
  };

  if (!moduleId) {
    return <div>模块 ID 无效</div>;
  }

  return (
    <div className={styles.container}>
      <div className={styles.header}>
        <div className={styles.headerLeft}>
          <button 
            onClick={() => navigate(-1)} 
            className={styles.backButton}
          >
            ← 返回
          </button>
          <h1 className={styles.title}>Schema 编辑器</h1>
        </div>
      </div>
      
      <div className={styles.contentFull}>
        <SchemaEditor
          moduleId={parseInt(moduleId, 10)}
          schemaId={schemaId ? parseInt(schemaId, 10) : undefined}
          onSave={handleSave}
          onCancel={handleCancel}
        />
      </div>
    </div>
  );
};

export default SchemaEditorPage;

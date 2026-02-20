import React, { useState } from 'react';
import DynamicForm, { type FormSchema } from '../components/DynamicForm';
import styles from './TestDynamicForm.module.css';

const testSchema: FormSchema = {
  name: {
    type: 'string',
    required: true,
    description: '资源名称'
  },
  region: {
    type: 'string',
    required: true,
    options: ['us-west-2', 'us-east-1', 'eu-west-1'],
    description: 'AWS区域'
  },
  instance_count: {
    type: 'number',
    required: false,
    default: 1,
    description: '实例数量'
  },
  enable_monitoring: {
    type: 'boolean',
    required: false,
    default: false,
    description: '启用监控'
  }
};

const TestDynamicForm: React.FC = () => {
  const [values, setValues] = useState<Record<string, any>>({});
  const [errors, setErrors] = useState<Record<string, string>>({});

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    
    // 简单验证
    const newErrors: Record<string, string> = {};
    Object.entries(testSchema).forEach(([key, schema]) => {
      if (schema.required && !values[key]) {
        newErrors[key] = '此字段为必填项';
      }
    });

    if (Object.keys(newErrors).length > 0) {
      setErrors(newErrors);
      return;
    }

    console.log('表单提交:', values);
    alert('表单提交成功！查看控制台输出。');
  };

  return (
    <div className={styles.container}>
      <div className={styles.card}>
        <h1 className={styles.title}>动态表单测试</h1>
        <p className={styles.subtitle}>基于Schema自动生成的表单</p>
        
        <form onSubmit={handleSubmit} className={styles.form}>
          <DynamicForm
            schema={testSchema}
            values={values}
            onChange={setValues}
            errors={errors}
          />
          
          <div className={styles.actions}>
            <button type="submit" className={styles.submitButton}>
              提交表单
            </button>
            <button 
              type="button" 
              onClick={() => setValues({})}
              className={styles.resetButton}
            >
              重置
            </button>
          </div>
        </form>

        <div className={styles.debug}>
          <h3>当前值:</h3>
          <pre>{JSON.stringify(values, null, 2)}</pre>
        </div>
      </div>
    </div>
  );
};

export default TestDynamicForm;
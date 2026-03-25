import React, { useState, useEffect, useCallback } from 'react';
import api from '../services/api';

interface PendingItem {
  id: string;
  capability: string;
  user_action: string;
  created_at: string;
}

const capabilityLabels: Record<string, string> = {
  form_generation: 'AI 配置生成',
  plan_summary: 'Plan 变更分析',
  apply_summary: 'Apply 执行分析',
  module_skill_generation: 'Module Skill 生成',
};

const stars = ['😞', '😕', '😐', '🙂', '😊'];

const FeedbackBanner: React.FC = () => {
  const [items, setItems] = useState<PendingItem[]>([]);

  const loadPending = useCallback(async () => {
    try {
      console.log('[FeedbackBanner] Loading pending feedback...');
      const res: any = await api.get('/ai/skill-usage/pending-feedback');
      console.log('[FeedbackBanner] Response:', res);
      setItems(res?.items || []);
    } catch (err) {
      console.warn('[FeedbackBanner] Error:', err);
    }
  }, []);

  useEffect(() => {
    loadPending();
    // 每 60 秒轮询一次
    const timer = setInterval(loadPending, 60000);
    return () => clearInterval(timer);
  }, [loadPending]);

  const submitFeedback = async (id: string, score: number) => {
    try {
      await api.put(`/ai/skill-usage/${id}/feedback`, { feedback: score });
    } catch {
      // silent
    }
    setItems(prev => prev.filter(item => item.id !== id));
  };

  const dismiss = (id: string) => {
    setItems(prev => prev.filter(item => item.id !== id));
  };

  if (items.length === 0) return null;

  return (
    <div style={{
      position: 'fixed',
      bottom: 16,
      right: 16,
      zIndex: 1050,
      display: 'flex',
      flexDirection: 'column',
      gap: 8,
      maxWidth: 360,
    }}>
      {items.map(item => (
        <div key={item.id} style={{
          background: '#fff',
          borderRadius: 8,
          boxShadow: '0 4px 16px rgba(0,0,0,0.12)',
          padding: '14px 16px',
          border: '1px solid #e8e8e8',
        }}>
          <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 8 }}>
            <span style={{ fontSize: 13, fontWeight: 600, color: '#262626' }}>
              {capabilityLabels[item.capability] || item.capability} 质量如何？
            </span>
            <span
              onClick={() => dismiss(item.id)}
              style={{ cursor: 'pointer', color: '#8c8c8c', fontSize: 16, lineHeight: 1 }}
              title="跳过"
            >×</span>
          </div>
          <div style={{ fontSize: 11, color: '#8c8c8c', marginBottom: 10 }}>
            {item.created_at} · {item.user_action === 'accepted' ? '已应用' : '已终止'}
          </div>
          <div style={{ display: 'flex', gap: 10, justifyContent: 'center' }}>
            {stars.map((emoji, i) => (
              <span
                key={i}
                onClick={() => submitFeedback(item.id, i + 1)}
                title={`${i + 1} 分`}
                style={{
                  cursor: 'pointer',
                  fontSize: 26,
                  transition: 'transform 0.15s',
                }}
                onMouseEnter={e => { (e.target as HTMLElement).style.transform = 'scale(1.3)'; }}
                onMouseLeave={e => { (e.target as HTMLElement).style.transform = 'scale(1)'; }}
              >{emoji}</span>
            ))}
          </div>
        </div>
      ))}
    </div>
  );
};

export default FeedbackBanner;

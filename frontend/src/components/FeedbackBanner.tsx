import React, { useState, useEffect, useCallback } from 'react';
import { useLocation } from 'react-router-dom';
import api from '../services/api';

interface PendingItem {
  id: string;
  capability: string;
  user_action: string;
  task_id: string | null;
  created_at: string;
}

const capabilityLabels: Record<string, string> = {
  form_generation: 'AI 配置生成',
  plan_summary: 'Plan 变更分析',
  apply_summary: 'Apply 执行分析',
  module_skill_generation: 'Module Skill 生成',
};

const stars = ['😞', '😕', '😐', '🙂', '😊'];

// 记住已跳过的 ID，避免轮询再弹出
const DISMISSED_KEY = 'skill_feedback_dismissed';
function getDismissed(): Set<string> {
  try {
    return new Set(JSON.parse(localStorage.getItem(DISMISSED_KEY) || '[]'));
  } catch { return new Set(); }
}
function addDismissed(id: string) {
  const set = getDismissed();
  set.add(id);
  // 只保留最近 50 个
  const arr = [...set].slice(-50);
  localStorage.setItem(DISMISSED_KEY, JSON.stringify(arr));
}

const FeedbackBanner: React.FC = () => {
  const [item, setItem] = useState<PendingItem | null>(null);
  const location = useLocation();

  // 从 URL 提取当前 task ID（如 /workspaces/xxx/tasks/123 或 /workspaces/xxx 页面）
  const currentTaskId = React.useMemo(() => {
    const match = location.pathname.match(/\/tasks\/(\d+)/);
    return match ? match[1] : null;
  }, [location.pathname]);

  const loadPending = useCallback(async () => {
    if (!localStorage.getItem('token')) return;
    try {
      const res: any = await api.get('/ai/skill-usage/pending-feedback');
      const all: PendingItem[] = res?.items || [];
      const dismissed = getDismissed();
      // 只在 task 详情页显示，且只显示当前 task 的评分
      if (!currentTaskId) {
        setItem(null);
        return;
      }
      const filtered = all.filter(i => {
        if (dismissed.has(i.id)) return false;
        return i.task_id === currentTaskId;
      });
      setItem(filtered[0] || null);
    } catch {
      // silent
    }
  }, [currentTaskId]);

  useEffect(() => {
    loadPending();
    const timer = setInterval(loadPending, 60000);
    // 监听 action 上报事件，立刻刷新
    const onRefresh = () => setTimeout(loadPending, 1500); // 等后端落库
    window.addEventListener('skill-action-reported', onRefresh);
    return () => {
      clearInterval(timer);
      window.removeEventListener('skill-action-reported', onRefresh);
    };
  }, [loadPending]);

  const submitFeedback = async (id: string, score: number) => {
    try {
      await api.put(`/ai/skill-usage/${id}/feedback`, { feedback: score });
    } catch {
      // silent
    }
    addDismissed(id);
    setItem(null);
    // 加载下一个
    setTimeout(loadPending, 500);
  };

  const dismiss = (id: string) => {
    addDismissed(id);
    setItem(null);
    setTimeout(loadPending, 500);
  };

  if (!item) return null;

  return (
    <div style={{
      position: 'fixed',
      bottom: 16,
      right: 16,
      zIndex: 1050,
      maxWidth: 340,
    }}>
      <div style={{
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
    </div>
  );
};

export default FeedbackBanner;

import { useState, useEffect } from 'react';
import styles from './MotivationalQuote.module.css';

interface MotivationalQuoteProps {
  username?: string;
}

const getGreetingByTime = (username: string) => {
  const now = new Date();
  const hour = now.getHours();
  
  // 早上（5:00 - 11:59）
  if (hour >= 5 && hour < 12) {
    return `${username}，早上好`;
  }
  
  // 中午（12:00 - 13:59）
  if (hour >= 12 && hour < 14) {
    return `${username}，中午好`;
  }
  
  // 下午（14:00 - 17:59）
  if (hour >= 14 && hour < 18) {
    return `${username}，下午好`;
  }
  
  // 晚上（18:00 - 22:59）
  if (hour >= 18 && hour < 23) {
    return `${username}，晚上好`;
  }
  
  // 深夜（23:00 - 4:59）
  return `${username}，夜深了`;
};

const getQuoteByTime = () => {
  const now = new Date();
  const hour = now.getHours();
  
  // console.log('[MotivationalQuote] 当前时间:', now.toLocaleString('zh-CN'), '小时:', hour);
  
  if (hour >= 5 && hour < 12) {
    return {
      text: '新的一天，迎接挑战',
      background: 'linear-gradient(135deg, #ff9a56 0%, #ff6a88 50%, #ffd3a5 100%)'
    };
  }

  if (hour >= 12 && hour < 18) {
    return {
      text: '保持节奏，稳步前行',
      background: 'linear-gradient(135deg, var(--brand-300) 0%, var(--brand) 100%)'
    };
  }

  if (hour >= 18 && hour < 23) {
    return {
      text: '辛苦了，今天也在进步',
      background: 'linear-gradient(135deg, var(--brand) 0%, var(--brand-ink) 100%)'
    };
  }

  return {
    text: '夜深了，注意休息',
    background: 'linear-gradient(135deg, var(--brand-700) 0%, var(--brand-ink) 50%, var(--brand-ink) 100%)'
  };
};

const MotivationalQuote: React.FC<MotivationalQuoteProps> = ({ username = '用户' }) => {
  const [quote, setQuote] = useState(getQuoteByTime());
  const [greeting, setGreeting] = useState(getGreetingByTime(username));

  useEffect(() => {
    // 每分钟检查一次时间，更新激励语和问候语
    const timer = setInterval(() => {
      setQuote(getQuoteByTime());
      setGreeting(getGreetingByTime(username));
    }, 60000); // 60秒

    return () => clearInterval(timer);
  }, [username]);

  return (
    <div className={styles.container} style={{ background: quote.background }}>
      <span className={styles.greeting}>{greeting}</span>
      <span className={styles.separator}>|</span>
      <span className={styles.text}>{quote.text}</span>
    </div>
  );
};

export default MotivationalQuote;

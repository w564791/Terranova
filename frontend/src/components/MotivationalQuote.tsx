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
  
  // 早安激励（5:00 - 11:59）- 日出渐变
  if (hour >= 5 && hour < 12) {
    return {
      icon: '🌅',
      text: '新的一天，新的起点！每一点努力，都是未来闪光的伏笔。去迎接今天的挑战吧！',
      background: 'linear-gradient(135deg, #ff9a56 0%, #ff6a88 50%, #ffd3a5 100%)' // 日出橙粉色
    };
  }
  
  // 午间激励（12:00 - 17:59）- 蓝天白云
  if (hour >= 12 && hour < 18) {
    return {
      icon: '🌞',
      text: '忙碌的上午辛苦了！稍作休息，补充能量，下午继续为梦想加速前行 💪',
      background: 'linear-gradient(135deg, #56ccf2 0%, #2f80ed 100%)' // 更深的蓝天渐变
    };
  }
  
  // 晚安激励（18:00 - 22:59）- 城市夜景
  if (hour >= 18 && hour < 23) {
    return {
      icon: '🌇',
      text: '今天也在认真生活，辛苦了！别忘了肯定自己——每一点进步，都是值得庆祝的事 ✨',
      background: 'linear-gradient(135deg, #667eea 0%, #764ba2 100%)' // 紫色夜景
    };
  }
  
  // 凌晨激励（23:00 - 4:59）- 星空
  return {
    icon: '🌙',
    text: '夜深人静，属于思考与积蓄力量的时刻。别急，所有坚持都会在黎明前发光 🌌',
    background: 'linear-gradient(135deg, #1e3c72 0%, #2a5298 50%, #7e22ce 100%)' // 深蓝紫星空
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
      <span className={styles.icon}>{quote.icon}</span>
      <span className={styles.greeting}>{greeting}</span>
      <span className={styles.separator}>|</span>
      <span className={styles.text}>{quote.text}</span>
    </div>
  );
};

export default MotivationalQuote;

import { useState, useEffect, useRef, useCallback } from 'react';

interface OutputMessage {
  type: 'output' | 'error' | 'completed' | 'connected' | 'stage_marker';
  line?: string;
  timestamp?: string;
  line_num?: number;
  stage?: string;
  status?: 'begin' | 'end';
}

export const useTerraformOutput = (taskId: number) => {
  const [lines, setLines] = useState<OutputMessage[]>([]);
  const [isConnected, setIsConnected] = useState(false);
  const [isCompleted, setIsCompleted] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const wsRef = useRef<WebSocket | null>(null);
  const reconnectTimeoutRef = useRef<number>();
  const reconnectAttemptsRef = useRef(0);
  const isConnectingRef = useRef(false); // 防止重复连接

  const connect = useCallback(() => {
    // 防止重复连接（React Strict Mode会导致useEffect执行2次）
    if (isConnectingRef.current || wsRef.current?.readyState === WebSocket.OPEN) {
      console.log('[useTerraformOutput] Already connecting or connected, skipping');
      return;
    }
    
    isConnectingRef.current = true;
    const token = localStorage.getItem('token');
    const wsUrl = `ws://localhost:8080/api/v1/tasks/${taskId}/output/stream`;
    const ws = new WebSocket(wsUrl, ['access_token', token || '']);
    wsRef.current = ws;

    ws.onopen = () => {
      console.log('[useTerraformOutput] Connected to terraform output stream');
      setIsConnected(true);
      setError(null);
      reconnectAttemptsRef.current = 0;
      isConnectingRef.current = false;
    };

    ws.onmessage = (event) => {
      try {
        const data: OutputMessage = JSON.parse(event.data);
        
        switch (data.type) {
          case 'connected':
            console.log('Stream connected, client_id:', (data as any).client_id);
            break;
            
          case 'output':
          case 'error':
          case 'stage_marker':
            setLines(prev => [...prev, data]);
            break;
            
          case 'completed':
            console.log('Task completed');
            setIsCompleted(true);
            break;
        }
      } catch (err) {
        console.error('Failed to parse message:', err);
      }
    };

    ws.onerror = (error) => {
      console.error('[useTerraformOutput] WebSocket error:', error);
      setError('连接错误');
      setIsConnected(false);
      isConnectingRef.current = false;
    };

    ws.onclose = () => {
      console.log('[useTerraformOutput] WebSocket closed');
      setIsConnected(false);
      isConnectingRef.current = false;
      
      // 如果任务未完成且重连次数未超限，自动重连
      if (!isCompleted && reconnectAttemptsRef.current < 10) {
        reconnectAttemptsRef.current++;
        const delay = Math.min(5000 * reconnectAttemptsRef.current, 30000);
        
        console.log(`Reconnecting in ${delay}ms (attempt ${reconnectAttemptsRef.current})...`);
        
        reconnectTimeoutRef.current = window.setTimeout(() => {
          connect();
        }, delay);
      }
    };
  }, [taskId, isCompleted]);

  useEffect(() => {
    connect();

    return () => {
      if (reconnectTimeoutRef.current) {
        clearTimeout(reconnectTimeoutRef.current);
      }
      if (wsRef.current?.readyState === WebSocket.OPEN) {
        wsRef.current.close();
      }
    };
  }, [connect]);

  const clearLogs = useCallback(() => {
    setLines([]);
  }, []);

  return { 
    lines, 
    isConnected, 
    isCompleted, 
    error,
    clearLogs
  };
};

/**
 * WebSocket服务
 * 用于实时通信，支持接管请求等功能
 */

type MessageHandler = (data: any) => void;

interface WebSocketMessage {
  type: string;
  session_id?: string;
  data: any;
}

class WebSocketService {
  private ws: WebSocket | null = null;
  private sessionId: string = '';
  private listeners: Map<string, MessageHandler[]> = new Map();
  private reconnectTimer: number | null = null;
  private reconnectAttempts = 0;
  private maxReconnectAttempts = 5;
  private reconnectDelay = 3000; // 3秒
  private isIntentionallyClosed = false;

  /**
   * 连接WebSocket
   */
  connect(sessionId: string): void {
    if (this.ws?.readyState === WebSocket.OPEN) {
      console.log('WebSocket already connected');
      return;
    }

    this.sessionId = sessionId;
    this.isIntentionallyClosed = false;

    const token = localStorage.getItem('token');
    if (!token) {
      console.error('No auth token found');
      return;
    }

    // 构建WebSocket URL
    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    const host = window.location.host;
    const wsUrl = `${protocol}//${host}/api/v1/ws/editing/${sessionId}`;

    console.log('Connecting to WebSocket:', wsUrl);

    try {
      // 使用 Sec-WebSocket-Protocol 传递token（比URL参数更安全）
      // 格式: "access_token, <token>"
      this.ws = new WebSocket(wsUrl, ['access_token', token]);

      this.ws.onopen = () => {
        console.log(' WebSocket connected');
        this.reconnectAttempts = 0;
        
        // 发送认证信息（如果需要）
        // this.send('auth', { token });
      };

      this.ws.onmessage = (event) => {
        try {
          const message: WebSocketMessage = JSON.parse(event.data);
          this.handleMessage(message);
        } catch (error) {
          console.error('Failed to parse WebSocket message:', error);
        }
      };

      this.ws.onclose = (event) => {
        console.log('❌ WebSocket disconnected', event.code, event.reason);
        this.ws = null;

        // 如果不是主动关闭，尝试重连
        if (!this.isIntentionallyClosed) {
          this.attemptReconnect();
        }
      };

      this.ws.onerror = (error) => {
        console.error('WebSocket error:', error);
      };
    } catch (error) {
      console.error('Failed to create WebSocket:', error);
      this.attemptReconnect();
    }
  }

  /**
   * 处理接收到的消息
   */
  private handleMessage(message: WebSocketMessage): void {
    console.log('📥 WebSocket message received:', message.type, message.data);

    const handlers = this.listeners.get(message.type);
    if (handlers && handlers.length > 0) {
      handlers.forEach(handler => {
        try {
          handler(message.data);
        } catch (error) {
          console.error(`Error in message handler for ${message.type}:`, error);
        }
      });
    }
  }

  /**
   * 尝试重连
   */
  private attemptReconnect(): void {
    if (this.reconnectAttempts >= this.maxReconnectAttempts) {
      console.error('Max reconnect attempts reached');
      return;
    }

    if (this.reconnectTimer) {
      return; // 已经在重连中
    }

    this.reconnectAttempts++;
    console.log(`🔄 Reconnecting... (${this.reconnectAttempts}/${this.maxReconnectAttempts})`);

    this.reconnectTimer = window.setTimeout(() => {
      this.reconnectTimer = null;
      this.connect(this.sessionId);
    }, this.reconnectDelay);
  }

  /**
   * 监听消息
   */
  on(event: string, handler: MessageHandler): void {
    if (!this.listeners.has(event)) {
      this.listeners.set(event, []);
    }
    this.listeners.get(event)!.push(handler);
  }

  /**
   * 取消监听
   */
  off(event: string, handler: MessageHandler): void {
    const handlers = this.listeners.get(event);
    if (handlers) {
      const index = handlers.indexOf(handler);
      if (index > -1) {
        handlers.splice(index, 1);
      }
    }
  }

  /**
   * 发送消息
   */
  send(type: string, data: any): void {
    if (!this.ws || this.ws.readyState !== WebSocket.OPEN) {
      console.warn('WebSocket not connected, cannot send message');
      return;
    }

    const message: WebSocketMessage = {
      type,
      data,
    };

    try {
      this.ws.send(JSON.stringify(message));
      console.log('📤 WebSocket message sent:', type);
    } catch (error) {
      console.error('Failed to send WebSocket message:', error);
    }
  }

  /**
   * 断开连接
   */
  disconnect(): void {
    this.isIntentionallyClosed = true;

    if (this.reconnectTimer) {
      clearTimeout(this.reconnectTimer);
      this.reconnectTimer = null;
    }

    if (this.ws) {
      this.ws.close();
      this.ws = null;
    }

    this.listeners.clear();
    console.log('WebSocket disconnected');
  }

  /**
   * 检查连接状态
   */
  isConnected(): boolean {
    return this.ws?.readyState === WebSocket.OPEN;
  }
}

// 导出单例
export const websocketService = new WebSocketService();

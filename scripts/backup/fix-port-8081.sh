#!/bin/bash

echo "=== 诊断端口 8081 占用情况 ==="
echo ""

# 检查端口占用
echo "1. 检查端口 8081 是否被占用..."
if lsof -nP -iTCP:8081 -sTCP:LISTEN >/dev/null 2>&1; then
    echo "   ✓ 端口 8081 正在被使用"
    echo ""
    echo "2. 查找占用进程..."
    lsof -nP -iTCP:8081 -sTCP:LISTEN
    echo ""
    
    # 获取PID
    PID=$(lsof -nP -iTCP:8081 -sTCP:LISTEN | grep LISTEN | awk '{print $2}' | head -1)
    
    if [ -n "$PID" ]; then
        echo "3. 发现进程 PID: $PID"
        echo ""
        echo "进程信息:"
        ps -p $PID -o pid,ppid,user,command
        echo ""
        
        read -p "是否要终止此进程? (y/n): " -n 1 -r
        echo ""
        if [[ $REPLY =~ ^[Yy]$ ]]; then
            echo "正在终止进程 $PID..."
            kill $PID
            sleep 2
            
            # 检查是否成功
            if lsof -nP -iTCP:8081 -sTCP:LISTEN >/dev/null 2>&1; then
                echo "进程仍在运行，尝试强制终止..."
                kill -9 $PID
                sleep 1
            fi
            
            if ! lsof -nP -iTCP:8081 -sTCP:LISTEN >/dev/null 2>&1; then
                echo "✓ 端口 8081 已释放"
            else
                echo "✗ 无法释放端口 8081"
            fi
        fi
    fi
else
    echo "   ✓ 端口 8081 未被占用"
fi

echo ""
echo "=== 诊断完成 ==="

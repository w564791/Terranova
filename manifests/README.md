# IAC Platform Kubernetes Deployment

> **快速体验？** 如果只是 POC / 演示 / 评估，推荐使用项目根目录的 [Docker Compose 快速部署](../docker-compose.example.yml)，无需 K8s 环境，三条命令即可启动。
>
> 本文档面向 **生产环境** 的 Kubernetes 部署，提供 TLS 加密、HA 高可用、网络策略、OPA 安全策略等完整能力。

## Prerequisites

- Kubernetes cluster (v1.27+)
- `kubectl` configured to access the cluster
- `helm` version >= v4.0.0
- `kind` version = latest

### create local kubernetes cluster

```
 kind create cluster
```

### Local Access

#### 生成本地信任证书（可选,macOS）

使用 [mkcert](https://github.com/FiloSottile/mkcert) 生成本地信任的 TLS 证书，浏览器访问时不会报不安全连接：

```bash
# 安装 mkcert
brew install mkcert
brew install nss   # Firefox 需要，Safari / Chrome 可跳过

# 将 mkcert CA 安装到系统信任链
mkcert -install

# 为平台域名生成证书
mkcert \
  www.iac-platform.com \
  iac-platform.com \
  api.iac-platform.com

# 生成的文件：
#   www.iac-platform.com+2.pem     (证书)
#   www.iac-platform.com+2-key.pem (私钥)
```

将生成的证书和私钥替换到 `tls/secret-gateway-tls.yaml` 中：

```bash
# base64 编码后替换 secret-gateway-tls.yaml 中的 tls.crt 和 tls.key
kubectl -n terraform create secret tls iac-gateway-tls \
  --cert=www.iac-platform.com+2.pem \
  --key=www.iac-platform.com+2-key.pem \
  --dry-run=client -o yaml > tls/secret-gateway-tls.yaml
```

#### 配置 hosts

在 `/etc/hosts` 中添加以下记录：

```
127.0.0.1 www.iac-platform.com iac-platform.com api.iac-platform.com

```

### Install cert-manager

cert-manager 用于自动签发和管理集群内部 TLS 证书。

```bash
# Install cert-manager
kubectl apply -f https://github.com/cert-manager/cert-manager/releases/download/v1.19.2/cert-manager.yaml

# Wait for cert-manager to be ready
kubectl -n cert-manager rollout status deployment/cert-manager
kubectl -n cert-manager rollout status deployment/cert-manager-webhook
kubectl -n cert-manager rollout status deployment/cert-manager-cainjector
```

Ref: https://cert-manager.io/docs/installation/

### Install Envoy Gateway

Envoy Gateway 作为 Kubernetes Gateway API 的实现，负责外部流量入口和 TLS 终止。

```bash
# Install Gateway API CRDs
kubectl apply -f https://github.com/kubernetes-sigs/gateway-api/releases/latest/download/standard-install.yaml

# Install Envoy Gateway
helm install eg oci://docker.io/envoyproxy/gateway-helm \
  --version 1.7.0 \
  -n envoy-gateway-system \
  --create-namespace \
  --skip-crds

# Wait for Envoy Gateway to be ready
kubectl -n envoy-gateway-system rollout status deployment/envoy-gateway
```

> **关于 WebSocket**：Envoy Gateway 通过 HTTPS (ALPN) 与浏览器协商 HTTP/2。HTTP/2 的 WebSocket 使用 Extended CONNECT (RFC 8441)，但 Go 的 gorilla/websocket 库不支持。因此 **所有用户流量（包括 /api/）统一走前端 nginx 代理**，由 nginx 以 HTTP/1.1 + 标准 WebSocket 升级头连接后端，避免协议不兼容。详见 httproute.yaml 和 nginx.conf。

Ref: https://gateway.envoyproxy.io/docs/install/

## Docker Images

| Image | Description |
|-------|-------------|
| [w564791/iac-platform](https://hub.docker.com/r/w564791/iac-platform) | Backend server (API + WebSocket) |
| [w564791/iac-frontend](https://hub.docker.com/r/w564791/iac-frontend) | Frontend (nginx) |
| [w564791/iac-agent](https://hub.docker.com/r/w564791/iac-agent) | Agent (Terraform/OpenTofu executor) |
| [w564791/iac-db-init](https://hub.docker.com/r/w564791/iac-db-init) | Database initialization job |

## Directory Structure

```
manifests/
├── kustomization.yaml              # Root kustomization (imports tls, base, gateway, db)
├── base/                           # Core resources
│   ├── kustomization.yaml
│   ├── namespace.yaml              # Namespace: terraform
│   ├── configmap.yaml              # Non-sensitive config (DB_HOST, DB_SSLMODE, ports, etc.)
│   ├── secret.yaml                 # Sensitive config (DB credentials, JWT secret)
│   ├── ha-rbac.yaml                # ServiceAccount, Role, RoleBinding
│   ├── deployment-backend.yaml     # Backend (2 replicas, HTTPS)
│   ├── deployment-frontend.yaml    # Frontend nginx (2 replicas, HTTPS)
│   ├── service-backend.yaml        # ClusterIP: 8080 (API) + 8090 (Agent CC)
│   └── service-frontend.yaml       # ClusterIP: 443
├── tls/                            # TLS certificates
│   ├── kustomization.yaml
│   ├── certificate.yaml            # cert-manager internal CA chain + service certs (incl. postgres)
│   ├── secret-gateway-tls.yaml     # Gateway external TLS certificate
│   └── certs/                      # mkcert certificate files (for local dev)
│       ├── localhost.pem
│       └── localhost-key.pem
├── db/                             # Database
│   ├── kustomization.yaml
│   ├── statefulset-postgres.yaml   # PostgreSQL StatefulSet + Service (conditional SSL)
│   ├── job-db-init.yaml            # K8s Job for DB initialization
│   ├── Dockerfile                  # DB init container image
│   ├── entrypoint.sh               # psql entrypoint script
│   └── init_seed_data.sql          # Schema + seed data
├── gateway/                        # Envoy Gateway API
│   ├── kustomization.yaml
│   ├── gateway.yaml                # GatewayClass + Gateway (HTTPS 443 + 8090)
│   ├── httproute.yaml              # Routes: frontend(/, /api/ via nginx), health(/health direct)
│   └── backend-tls-policy.yaml     # Gateway → backend/frontend HTTPS policy
└── gatekeeper/                     # OPA Gatekeeper 准入策略（可选，需单独部署）
    ├── kustomization.yaml
    ├── config-sync.yaml            # 同步 Pod 数据到 Gatekeeper 缓存
    ├── templates/
    │   ├── constraint-template-block-exec.yaml       # BlockExecByLabel
    │   ├── constraint-template-block-token.yaml      # BlockTokenRequest
    │   ├── constraint-template-restrict-sa.yaml      # RestrictServiceAccount
    │   ├── constraint-template-restrict-secret.yaml  # RestrictSecretAccess
    │   ├── constraint-template-block-ephemeral.yaml  # BlockEphemeralContainer
    │   ├── constraint-template-block-pod-connect.yaml # BlockPodConnect
    │   └── constraint-template-block-privileged.yaml # BlockPrivilegedPod
    └── constraints/
        ├── block-exec-agent-backend.yaml             # 禁止 exec 进 agent/backend Pod
        ├── block-token-terraform-ns.yaml             # 禁止 terraform NS 的 SA token 创建
        ├── restrict-sa-terraform-ns.yaml             # 高权限 SA 绑定 Pod 标签
        ├── restrict-secret-terraform-ns.yaml         # Secret 引用绑定 Pod 标签
        ├── block-ephemeral-terraform-ns.yaml         # 禁止注入 ephemeral container
        ├── block-pod-connect-terraform-ns.yaml       # 禁止 attach / port-forward
        └── block-privileged-terraform-ns.yaml        # 禁止特权容器 / hostPath
```

## Quick Deploy


```bash
cd manifests

# 基于当前时间+PID+主机名自动生成 JWT_SECRET（每次部署不同）
JWT_KEY=$(echo -n "$(date +%s)-$$-$(hostname)" | openssl dgst -sha256 -binary | base64 | tr -d '\n')

sed -i '' "s|JWT_SECRET=.*|JWT_SECRET=${JWT_KEY}|" base/kustomization.yaml

kubectl kustomize | kubectl create -f -

kubectl -n terraform wait --for=condition=complete job/iac-db-init --timeout=120s
```

> `JWT_SECRET` 每次部署时基于 `时间戳(秒) + PID + 主机名` 经 SHA-256 生成 256-bit 密钥，通过 `sed` 直接写入 `base/kustomization.yaml`。

## Configuration

Before deploying, update the following values to match your environment:

**`base/configmap.yaml`**
| Key | Description | Default |
|-----|-------------|---------|
| `DB_HOST` | PostgreSQL host | `postgres` |
| `DB_PORT` | PostgreSQL port | `5432` |
| `DB_NAME` | Database name | `iac_platform` |
| `DB_SSLMODE` | PostgreSQL SSL mode | `require` |
| `TZ` | Timezone | `Asia/Singapore` |

**`base/secret.yaml`**
| Key | Description | Default |
|-----|-------------|---------|
| `DB_USER` | Database user | `postgres` |
| `DB_PASSWORD` | Database password | `postgres123` |

**`base/kustomization.yaml` — secretGenerator**
| Key | Description | 生成方式 |
|-----|-------------|---------|
| `JWT_SECRET` | JWT signing key | 部署时基于 `时间戳 + 主机名` 经 SHA-256 自动生成 256-bit 密钥，无需手动配置 |

**`tls/secret-gateway-tls.yaml`**
- Replace with your own TLS certificate for external access (current: mkcert self-signed for `*.iac-platform.com`)


### 访问平台

通过 port-forward 将 Envoy Gateway 暴露到本地：

```bash
# 将 Gateway 映射到本地 8443 端口
kubectl port-forward -n envoy-gateway-system svc/envoy-terraform-iac-platform-ce676110 8443:443

# 访问平台
# https://www.iac-platform.com:8443
```

## Verify Deployment

```bash
# Check all resources
kubectl -n terraform get all,certificate,gateway,httproute

# Check pods are running
kubectl -n terraform get pods

# Check certificates are issued
kubectl -n terraform get certificate

# Test backend health endpoint (via port-forward)
kubectl port-forward -n envoy-gateway-system svc/envoy-terraform-iac-platform-ce676110 8443:443

curl -k https://localhost:8443/health
```

## Architecture

```
External Traffic                         kubectl (exec/attach/debug/...)
       │                                          │
       ▼                                          ▼
┌─────────────────┐ TLS Terminate        ┌────────────────┐
│     Gateway     │ 443 / 8090           │ kube-apiserver │
└────────┬────────┘                      └───────┬────────┘
         │ HTTPS (internal CA)              Admission Webhook
         │                               ┌───────▼────────┐
         │                               │   Gatekeeper   │
         │                               │  (OPA Engine)  │
         │                               └───────┬────────┘
         │                                       ╳ exec → backend/agent
         │                                       ╳ attach / port-forward
         │                                       ╳ create token → SA
         │                                       ╳ ephemeral container
         │                                       ╳ privileged / hostPath
         │                                       ╳ SA 与 Pod 标签不匹配
         │                                       ╳ Secret 与 Pod 标签不匹配
         │
         │
         ▼ HTTPS (internal CA)
    ┌──────────┐  proxy /api/  ┌─────────────┐
    │ Frontend │──────────────▶│   Backend   │◀── Agent Pods
    │ (nginx)  │  HTTP/1.1+WS  │  (Go API)   │    (CC WebSocket)
    │ :443 TLS │               │ :8080 :8090 │
    └──────────┘               └──────┬──────┘
                        │
                        ▼
                 ┌─────────────┐
                 │ PostgreSQL  │
                 └─────────────┘
```
## 安全增强

### OPA Gatekeeper 准入控制（可选）

#### 为什么需要

`terraform` 命名空间运行的 Pod 持有高敏感凭证：

- **Backend Pod** — 数据库密码、JWT 签名密钥、云平台凭证
- **Agent Pod** — Terraform state（含云账号 AK/SK）、Provider 缓存的临时凭证

如果攻击者获得了集群内有限的 RBAC 权限（例如 `pods/exec` 或 `serviceaccounts/token`），即可：

1. **`kubectl exec`** 进入 backend/agent Pod，直接读取环境变量或文件系统中的凭证
2. **`kubectl create token`** 为 `terraform` 命名空间的 ServiceAccount 签发 token，冒充其身份调用 Kubernetes API
3. **创建 Pod 挂载高权限 SA**（如 `iac-platform`），用任意标签绕过身份绑定，借此获取该 SA 的 RBAC 权限（可读写 Secrets、操作 Pod）

Gatekeeper 策略在准入层拦截这三类操作，作为 RBAC 之外的纵深防御。

#### 影响范围

| 策略 | 拦截操作 | 受保护资源 | 不受影响 |
|------|---------|-----------|---------|
| `BlockExecByLabel` | `kubectl exec` / `kubectl cp` | agent + backend Pod | frontend、postgres Pod |
| `BlockTokenRequest` | `kubectl create token` | terraform NS 所有 SA | 其他 NS 的 SA |
| `RestrictServiceAccount` | Pod 创建时校验 SA-标签绑定 | `iac-platform` SA 仅允许 backend + agent Pod | 使用 `default` SA 的 Pod |
| `RestrictSecretAccess` | Pod 创建时校验 Secret-标签绑定 | 所有已声明的 Secret（见下方绑定表） | 未列入保护的 Secret |
| `BlockEphemeralContainer` | `kubectl debug`（注入调试容器） | terraform NS 所有 Pod | 其他 NS 的 Pod |
| `BlockPodConnect` | `kubectl attach` / `kubectl port-forward` | terraform NS 所有 Pod | 其他 NS 的 Pod |
| `BlockPrivilegedPod` | 创建特权容器、hostPath、hostPID、hostNetwork | terraform NS 所有 Pod | 其他 NS 的 Pod |

**Secret-标签绑定关系：**

| Secret | 允许的 Pod 标签 |
|--------|----------------|
| `iac-platform`（DB 凭证） | backend, postgres, db-init, agent |
| `iac-jwt`（JWT 密钥） | backend, agent |
| `iac-platform-tls` | backend |
| `iac-frontend-tls` | frontend |
| `iac-postgres-tls` | postgres |
| `iac-internal-ca` | db-init |
| `iac-gateway-tls` | 无（仅 Gateway 资源引用，任何 Pod 不得挂载） |

> **注意**：`kubectl cp` 底层使用 exec，同样被 `BlockExecByLabel` 拦截。

#### 安装 Gatekeeper

Gatekeeper 默认不拦截 CONNECT 操作（exec 属于 CONNECT verb）。安装时必须启用 `enableConnectOperations`：

```bash
helm repo add gatekeeper https://open-policy-agent.github.io/gatekeeper/charts
helm repo update

helm install gatekeeper gatekeeper/gatekeeper \
  --namespace gatekeeper-system --create-namespace \
  --set enableConnectOperations=true

# 等待就绪
kubectl -n gatekeeper-system rollout status deployment/gatekeeper-audit
kubectl -n gatekeeper-system rollout status deployment/gatekeeper-controller-manager
```

#### 部署策略

Gatekeeper 策略独立于主应用，不包含在 `kustomization.yaml` 中，需单独部署：

```bash
# 部署所有策略
kubectl kustomize manifests/gatekeeper | kubectl apply -f -

# 或分步部署（推荐，确保 CRD 先注册）
# 1. 同步配置 + ConstraintTemplate
kubectl apply -f manifests/gatekeeper/config-sync.yaml
kubectl apply -f manifests/gatekeeper/templates/

# 2. 等待 Template CRD 就绪
for crd in blockexecbylabel blocktokenrequest restrictserviceaccount \
           restrictsecretaccess blockephemeralcontainer blockpodconnect \
           blockprivilegedpod; do
  kubectl wait --for=condition=established \
    crd/${crd}.constraints.gatekeeper.sh --timeout=60s
done

# 3. 部署 Constraint 实例
kubectl apply -f manifests/gatekeeper/constraints/
```

#### 验证

```bash
# exec 应被拒绝
kubectl exec -n terraform <backend-pod> -- /bin/sh   # denied
kubectl exec -n terraform <agent-pod> -- /bin/sh     # denied

# exec 应允许
kubectl exec -n terraform <frontend-pod> -- echo ok  # allowed
kubectl exec -n terraform <postgres-pod> -- echo ok  # allowed

# token 应被拒绝
kubectl create token iac-platform -n terraform       # denied

# token 应允许（其他命名空间）
kubectl create token default -n default              # allowed

# SA 绑定 — 用错误标签挂载 iac-platform SA 应被拒绝
kubectl run rogue --image=busybox -n terraform \
  --overrides='{"spec":{"serviceAccountName":"iac-platform"}}' -- sleep 3600  # denied

# Secret 绑定 — 用 default SA 挂载受保护 secret 应被拒绝
kubectl run steal --image=busybox -n terraform \
  --overrides='{"spec":{"volumes":[{"name":"s","secret":{"secretName":"iac-jwt"}}],
  "containers":[{"name":"steal","image":"busybox","volumeMounts":
  [{"name":"s","mountPath":"/s"}]}]}}' -- sleep 3600                          # denied

# ephemeral container — kubectl debug 应被拒绝
kubectl debug -n terraform <backend-pod> -it --image=busybox                  # denied

# attach / port-forward 应被拒绝
kubectl attach -n terraform <backend-pod>                                     # denied
kubectl port-forward -n terraform <postgres-pod> 5432:5432                    # denied

# 特权容器 应被拒绝
kubectl run priv --image=busybox -n terraform \
  --overrides='{"spec":{"containers":[{"name":"priv","image":"busybox",
  "securityContext":{"privileged":true}}]}}' -- sleep 3600                    # denied

# 正常部署不受影响（使用 default SA、无 secret 挂载）
kubectl run test --image=busybox -n terraform -- sleep 3600                   # allowed
```

#### 豁免配置

`BlockTokenRequest` 预留了 `exemptServiceAccounts` 参数。如需豁免特定 ServiceAccount（例如系统控制器），编辑 `constraints/block-token-terraform-ns.yaml`：

```yaml
parameters:
  exemptServiceAccounts:
    - system-controller-sa
```

`RestrictServiceAccount` 通过 `bindings` 配置 SA 与 Pod 标签的绑定关系。如需新增绑定，编辑 `constraints/restrict-sa-terraform-ns.yaml`：

```yaml
parameters:
  bindings:
    - serviceAccountName: iac-platform
      allowedPodLabels:
        - labels:
            app.kubernetes.io/component: backend
        - labels:
            component: agent
    - serviceAccountName: new-privileged-sa
      allowedPodLabels:
        - labels:
            app.kubernetes.io/component: new-component
```

`RestrictSecretAccess` 通过 `protectedSecrets` 配置 Secret 与 Pod 标签的绑定关系。如需新增受保护 Secret 或扩展允许访问的 Pod，编辑 `constraints/restrict-secret-terraform-ns.yaml`：

```yaml
parameters:
  protectedSecrets:
    - secretName: new-secret
      allowedPodLabels:
        - labels:
            app.kubernetes.io/component: new-component
```

## Uninstall

```bash

kubectl kustomize|kubectl delete -f -

```

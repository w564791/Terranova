# IAC Platform Kubernetes Deployment


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
└── gateway/                        # Envoy Gateway API
    ├── kustomization.yaml
    ├── gateway.yaml                # GatewayClass + Gateway (HTTPS 443 + 8090)
    ├── httproute.yaml              # Routes: frontend(/), API(/api/,/health)
    └── backend-tls-policy.yaml     # Gateway → backend/frontend HTTPS policy
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
External Traffic
       │
       ▼
┌─────────────┐  TLS Terminate (iac-gateway-tls)
│   Gateway   │  Ports: 443 (Web/API), 8090 (Agent CC)
└──────┬──────┘
       │ HTTPS (cert-manager internal CA)
       ├──────────────────────┐
       ▼                      ▼
┌─────────────┐        ┌─────────────┐
│  Frontend   │        │   Backend   │
│  (nginx)    │───────▶│  (Go API)   │◀── Agent Pods
│  :443 TLS   │  proxy │ :8080 :8090 │    (CC WebSocket)
└─────────────┘        └──────┬──────┘
                              │
                              ▼
                       ┌─────────────┐
                       │ PostgreSQL  │
                       └─────────────┘
```

## Uninstall

```bash

kubectl kustomize|kubectl delete -f -

```

# IAC Platform Kubernetes Deployment

## Prerequisites

- Kubernetes cluster (v1.27+)
- `kubectl` configured to access the cluster

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

## Directory Structure

```
manifests/
├── base/                        # Core resources
│   ├── namespace.yaml           # Namespace: terraform
│   ├── configmap.yaml           # Non-sensitive config (DB_HOST, ports, etc.)
│   ├── secret.yaml              # Sensitive config (DB credentials, JWT secret)
│   ├── ha-rbac.yaml             # ServiceAccount, Role, RoleBinding
│   ├── deployment-backend.yaml  # Backend (2 replicas, HTTPS)
│   ├── deployment-frontend.yaml # Frontend nginx (2 replicas, HTTPS)
│   ├── service-backend.yaml     # ClusterIP: 8080 (API) + 8090 (Agent CC)
│   └── service-frontend.yaml    # ClusterIP: 443
├── tls/                         # TLS certificates
│   ├── certificate.yaml         # cert-manager internal CA chain + service certs
│   ├── secret-gateway-tls.yaml  # Gateway external TLS certificate
│   └── certs/                   # mkcert certificate files (for local dev)
├── db/                          # Database initialization
│   ├── Dockerfile               # DB init container image
│   ├── entrypoint.sh            # psql entrypoint script
│   ├── init_seed_data.sql       # Schema + seed data
│   └── job-db-init.yaml         # K8s Job for DB initialization
└── gateway/                     # Envoy Gateway API
    ├── gateway.yaml             # GatewayClass + Gateway (HTTPS 443 + 8090)
    ├── httproute.yaml           # Routes: frontend(/), API(/api/,/health)
    └── backend-tls-policy.yaml  # Gateway → backend/frontend HTTPS policy
```

## Quick Deploy

```bash
# One-click deploy (ordered)
kubectl apply -f manifests/base/ && \
kubectl apply -f manifests/tls/ && \
kubectl apply -f manifests/gateway/ && \
kubectl apply -f manifests/db/job-db-init.yaml
```

## Step-by-Step Deploy

```bash
# 1. Create namespace, configmap, secrets, RBAC, deployments, services
kubectl apply -f manifests/base/

# 2. Wait for cert-manager to issue certificates
kubectl apply -f manifests/tls/
kubectl -n terraform get certificate -w
# Wait until READY=True for: iac-internal-ca, iac-platform-tls, iac-frontend-tls

# 3. Deploy Gateway and routing rules
kubectl apply -f manifests/gateway/

# 4. Initialize database (run once)
kubectl apply -f manifests/db/job-db-init.yaml
kubectl -n terraform wait --for=condition=complete job/iac-db-init --timeout=120s
```

## Configuration

Before deploying, update the following values to match your environment:

**`base/configmap.yaml`**
| Key | Description | Default |
|-----|-------------|---------|
| `DB_HOST` | PostgreSQL host | `10.179.219.54` |
| `DB_PORT` | PostgreSQL port | `15433` |
| `DB_NAME` | Database name | `iac_platform` |
| `DB_SSLMODE` | PostgreSQL SSL mode | `require` |
| `TZ` | Timezone | `Asia/Singapore` |

**`base/secret.yaml`**
| Key | Description | Default |
|-----|-------------|---------|
| `DB_USER` | Database user | `postgres` |
| `DB_PASSWORD` | Database password | `postgres123` |
| `JWT_SECRET` | JWT signing key | **must change** |

**`tls/secret-gateway-tls.yaml`**
- Replace with your own TLS certificate for external access (current: mkcert self-signed for `*.iac-platform.com`)

## Verify Deployment

```bash
# Check all resources
kubectl -n terraform get all,certificate,gateway,httproute

# Check pods are running
kubectl -n terraform get pods

# Check certificates are issued
kubectl -n terraform get certificate

# Test backend health endpoint (via port-forward)
kubectl -n terraform port-forward svc/iac-platform 8080:8080
curl -k https://localhost:8080/health
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
kubectl delete -f manifests/db/job-db-init.yaml
kubectl delete -f manifests/gateway/
kubectl delete -f manifests/tls/
kubectl delete -f manifests/base/
```

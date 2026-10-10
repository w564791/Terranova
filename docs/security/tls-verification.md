# TLS verification for internal clients

Terranova never disables certificate verification for its own clients:

| Client | Code | Trust |
|--------|------|-------|
| Agent → API (register, task data, logs, state upload) | `services/agent_api_client.go` | system roots + `IAC_CA_FILE` / `AGENT_CA_FILE` |
| Agent C&C WebSocket | `agent/control/cc_manager.go` | same |
| Terraform HTTP state backend (`TF_HTTP_ADDRESS`, run by the agent/server) | `services/terraform_executor.go` | `_TERRANOVA_CA_CERT` if set, else system roots + `IAC_CA_FILE` passed as `TF_HTTP_CLIENT_CA_CERTIFICATE_PEM` |
| Server outbound HTTP (run tasks, notifications, CMDB sources, SSO, AI providers, Terraform downloads) | `http.DefaultTransport` | system roots + `IAC_CA_FILE` (installed at startup) |

The TLS setup lives in `internal/tlstrust`. A test (`TestNoHardcodedInsecureSkipVerify`)
fails if any non-test Go source in `backend/` sets `InsecureSkipVerify`.

## Private CAs: `IAC_CA_FILE`

Set `IAC_CA_FILE` (server and agent; the agent also accepts `AGENT_CA_FILE`) to
a PEM file with the CA certificate(s) that sign your internal endpoints. They
are **added** to the system roots. The file is validated at startup: it must be
readable, contain at least one certificate and no private key; otherwise the
process refuses to start (in every mode).

Kubernetes agent pools: mount the CA from a ConfigMap and set
`IAC_CA_FILE=/etc/terranova-ca/ca.crt` in the pool's environment. git (Terraform
module sources) and Terraform providers use the image's trust store; add the CA
there too (`update-ca-trust` / `update-ca-certificates`) if they need it.

## Verification bypass options are refused in production

At startup the server and the agent look for options that request skipping
verification:

- `IAC_TLS_INSECURE_SKIP_VERIFY`, `IAC_INSECURE_SKIP_VERIFY`, `IAC_AGENT_TLS_INSECURE`,
  `AGENT_TLS_INSECURE`, `AGENT_INSECURE_SKIP_VERIFY`, `TLS_INSECURE`,
  `TLS_INSECURE_SKIP_VERIFY`, `TLS_SKIP_VERIFY`, `INSECURE_SKIP_VERIFY`,
  `SKIP_TLS_VERIFY` (true/1/yes/on): **not honored** by Terranova, which has
  no bypass; detected so nobody relies on them;
- `GIT_SSL_NO_VERIFY` (any value): honored by git, which the agent runs for
  Terraform module sources;
- `NODE_TLS_REJECT_UNAUTHORIZED=0`, `PYTHONHTTPSVERIFY=0`: honored by Node.js /
  Python child processes.

With `ENV=production` (the same switch as the key checks) any of them makes the
process refuse to start. Otherwise it starts with a loud warning naming each
option:

```
[TLS] WARNING: !!! agent: TLS verification bypass option GIT_SSL_NO_VERIFY is set (...). Allowed only because ENV is not production; the agent will refuse to start with it in production !!!
```

Agent mode: a production platform injects `ENV=production` into the agent
pods it creates (K8s pools, deployments, jobs); a pool template cannot
override it. Static agents started by hand must set `ENV=production`
themselves.

### Plaintext agent protocol (`http` / `ws`)

`IAC_AGENT_PROTOCOL` defaults to `http` (C&C WebSocket then uses `ws`). In
production (`ENV=production`) the agent **refuses to start** on plaintext
unless `IAC_AGENT_ALLOW_PLAINTEXT=cluster-internal` is set exactly; any other
value of that variable is a startup error. With the flag set it starts with a
loud warning. Outside production, plaintext still starts with a warning.

Platform-created K8s agent pods/deployments/jobs: a production platform that
still speaks `http` to in-cluster agents injects
`IAC_AGENT_ALLOW_PLAINTEXT=cluster-internal` so existing pools do not break.
Choice: opt-in the cluster-internal flag rather than force `https`, because
ClusterIP traffic is commonly plaintext today; operators who terminate TLS on
the platform set protocol `https` (or `PLATFORM_PROTOCOL=https`) and need no
flag. External / static agents must set `IAC_AGENT_PROTOCOL=https` (or the
exact allow flag if they intentionally use plaintext inside a trusted network).

## Not covered

Workspace-level settings belong to the workspace's own Terraform run and are
not checked here: e.g. a workspace environment variable `GIT_SSL_NO_VERIFY`,
`TF_CLI_ARGS_init=-backend-config=skip_cert_verification=true`, or a
workspace `TF_HTTP_CLIENT_CA_CERTIFICATE_PEM` (which takes precedence over the
platform CA for that workspace's state backend).

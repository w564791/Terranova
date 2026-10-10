# Private keys are never committed

This repository is public. Private keys (TLS, SSH, signing) must never be
tracked in git, not even for local development or tests.

## Where keys come from instead

| Need | Source |
|------|--------|
| Local HTTPS for Vite / local backend | `make dev-certs` writes `certs/localhost.pem` and `certs/localhost-key.pem` (gitignored). mkcert is used when installed (trusted local CA), otherwise an openssl self-signed certificate. `FORCE=1` regenerates; `DEV_CERT_HOSTS` sets the names. |
| Gateway TLS Secret `iac-gateway-tls` (kustomize) | `manifests/tls/kustomization.yaml` generates it from gitignored `manifests/tls/certs/tls.crt` / `tls.key`. `make dev-certs` copies the dev pair there; real environments supply their own certificate or manage the Secret with cert-manager / an external secret store. |
| Internal service certificates | cert-manager (`manifests/tls/certificate.yaml`), issued in-cluster. |
| Tests | Generate keys at runtime (`crypto/rsa`, `crypto/ecdsa`, `crypto/ed25519`, `httptest.NewTLSServer`, or `openssl` in a setup step). Do not add key fixtures. |

`.gitignore` covers `/certs/`, `/manifests/tls/certs/`, `*.pem`, `*.key` and `*-key.pem`.

## Guard

`scripts/check-private-keys.sh` fails when any tracked file (the git index)
contains

- a PEM private-key header matching `-----BEGIN [A-Z ]*PRIVATE KEY-----`
  (RSA, EC, DSA, OPENSSH, ENCRYPTED, PKCS#8, ...), or
- the same header base64-encoded, as in the `data:` of a Kubernetes Secret
  (`tls.key: LS0tLS1CRUdJTi...`).

It runs in CI as the `secrets-guard` job of `.github/workflows/ci.yml` and
locally via `make check-private-keys`.

Optional pre-commit hook (checks what is staged):

```bash
ln -sf ../../scripts/pre-commit-private-keys.sh "$(git rev-parse --git-common-dir)/hooks/pre-commit"
```

If you already use a pre-commit hook, call `scripts/pre-commit-private-keys.sh`
from it instead.

### Exclusions

`scripts/private-key-allowlist.txt` lists paths the guard skips, one per line.
It is empty. Only a clearly fake, test-only fixture may be added, with a comment
explaining why it cannot be generated at runtime.

## If a key was committed

Removing the file does not remove it from history, and the repository is
public: treat the key as compromised. Remove it from the tree, generate a new
key pair, replace every certificate issued for the old key, and (if required)
rewrite history separately.

### Leaked: mkcert gateway certificate (removed in 93aaba2)

`manifests/tls/certs/localhost-key.pem` and the base64 `tls.key` in
`manifests/tls/secret-gateway-tls.yaml` held the private key of:

| | |
|---|---|
| Subject | `O=mkcert development certificate, OU=ken@kens-MacBook-Pro.local` |
| Issuer | `mkcert ken@kens-MacBook-Pro.local` (mkcert development CA) |
| SANs | `iac-platform.com`, `www.iac-platform.com`, `api.iac-platform.com` |
| Valid | 2026-02-10 to 2028-05-10 (04:47:36 UTC) |
| Serial | `0AAD8EB0AEF0A674DE3E9509F2EC52AD` |
| SHA-256 fingerprint | `01:5F:23:22:92:D1:82:E3:BA:C7:1F:B8:95:74:FB:2F:75:CF:76:AB:4B:DD:C0:C5:7A:EE:DC:74:C8:3D:1E:7E` |

It is in the public history, so anyone can present it. Every environment that used it:

1. Replace the gateway certificate: `make dev-certs` (local) or your own
   certificate in `manifests/tls/certs/tls.crt` / `tls.key`, then redeploy
   `iac-gateway-tls`.
2. Remove trust in it: clients that trusted this leaf certificate or the
   `mkcert ken@kens-MacBook-Pro.local` CA (browsers, `IAC_CA_FILE`,
   `_TERRANOVA_CA_CERT`, OS trust stores) must stop trusting it. Only the
   leaf key leaked, not the CA key, but the leaf alone is enough to
   impersonate the three hostnames until 2028-05-10.
3. After the replacement, rotate the agent pool tokens (and any other
   credentials sent to those hostnames, e.g. run task/HMAC secrets and user
   sessions): traffic to an endpoint presenting this certificate may have
   been intercepted.

Agents verify certificates and refuse bypass options in production (see
`docs/security/tls-verification.md`); private CAs go in `IAC_CA_FILE`.

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

The mkcert localhost / `*.iac-platform.com` key that used to live in
`manifests/tls/certs/localhost-key.pem` and `manifests/tls/secret-gateway-tls.yaml`
is such a key: every environment that used it must run `make dev-certs` (or
install its own certificate) and redeploy `iac-gateway-tls`.

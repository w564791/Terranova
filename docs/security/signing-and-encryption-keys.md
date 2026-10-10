# Signing and encryption keys

Until this change one secret, `JWT_SECRET`, did everything: it signed login,
user API and team API tokens and Run Task callback tokens directly, signed
state-backend tokens as `"state:" + JWT_SECRET`, and `SHA-256(JWT_SECRET)` was
the AES-256-GCM key for sensitive variables (and every other
`internal/crypto` value: MFA secrets, SSO tokens, notification secrets, CMDB
source credentials, Run Task access tokens). Leaking or rotating it affected
all of them at once.

It is now split into two independent roots, neither derived from `JWT_SECRET`:

| Variable | Use | Format |
|---|---|---|
| `DATA_ENCRYPTION_KEY` | AES-256-GCM key for `internal/crypto` values | 32 bytes, base64 or hex |
| `SIGNING_ROOT_KEY` | HKDF-SHA256 root of every JWT signing key | >= 32 bytes, base64 or hex |
| `JWT_SECRET` | legacy only (see below) | unchanged |

Generate `DATA_ENCRYPTION_KEY` with `openssl rand -base64 32` (exactly 32
bytes) and `SIGNING_ROOT_KEY` with `openssl rand -base64 48`. Keys are never
committed: the Kubernetes manifests generate the `iac-jwt` Secret from the
gitignored `manifests/base/keys.env` (template `keys.env.example`), docker
compose and the Makefile read them from the gitignored `.env`
(`make generate-secret`). Both are read through
`internal/keys.KeyProvider` (`keys.DataKeys`, `keys.SigningRoots`); the
environment provider is the default, a KMS provider can be installed instead
(`internal/keys/kms.go`).

## Startup rules

* `ENV=production`: a missing `DATA_ENCRYPTION_KEY` or `SIGNING_ROOT_KEY`
  refuses to start (`[Keys] refusing to start`).
* Otherwise (development): a missing key logs a loud WARNING and that side runs
  in **legacy mode**, i.e. exactly the old `JWT_SECRET` behaviour.
* A key that is set but malformed (wrong length / encoding), or a malformed
  `LEGACY_TOKEN_CUTOFF` / `LEGACY_TOKEN_ISSUED_BEFORE`, refuses to start in
  every mode.
* `JWT_SECRET` is required in legacy signing / encryption mode, and whenever
  any sensitive variable row still has `key_version = 0` (legacy ciphertext,
  or plaintext not yet encrypted; only `JWT_SECRET` can tell them apart):
  after connecting to the database the server counts such rows in
  `workspace_variables` and `varset_variables` and refuses to start without
  `JWT_SECRET` if any remain, naming the tables and counts. This does not
  depend on `LEGACY_TOKEN_CUTOFF`: data stays legacy until it is re-encrypted,
  however long that takes.

## Signing

Per-purpose keys: `HKDF-SHA256(SIGNING_ROOT_KEY, salt = none,
info = "terranova/jwt/<purpose>")`, purposes `user` (login, user API and team
API tokens), `state` (state backend), `runtask` (Run Task callbacks), and
`agent` / `run` (reserved for per-agent and manifest run tokens). A token of one
purpose never verifies as another.

Every new token is HS256 with header `kid = "<purpose>-v<version>"`, version
from `SIGNING_ROOT_KEY_VERSION` (default 1).

### Rotating the signing root

1. Set `SIGNING_ROOT_KEY_PREVIOUS` / `SIGNING_ROOT_KEY_PREVIOUS_VERSION` to the
   current key and its version, and `SIGNING_ROOT_KEY` /
   `SIGNING_ROOT_KEY_VERSION` to a new key with a higher version. Deploy.
   New tokens carry the new kid; tokens with the previous kid keep verifying.
2. After the longest token lifetime, remove the `_PREVIOUS` variables. (API
   tokens with longer lifetimes must be reissued first.)

## Encryption

New values are written as `tnk<version>:` + base64(nonce | ciphertext) under
`DATA_ENCRYPTION_KEY` version `<version>` (`DATA_ENCRYPTION_KEY_VERSION`,
default 1). `workspace_variables` and `varset_variables` also record it in the
new `key_version` column (migration `20261010_10_variable_key_version`);
decryption selects the key by `key_version` and refuses a value whose prefix
disagrees. Version 0 means legacy (`SHA-256(JWT_SECRET)`), decrypt only.

Re-encryption: the leader runs `services.ReencryptLegacyVariables` once at
startup. It reads only sensitive rows with `key_version = 0` and rewrites each
with a compare-and-set on `(id, key_version = 0, value)`, so it is idempotent
and safe to run concurrently or repeatedly; rows already on a data key are
never touched. Each pass logs `legacy rows remaining=<n>` per table.

Whether a value is encrypted is decided by explicit markers only, never by
its shape (a plaintext secret can be valid base64, e.g. most cloud secret
keys):

* a `tnk<version>:` prefix (and the row's `key_version`, where recorded) is a
  ciphertext under that data-key version;
* an unprefixed value in a `key_version = 0` row is a legacy ciphertext only
  if it **authenticates** (AES-256-GCM) under `SHA-256(JWT_SECRET)`;
* everything else is plaintext. Reads return it as is; writes encrypt it.

The job therefore rewrites three kinds of `key_version = 0` sensitive rows:
prefixed values (column relabelled), legacy ciphertexts (re-encrypted) and
plaintext values that were stored unencrypted, including base64-looking ones
that older code mistook for ciphertexts (encrypted). Without `JWT_SECRET` a
legacy ciphertext cannot be told from plaintext, so the job refuses to
classify unprefixed rows and the server refuses to start while any
`key_version = 0` sensitive row remains. `JWT_SECRET` must be the
environment's real legacy secret: with a wrong one, legacy ciphertexts fail
authentication and are wrapped as plaintext (recoverable by decrypting the
wrapped value and opening it with the right key, but the application would use
the wrong value until then). Check `reencrypted=` vs `encrypted(plaintext)=` in
the log of the first pass.

Other encrypted columns (MFA, SSO, notification, CMDB source, Run Task access
tokens) are self-describing (`tnk<v>:` prefix): they are written with the data
key from now on and legacy values still decrypt with `JWT_SECRET`; they are
re-encrypted when rewritten, not by the job yet.

Rotating the data key: move the current key to `DATA_ENCRYPTION_KEY_PREVIOUS`
(+ `_PREVIOUS_VERSION`), set a new `DATA_ENCRYPTION_KEY` with a higher
`_VERSION`. Values of the previous version keep decrypting; rewriting them
under the new version is a follow-up of the same job (today it only converts
version 0).

## Legacy tokens (no `kid`)

Tokens issued before the switch have no `kid` and are signed with
`JWT_SECRET` (`"state:" + JWT_SECRET` for state tokens). They are verified
with the legacy key only while all of the following hold:

* `LEGACY_TOKEN_CUTOFF` (RFC3339) is set and `now <= cutoff`. This is the
  control: past it every no-kid token is rejected, whatever its `exp`. Set it
  to **deploy time + the longest lifetime you need to honour**: login tokens
  24 h, state tokens 7 days (in-flight tasks), Run Task tokens 1 h. User / team
  API tokens can have longer or no expiry; they stop working at the cutoff and
  must be reissued before it.
* The token has an `iat` and it is not after `LEGACY_TOKEN_ISSUED_BEFORE`
  (RFC3339, the deploy time; defaults to the process start). `iat` is chosen
  by whoever signs the token, so this only catches tokens minted with the old
  secret after the switch; the cutoff is what bounds the window.

`LEGACY_TOKEN_CUTOFF` unset means **no legacy token is accepted** (fail
closed), also in production; the server still starts. Making it mandatory
would force operators to keep a stale timestamp in the config forever after the
window; leaving it unset simply means users log in again, API tokens are
reissued and tasks in flight at deploy time lose their state token. Set it if
that is not acceptable.

In development legacy mode (no `SIGNING_ROOT_KEY`) no-kid tokens are what is
issued, so they are accepted without the cutoff.

### After the window: retiring JWT_SECRET

`JWT_SECRET` can be removed only when **both** hold:

* the re-encryption job reports **zero legacy rows** (`[Keys] variable
  re-encryption: ... legacy rows remaining=0`); otherwise the server refuses to
  start without it. The other `internal/crypto` columns (MFA, SSO,
  notification, CMDB source, Run Task access tokens) have no `key_version`
  column and are not covered by that check or job yet: confirm none of them
  still holds an unprefixed (legacy) value before removing it;
* `LEGACY_TOKEN_CUTOFF` has passed (no no-kid token is accepted any more).

Then:

1. Remove the legacy verification path (`keys.Keyfunc` no-kid branch,
   `LegacyStateSecret`, `LegacyJWTSecret`; TODO in `internal/keys/signing.go`)
   and `LEGACY_TOKEN_CUTOFF` / `LEGACY_TOKEN_ISSUED_BEFORE`.
2. Remove `JWT_SECRET` from the deployment and rotate it out of any secret
   store (the shipped manifests generated it predictably from
   time/PID/hostname, or used the committed value, see below).

## Environments that used a committed or default JWT_SECRET

This repository is public. `manifests/base/kustomization.yaml` contained a
literal `JWT_SECRET` from commit `3b140f8` until it was replaced by the
gitignored `keys.env`; `docker-compose*.yml` defaulted to
`change-me-to-a-random-secret-key` and the Makefile dev targets to
`dev-secret-key-change-in-production`. Any environment that ever ran with one of
those values (or with the README's time/PID/hostname value, which is
guessable) must assume that anyone can forge tokens without `kid` (login,
user / team API, state, Run Task) and can decrypt every legacy value
(`key_version = 0` rows and unprefixed `internal/crypto` values) from a
database dump or backup. For those environments the rollout of this change is:

1. Set `LEGACY_TOKEN_CUTOFF` to the **deploy time** (no legacy token window;
   leaving it unset is equivalent). Every no-kid token is rejected from the
   first start: users log in again, user / team API tokens are reissued,
   tasks in flight at deploy time lose their state token. Do **not** use a
   window: during it the public secret would still mint valid tokens.
2. Set `DATA_ENCRYPTION_KEY` and `SIGNING_ROOT_KEY` (never derived from, or
   equal to, the old value). In development legacy mode (no
   `SIGNING_ROOT_KEY`) tokens are still signed with `JWT_SECRET`; do not run
   an exposed environment in that mode.
3. Rotate `JWT_SECRET` now: after step 1 its only remaining use is decrypting
   legacy data, so keep the old value only for the re-encryption pass
   (`legacy rows remaining=0`, and the other `internal/crypto` columns checked
   as described above), then remove it from the deployment and every secret
   store in the same rollout. Never reuse it anywhere.
4. Treat every secret that was stored legacy-encrypted as disclosed and rotate
   it at its source (cloud credentials in sensitive variables, MFA enrolments,
   SSO / notification / CMDB source / Run Task credentials).

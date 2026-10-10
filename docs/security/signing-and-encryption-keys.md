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

Generate each with `openssl rand -base64 32`. Both are read through
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
  any sensitive variable row still has `key_version = 0` (legacy-encrypted):
  after connecting to the database the server counts such rows in
  `workspace_variables` and `varset_variables` (and legacy `plan_data`
  envelopes) and refuses to start without
  `JWT_SECRET` if any remain, naming the tables and counts. This does not
  depend on `LEGACY_TOKEN_CUTOFF`: data stays legacy until it is re-encrypted,
  however long that takes.

## Signing

Per-purpose keys: `HKDF-SHA256(SIGNING_ROOT_KEY, salt = none,
info = "terranova/jwt/<purpose>")`, purposes `user` (login, user API and team
API tokens), `state` (state backend), `runtask` (Run Task callbacks), and
`agent` (per-agent tokens; see api-fix-tasks/14-manifest.md item 20) and
`run` (reserved for manifest run tokens). A token of one purpose never
verifies as another. `agent` tokens have no legacy (JWT_SECRET) scheme: without
`SIGNING_ROOT_KEY` (development legacy mode) none are issued and agents keep
using the pool token.

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
never touched. Each pass logs `legacy rows remaining=<n>` per table. A base64 value that the legacy key cannot open is left alone and
counted (`undecryptable`) rather than risk encrypting a ciphertext as if it were
plaintext.

Binary plans (`workspace_tasks.plan_data`, envelope encryption, see
`internal/crypto/plan_envelope.go`): envelope version 2 wraps the per-plan
data key with a KEK derived from `DATA_ENCRYPTION_KEY` and records its
`key_version` in the authenticated header (`"TNPD" | 0x02 | key_version |
expires_at`); version 1 envelopes are the legacy ones (key version 0, KEK from
`JWT_SECRET`), decrypt only. `CleanupPlanData` (leader, startup and periodic)
re-seals version 1 envelopes under the current data key with the same expiry,
compare-and-set on the old bytes (idempotent; only version 1 rows). The key
version is in the header rather than a column because plan_data has several
writers and SQL can still find legacy rows (`get_byte(plan_data, 4) = 1`);
the startup check counts them too.

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
  re-encryption: ... legacy rows remaining=0`, plan_data included); otherwise the server refuses to
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
   time/PID/hostname).

# RBAC + UX Overhaul — Design

Date: 2026-06-24
Status: Approved (pending spec review)

## Goal

Make roles mean something, make the bot self-describing per user, and make
permission changes auditable. Today `operator`/`viewer` are empty labels —
every capability must be granted by hand — and `/help` is static text that
shows commands a user may not be allowed to run.

Four outcomes, all in scope:

1. **Roles that mean something** — `viewer`/`operator`/`admin` become presets,
   scopable per namespace. `/role moz viewer -n prod` grants all read access in
   `prod` in one command.
2. **Discoverability UX** — per-user `/help`, enriched `/start`, visible
   selector restrictions, actionable denial messages.
3. **Auditability & safety** — grant/revoke/role changes recorded as K8s Events.
4. **Easier admin grant flow** — `/role` with namespace flag replaces hand-listing
   verbs.

## Non-goals (YAGNI)

Explicitly out of scope; revisit only if a real need appears:

- OR / negated label selectors (AND-only stays).
- Interactive button wizard for grant (flag form is enough).
- Bulk grant / grant to user groups.
- A 4th `deployer` tier.
- JSON output, pagination.
- Persisting pending confirmations across bot restart (in-memory map + 2-min
  TTL stays; restart is rare and a stale confirm simply expires safely).

---

## 1. Data model (CRD change)

Add `roleBindings[]` to `TelegramBotPermission.spec`. Keep `permissions[]`
(fine-grained `/grant`) and the legacy flat `role` field for back-compat.

```yaml
apiVersion: kbot.go.mamad.dev/v1
kind: TelegramBotPermission
metadata:
  name: user-12345
spec:
  telegramUserId: 12345
  roleBindings:
    - role: viewer        # viewer | operator | admin
      namespace: prod     # "*" for all namespaces
      selector: app=web   # optional; applied to every verb the role expands to
    - role: operator
      namespace: staging
  permissions: []         # unchanged; still honored for fine-grained /grant
  role: ""                # legacy; auto-migrated on read (see Migration)
```

### Role catalog — single source of truth

New file `internal/rbac/roles.go`. One map defines what each tier expands to.
Changing a preset here changes it for every user.

```
viewer   verbs:  get, list, logs, describe, events, top
         resources: pods, deployments, services
operator verbs:  viewer + restart, rollback, scale
         resources: pods, deployments, services
admin    cluster-wide: all namespaces, all resources, all verbs
```

Notes:
- `describe`, `events`, `top` must exist as recognized verbs. Confirm against the
  CRD verb enum (`manifests/crd.yaml`) and the verb strings the handlers pass to
  `CheckPermission`. If the handlers currently check `get`/`list` for these, the
  catalog maps to whatever verb strings the handlers actually use — the catalog
  must match handler call sites exactly, not invent new verb names. This is a
  build-time invariant to verify, not an assumption.
- `admin` in a roleBinding is equivalent to the legacy `role: admin` god-mode.

### Migration (auto, on read)

When the manager reads a CR, normalize legacy `role` into the new model in
memory (do not rewrite the CR unless it is otherwise being updated):

- `role: viewer` or `role: operator` → synthesize a `roleBinding{role, namespace: "*"}`.
- `role: admin` → keep treating as cluster god-mode (a single
  `roleBinding{role: admin, namespace: "*"}` is equivalent; either representation
  is fine as long as the validator short-circuits).
- `role: ""` with `roleBindings` present → use `roleBindings` as-is.

Old CRs keep working with no manual migration. New writes via `/role` populate
`roleBindings` and leave `role` empty.

---

## 2. Validator changes

`internal/rbac/validator.go`.

`CheckPermission` gains an expansion step:

1. Bootstrap admin (env) → allow (unchanged).
2. Fetch CR; run legacy→roleBindings normalization.
3. If any binding is `role: admin` (or legacy admin) → allow, unrestricted
   (unchanged short-circuit).
4. Expand each non-admin `roleBinding` through the role catalog into effective
   `(namespace, resource, verb, selector)` rules.
5. Check the requested operation against **the union of** expanded rules **and**
   existing `permissions[]`. Namespace wildcard (`*`), resource match, verb match
   as today.
6. Selector enforcement is unchanged: a binding's selector flows into the same
   `validateSelector` path used by `permissions[]`. `Decision.EffectiveSelector`
   is set from whichever matching rule carried a selector so list/get handlers
   keep filtering correctly.

`ValidateAndGetNamespaces` must also walk `roleBindings` (not just `permissions[]`)
when collecting the set of accessible namespaces.

`FormatPermissionDenied` (see §4) gains context about what role the user *does*
have in the target namespace so it can suggest a fix.

---

## 3. Admin command UX

`internal/bot/handlers.go`, `bot.go` dispatch.

```
/role <user> <viewer|operator|admin> [-n <ns>] [-l <selector>]
        # add or replace the role binding for that namespace
        # -n omitted  => namespace "*"  (everywhere)
/role <user> none -n <ns>
        # remove the role binding for that namespace
        # "none" with no -n removes the "*" binding
/grant ...     # unchanged — fine-grained capability add
/revoke ...    # unchanged — fine-grained capability remove
```

Manager gains:
- `SetRoleBinding(userID, role, namespace, selector)` — upsert into `roleBindings`,
  replacing any existing binding for the same namespace. Reuses the
  read-modify-write + optimistic-retry path (`mutateUserPermission`).
- `RemoveRoleBinding(userID, namespace)` — delete the binding for a namespace.

`/role` with an unknown tier returns a clear error listing valid tiers.

`SetRole` (legacy single-field setter) stays callable but is superseded; `/role`
routes to `SetRoleBinding`. Decide at implementation time whether to keep
`SetRole` as a thin wrapper or remove its handler wiring — no external callers
besides the bot.

---

## 4. Discoverability UX

`internal/bot/handlers.go`, plus a small capability-introspection helper.

### Per-user `/help`

`/help` lists only commands the caller can run, derived from effective
permissions:

- Everyone with any access: `/start`, `/help`, `/whoami`, `/namespaces`,
  `/permissions` (self).
- Read verbs present somewhere → show `/pods /deployments /services /logs
  /describe /events /top`.
- Write verbs (restart/rollback/scale) present → show those.
- Admin → show the admin section (`/grant /revoke /role /users /selfupdate`).

Implementation: a helper that summarizes a user's effective capabilities (reads?
writes? admin?) and `/help` renders sections conditionally. Static command
reference text is fine within each shown section.

### Enriched `/start` (+ `/whoami` alias)

`/start` stays (Telegram convention — sent on first contact) and is enriched.
`/whoami` is registered as an alias to the same handler.

Output shows identity + effective roles + what they unlock:

```
You are user 12345.
Roles:
  • viewer  in prod
  • operator in staging
You can: list/inspect resources, view logs (prod, staging);
         restart/rollback/scale (staging).
Type /help for commands.
```

### Selector visibility

When a list/get result is narrowed by a permission/binding selector, append a
footer to the message:

```
🔒 filtered to app=web
```

So users understand why they see a subset rather than assuming the namespace is
empty. Applies to `/pods /deployments /services` and anywhere
`Decision.EffectiveSelector` is non-empty.

### Actionable denials

`FormatPermissionDenied` upgrades from `❌ <reason>` to include the user's
standing in the target namespace and a suggested fix:

```
❌ No 'restart' access to deployments in 'prod'.
   You have viewer there. Ask an admin: /role <you> operator -n prod
```

If the user has no binding at all in that namespace, say so and suggest viewer.

---

## 5. Auditability

`internal/k8s` event recording (the existing `RecordEvent` path used by
restart/scale) is extended to permission mutations.

Record a K8s Event on the bot's namespace for each of:
- `SetRoleBinding`  → reason `PermissionGranted`, message
  `admin <adminID> set user <id> = <role> in <ns>`
- `RemoveRoleBinding` → reason `PermissionRevoked`
- `/grant`, `/revoke` → reasons `PermissionGranted` / `PermissionRevoked`

Events are emitted from the bot handlers (which know the acting admin's ID),
after the manager mutation succeeds. Failure to record an event must not fail the
operation — log and continue.

---

## Affected files (map)

| File | Change |
|------|--------|
| `manifests/crd.yaml` | Add `roleBindings` to schema; confirm verb enum covers describe/events/top |
| `internal/rbac/types.go` | Add `RoleBinding` type + `RoleBindings []RoleBinding` field |
| `internal/rbac/roles.go` (new) | Role catalog: tier → verbs×resources |
| `internal/rbac/validator.go` | Expand roleBindings at check time; namespaces walk bindings; richer denial |
| `internal/rbac/manager.go` | Legacy→binding normalization on read; `SetRoleBinding`/`RemoveRoleBinding`; permission-summary includes bindings |
| `internal/bot/handlers.go` | `/role` rewrite, per-user `/help`, enriched `/start`+`/whoami`, selector footer, denial wiring |
| `internal/bot/bot.go` | Register `/whoami`; dispatch updates |
| `internal/k8s/*` (event path) | Emit events for permission mutations |

## Testing

- **Validator unit tests**: a `viewer -n prod` binding allows `list pods -n prod`,
  denies `restart` in prod, denies anything in `staging`; `operator` allows
  restart; admin binding allows all; selector on a binding filters and denies
  non-matching named resources; `permissions[]` + bindings union works.
- **Migration tests**: legacy `role: viewer` behaves identically to a
  `roleBinding{viewer, "*"}`; legacy `admin` stays god-mode.
- **Manager tests**: `SetRoleBinding` upserts (replace same-ns, add new-ns);
  `RemoveRoleBinding` deletes only the targeted ns; optimistic retry path intact.
- **Command/UX**: `/role moz viewer -n prod` round-trips into a binding;
  `/help` hides admin section for non-admins; denial message names current role +
  fix.
- Run `go test ./...` and `go vet`. Confirm verb-string parity between role
  catalog and handler call sites (build-time invariant).

## Open implementation checks (resolve during build, not assume)

1. Exact verb strings the handlers pass for describe/events/top — catalog must
   match these, and the CRD verb enum must include them.
2. Whether `services` should support a `describe`/selector path consistently with
   pods/deployments (today services bypass selector validation).
3. Where the bot namespace name comes from for audit events (`BOT_NAMESPACE`
   config already exists — reuse it).

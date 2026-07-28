---
title: Globals — ASL
description: Session-scoped global variables, read in AQL and set from the client
---

# Globals

A `global` is a request-scoped variable — the authenticated user, the active
tenant, a timezone — that your queries and [policies](/asl/policies) can read
without threading it through every call. It's declared at the top level:

```asl
global current_user: uuid;      # optional by default
global required tenant: str;
```

- **Leading `global`**, then an optional **`required`** modifier, then
  `name: type`. Omitting `required` makes the global optional.
- The type is any scalar (`uuid`, `str`, `int64`, …) — not an object type.

Globals are **not DDL**: nothing is added to a migration for a `global` line. They
are backed by a Postgres [session setting](https://www.postgresql.org/docs/current/functions-admin.html#FUNCTIONS-ADMIN-SET)
(a custom GUC, `app.<name>`).

## Reading a global

Reference it in any AQL expression — a policy predicate, a query `filter`, a
computed field — as `global <name>`:

```asl
type Doc {
  required owner: uuid;
  policy owner_only for all
    using ( .owner = global current_user );
}
```

It lowers to a read of the backing session setting:

```sql
owner = current_setting('app.current_user', true)::UUID
```

**Optional vs required** maps to `current_setting`'s missing-ok flag:

| Declaration                 | Lowering                                       | When unset      |
| --------------------------- | ---------------------------------------------- | --------------- |
| `global current_user: uuid` | `current_setting('app.current_user', true)`    | `NULL`          |
| `global required tenant: str` | `current_setting('app.tenant', false)`       | **raises** (fail-closed) |

A required global that hasn't been set makes the query error rather than silently
matching or returning nothing — the safe default for a tenant/isolation guard.

## Setting a global from the client

`axel codegen` emits one `with<Name>` helper per global on the generated `Runner`.
It opens a transaction, pushes the value into the session with
`set_config('app.<name>', $1, true)` (bound as a parameter — never interpolated —
and transaction-local), then runs your queries against a client bound to that
transaction. This is safe under connection pooling: the setting can't leak to the
next borrower of the connection.

::: code-group

```ts [TypeScript]
await runner.withCurrentUser(userId, async (q) => {
  // every query here sees current_setting('app.current_user') = userId
  return q.listDocs({ /* … */ });
});
```

```go [Go]
err := runner.WithCurrentUser(ctx, userID, func(q *generated.Queries) error {
    _, err := q.ListDocs(ctx, generated.ListDocsParams{ /* … */ })
    return err
})
```

:::

Because the setting is transaction-local, the queries you want it applied to must
run **inside** the callback.

### Without the Runner

The standalone query functions also accept globals directly — Go via functional
options, TypeScript via a trailing options object. When any global is passed, the
function runs its query inside a transaction that first applies the globals
(compiled INSERTs carry their own `BEGIN/COMMIT`, which is stripped so it doesn't
nest). With no options, the call is unchanged.

::: code-group

```go [Go]
doc, err := generated.CreateDoc(ctx, db, params, generated.WithCurrentUser(userID))
```

```ts [TypeScript]
const doc = await createDoc(db, params, { currentUser: userId });
```

:::

## The GUC namespace

Globals live under the fixed `app.` prefix (`global current_user` →
`app.current_user`). If you set the value yourself outside the generated helper,
use the same name:

```sql
SELECT set_config('app.current_user', '…uuid…', true);  -- transaction-local
```

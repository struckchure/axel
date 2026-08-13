---
title: Expiring rows + cleanup — Examples
description: A TTL policy that hides stale rows, swept by a scheduled job
---

# Expiring rows + cleanup

Give rows a time-to-live: hide them from reads once they expire, and delete them
on a schedule. A [policy](/asl/policies) filters expired rows out of `SELECT`; a
[`@for`](/asl/functions) function registers a [pg_cron](/asl/extensions) job to
purge them.

```asl
use extension 'pg_cron';

type Session extending Base {
  required token: str { constraint exclusive; };
  expires_at: datetime;

  policy hide_expired for select
    using ( .expires_at is null or .expires_at >= now() );
}

@for Session
function session_gc() -> int64 {
  return cron.schedule('session-gc', '*/5 * * * *',
    aql`delete Session filter .expires_at < now()`);
};
```

- The policy makes a query see only rows that are **not yet expired** (or have no
  expiry) — the app never has to add a `WHERE expires_at >= now()`.
- The sweep is written as [inline AQL](/asl/functions#inline-aql-aql): it compiles
  to `DELETE FROM "session" … ` while the migration is generated, so a rename in
  the type is caught at build time rather than silently breaking the cron job.
- `@for Session` marks `session_gc` as a run-once setup function: Axel invokes it
  a single time (`SELECT session_gc();`) in the migration that first creates it,
  scheduling the cron job. See [Functions → `@for`](/asl/functions).
- The cleanup job connects as a privileged role and bypasses RLS, so it can see
  and delete the expired rows the app can't.

Reading is just a normal select — expired sessions are already invisible:

```aql
select Session { id, token } filter .token = $token<str>;
```

## From generated code

Saved as `get_session.aql`, the lookup returns the row or `null` — and an expired
session reads as `null` because the policy has already filtered it out:

::: code-group

```ts [TypeScript]
const session = await runner.query.getSession({ token }); // GetSessionRow | null
if (!session) throw new Error("invalid or expired session");
```

```go [Go]
session, err := runner.Query.GetSession(ctx, gen.GetSessionParams{Token: token}) // *GetSessionRow
if session == nil {
    return errors.New("invalid or expired session")
}
```

:::

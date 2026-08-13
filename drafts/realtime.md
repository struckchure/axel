# PostgreSQL WAL-Based Realtime Communication Service

## 1. Purpose

Build a standalone realtime communication service that consumes PostgreSQL logical replication changes and exposes them to connected clients through WebSockets.

The service should provide a Supabase-Realtime-like capability without requiring the application to implement database triggers or manually publish events.

The core flow is:

```text
PostgreSQL
    │
    │ WAL
    ▼
Logical Replication
    │
    ▼
Replication Consumer
    │
    ▼
Change Decoder
    │
    ▼
Event Pipeline
    │
    ├── Subscription Matching
    ├── Authorization
    └── Event Transformation
    │
    ▼
Connection Manager
    │
    ▼
WebSocket
    │
    ├── Browser
    ├── Mobile
    └── Backend Clients
```

---

# 2. Goals

The system must:

- Consume PostgreSQL logical replication streams.
- Preserve PostgreSQL transaction boundaries.
- Track and persist replication LSNs.
- Recover automatically after service restarts.
- Avoid losing database changes while the replication slot is retained.
- Support multiple PostgreSQL tables.
- Support `INSERT`, `UPDATE`, and `DELETE`.
- Allow clients to subscribe to database changes.
- Allow filtering subscriptions by schema, table, operation, and column values.
- Authenticate realtime clients.
- Authorize access to subscriptions.
- Deliver events over WebSockets.
- Support thousands of concurrent connections.
- Handle slow/disconnected clients without blocking WAL consumption.
- Expose health and operational metrics.
- Provide structured logs.
- Make event delivery horizontally scalable.

---

# 3. Non-Goals

The initial implementation should not attempt to:

- Replace PostgreSQL logical replication.
- Become a general-purpose message broker.
- Provide durable client-side message queues.
- Guarantee exactly-once WebSocket delivery.
- Execute arbitrary SQL supplied by clients.
- Expose raw PostgreSQL WAL to clients.
- Couple realtime delivery directly to application-specific business logic.

The system should treat PostgreSQL as the source of truth.

---

# 4. High-Level Architecture

```text
                       ┌─────────────────────┐
                       │     PostgreSQL      │
                       │                     │
                       │  WAL                │
                       │   │                 │
                       │   ▼                 │
                       │ Publication         │
                       │   │                 │
                       │   ▼                 │
                       │ Replication Slot    │
                       └──────────┬──────────┘
                                  │
                                  │ Logical Replication
                                  ▼
                       ┌─────────────────────┐
                       │ Replication Reader  │
                       └──────────┬──────────┘
                                  │
                                  ▼
                       ┌─────────────────────┐
                       │ Transaction Decoder │
                       └──────────┬──────────┘
                                  │
                                  ▼
                       ┌─────────────────────┐
                       │ Change Normalizer   │
                       └──────────┬──────────┘
                                  │
                                  ▼
                       ┌─────────────────────┐
                       │ Event Dispatcher    │
                       └──────┬───────┬──────┘
                              │       │
                    ┌─────────┘       └──────────┐
                    ▼                            ▼
          ┌─────────────────┐          ┌─────────────────┐
          │ Subscription    │          │ Authorization   │
          │ Matcher         │          │ Engine          │
          └────────┬────────┘          └─────────────────┘
                   │
                   ▼
          ┌─────────────────┐
          │ Connection      │
          │ Manager         │
          └────────┬────────┘
                   │
                   ▼
              WebSockets
                   │
          ┌────────┼────────┐
          ▼        ▼        ▼
       Client A Client B Client C
```

---

# 5. Core Components

## 5.1 PostgreSQL

PostgreSQL is the authoritative source of database state.

Required capabilities:

- WAL
- Logical replication
- Publications
- Logical replication slots

Example:

```sql
CREATE PUBLICATION realtime
FOR TABLE users, orders, messages;
```

The service must never modify application data as part of normal realtime processing.

---

# 5.2 Publication Manager

Responsible for determining which PostgreSQL tables are available for realtime replication.

Responsibilities:

- Discover publications.
- Validate configured tables.
- Optionally create/manage the realtime publication.
- Detect newly added tables.
- Validate replica identity requirements.
- Expose publication status.

The service should support both:

```text
Managed publication
```

and:

```text
Externally managed publication
```

In managed mode, the service may create:

```sql
CREATE PUBLICATION realtime ...
```

In external mode, database administrators own publication configuration.

---

# 5.3 Replication Slot

A dedicated logical replication slot should be created for the realtime service.

Example:

```sql
SELECT pg_create_logical_replication_slot(
    'realtime_slot',
    'pgoutput'
);
```

The slot represents the durable position of the realtime consumer.

Responsibilities:

- Preserve required WAL.
- Maintain replication position.
- Allow recovery after service restart.

The slot must never be silently deleted.

The service should detect excessive replication lag and expose alerts/metrics.

---

# 5.4 Replication Reader

The replication reader maintains the PostgreSQL replication connection.

Responsibilities:

- Establish replication connection.
- Authenticate.
- Start replication.
- Receive WAL messages.
- Send standby status updates.
- Track received LSN.
- Track flushed/acknowledged LSN.
- Reconnect after failure.
- Resume from the last acknowledged position.

Conceptually:

```text
PostgreSQL
    │
    │ XLogData
    ▼
Replication Reader
    │
    │ feedback
    ▼
PostgreSQL
```

The reader must not block because of WebSocket clients.

---

# 5.5 Transaction Decoder

The decoder translates PostgreSQL logical replication messages into internal events.

It must understand at minimum:

- Begin
- Commit
- Relation
- Insert
- Update
- Delete
- Truncate
- Origin, where applicable

Example internal event:

```json
{
  "transaction_id": "...",
  "lsn": "0/1234567",
  "operation": "INSERT",
  "schema": "public",
  "table": "messages",
  "columns": {
    "id": 100,
    "room_id": 10,
    "content": "Hello"
  }
}
```

The decoder must preserve transaction ordering.

---

# 5.6 Change Normalizer

The normalizer converts PostgreSQL-specific replication messages into the service's stable internal event model.

PostgreSQL-specific details should not leak into the WebSocket protocol.

Internal representation:

```text
DatabaseChange
├── LSN
├── Transaction ID
├── Transaction sequence
├── Operation
├── Schema
├── Table
├── New Record
├── Old Record
├── Changed Columns
└── Commit Timestamp
```

Example:

```json
{
  "type": "database_change",
  "operation": "UPDATE",
  "schema": "public",
  "table": "users",
  "record": {
    "id": 10,
    "name": "Mohammed"
  },
  "old_record": {
    "id": 10,
    "name": "John"
  }
}
```

---

# 5.7 Event Dispatcher

The dispatcher is responsible for routing normalized database events.

Responsibilities:

- Receive committed transactions.
- Find matching subscriptions.
- Apply authorization.
- Apply filters.
- Produce client events.
- Forward events to the connection manager.

The dispatcher must not directly depend on WebSocket implementation details.

---

# 5.8 Subscription Manager

Maintains active client subscriptions.

A subscription should contain:

```text
Subscription
├── ID
├── Client ID
├── Connection ID
├── Schema
├── Table
├── Event Type
├── Filter
└── Authorization Context
```

Example:

```json
{
  "schema": "public",
  "table": "messages",
  "event": "INSERT",
  "filter": {
    "room_id": 123
  }
}
```

A client may maintain multiple subscriptions over one WebSocket connection.

---

# 5.9 Subscription Matcher

Matches database changes against active subscriptions.

For example:

```text
Change:

public.messages
INSERT
room_id = 123
```

Subscriptions:

```text
Client A → messages / room_id=123     ✓
Client B → messages / room_id=456     ✗
Client C → users                       ✗
Client D → messages / INSERT          ✓
```

The matcher should be optimized for high connection counts.

Avoid scanning every subscription for every database event.

Possible indexing strategy:

```text
schema
  └── table
       └── operation
            └── subscriptions
```

Filtering is then performed only against relevant subscriptions.

---

# 5.10 Authorization Engine

Authentication determines:

> Who is this client?

Authorization determines:

> Is this client allowed to receive this event?

Authorization must happen independently of subscription matching.

Example:

```text
User 10
    │
    ▼
Subscribe to messages/room=123
    │
    ▼
Authorization Engine
    │
    ├── allowed → subscription created
    └── denied  → subscription rejected
```

Authorization should support application-defined policies.

The service should not assume that possession of a valid JWT automatically grants access to every table.

---

# 5.11 Connection Manager

Maintains all active client connections.

Responsibilities:

- Accept WebSocket connections.
- Assign connection IDs.
- Authenticate clients.
- Track subscriptions.
- Send events.
- Handle disconnects.
- Handle ping/pong.
- Detect stale connections.
- Enforce connection limits.
- Apply per-client backpressure.

Example:

```text
Connection
├── ID
├── User
├── Authentication Context
├── Created At
├── Last Activity
├── Subscriptions
├── Outbound Queue
└── WebSocket
```

---

# 5.12 Outbound Event Queue

Every connection should have an independent outbound queue.

```text
Event Dispatcher
       │
       ▼
Connection A → Queue → WebSocket
Connection B → Queue → WebSocket
Connection C → Queue → WebSocket
```

A slow client must not block other clients.

The system should define a maximum queue size.

Example policy:

```text
queue < limit
    → continue

queue >= limit
    → disconnect slow client
```

Do not allow unbounded memory growth.

---

# 5.13 WebSocket Server

The WebSocket layer provides the public realtime protocol.

Example:

```text
GET /realtime
```

Connection lifecycle:

```text
CONNECT
   │
   ▼
AUTHENTICATE
   │
   ▼
SUBSCRIBE
   │
   ▼
RECEIVE EVENTS
   │
   ▼
UNSUBSCRIBE
   │
   ▼
DISCONNECT
```

---

# 6. Client Protocol

The initial protocol should be deliberately simple.

### Connect

```text
WebSocket /realtime
```

### Authenticate

```json
{
  "type": "auth",
  "token": "JWT"
}
```

### Subscribe

```json
{
  "type": "subscribe",
  "id": "sub_123",
  "schema": "public",
  "table": "messages",
  "event": "INSERT",
  "filter": {
    "room_id": 123
  }
}
```

### Subscription acknowledgement

```json
{
  "type": "subscribed",
  "id": "sub_123"
}
```

### Database event

```json
{
  "type": "event",
  "subscription_id": "sub_123",
  "event": "INSERT",
  "schema": "public",
  "table": "messages",
  "record": {
    "id": 100,
    "room_id": 123,
    "content": "Hello"
  }
}
```

### Unsubscribe

```json
{
  "type": "unsubscribe",
  "id": "sub_123"
}
```

---

# 7. Event Ordering

Ordering must be explicitly defined.

The system should guarantee:

### Within a transaction

Events are emitted in their PostgreSQL logical replication order.

### Across transactions

Events are emitted according to PostgreSQL commit order as observed by logical replication.

### Across WebSocket connections

No global ordering guarantee is required.

Therefore:

```text
Client A
transaction 1 → transaction 2 → transaction 3
```

is ordered.

But:

```text
Client A event
Client B event
```

does not imply a globally synchronized delivery time.

---

# 8. LSN Management

LSN is fundamental to reliability.

The system should track:

```text
received_lsn
flushed_lsn
committed_lsn
```

The replication client should periodically send PostgreSQL feedback indicating the latest safely processed LSN.

A simplified lifecycle:

```text
Receive WAL
    │
    ▼
Decode
    │
    ▼
Process transaction
    │
    ▼
Dispatch events
    │
    ▼
Mark processed
    │
    ▼
Acknowledge LSN
```

The system must not acknowledge an LSN before the corresponding transaction has been successfully processed.

---

# 9. Failure Recovery

## Realtime server crashes

The replication slot retains WAL.

After restart:

```text
PostgreSQL
    │
    ▼
Replication Slot
    │
    ▼
Last acknowledged LSN
    │
    ▼
Realtime Service
```

The service resumes processing.

---

## WebSocket server crashes

Clients reconnect.

They must establish their subscriptions again.

The system should provide a mechanism for clients to recover state if necessary.

---

## PostgreSQL disconnects

The replication reader should:

1. Detect connection failure.
2. Close the replication connection.
3. Reconnect using exponential backoff.
4. Resume from the last acknowledged LSN.
5. Continue processing.

---

# 10. Backpressure

Backpressure must exist at multiple levels:

```text
PostgreSQL
    │
    ▼
Replication Reader
    │
    ▼
Event Pipeline
    │
    ▼
Subscription Matcher
    │
    ▼
Client Queues
```

The replication reader must not be allowed to stall indefinitely because one client is slow.

A slow client should eventually be disconnected.

Metrics should expose:

```text
active_connections
slow_connections
queued_events
dropped_connections
```

---

# 11. Horizontal Scaling

A single replication slot should initially have **one active WAL consumer**.

Multiple realtime servers cannot simply consume the same logical replication stream independently.

Therefore the initial architecture should be:

```text
                   PostgreSQL
                       │
                       ▼
                Replication Reader
                       │
                       ▼
                 Event Broker
                 /     |      \
                /      |       \
               ▼       ▼        ▼
             Node A  Node B   Node C
               │       │        │
             clients clients  clients
```

The replication reader becomes the source of realtime events.

A broker such as Redis Streams, NATS, Kafka, or another durable transport can distribute events among realtime nodes.

However, this should **not be introduced in the first version unless scaling requires it**.

Start with:

```text
PostgreSQL
    ↓
One Realtime instance
    ↓
Many WebSockets
```

Then introduce distributed fanout.

---

# 12. Security

## Authentication

Support:

- JWT
- API keys
- Service credentials

JWT should expose an application-defined identity.

Example:

```json
{
  "sub": "user_123",
  "role": "authenticated"
}
```

## Authorization

Authorization must happen:

1. During connection authentication.
2. During subscription creation.
3. Where necessary during event delivery.

Do not trust client-supplied:

```text
user_id
tenant_id
role
```

unless they come from a verified authentication context.

---

# 13. Multi-Tenancy

The realtime service should support tenant isolation.

Example:

```text
Tenant A
 ├── users
 ├── messages
 └── orders

Tenant B
 ├── users
 ├── messages
 └── orders
```

A client from Tenant A must never receive Tenant B events.

Tenant identity should come from authenticated server-side context rather than arbitrary subscription parameters.

---

# 14. PostgreSQL Requirements

The service requires logical replication to be enabled.

Typical configuration:

```conf
wal_level = logical
```

Replication connections must also be permitted.

The service requires an appropriate PostgreSQL role with replication privileges.

The database should have:

```sql
CREATE PUBLICATION realtime ...
```

and:

```text
logical replication slot
```

Replica identity must also be considered for updates/deletes.

For tables where old row values are required, configure appropriate replica identity.

---

# 15. Observability

The service must expose metrics for:

### PostgreSQL

```text
replication_lag_bytes
replication_lag_seconds
current_lsn
last_acknowledged_lsn
slot_wal_retained
```

### Replication

```text
replication_connected
wal_messages_received
transactions_received
changes_received
transactions_processed
```

### WebSocket

```text
active_connections
connections_total
connections_rejected
subscriptions_active
events_sent
events_failed
slow_clients
```

### System

```text
memory_usage
goroutines
CPU usage
event_queue_size
```

---

# 16. Health Endpoints

The service should provide:

```text
GET /health
```

Basic process health.

```text
GET /ready
```

Readiness should verify:

- PostgreSQL connectivity.
- Replication connection.
- Replication slot availability.
- Event processing health.

```text
GET /metrics
```

Prometheus-compatible metrics.

---

# 17. Internal Component Roles

The implementation should maintain clear separation of responsibilities.

| Component              | Responsibility                                    |
| ---------------------- | ------------------------------------------------- |
| Publication Manager    | Manage/discover PostgreSQL publications           |
| Replication Manager    | Manage replication slot and replication lifecycle |
| Replication Reader     | Consume PostgreSQL logical replication            |
| Transaction Decoder    | Decode PostgreSQL replication messages            |
| Change Normalizer      | Convert DB changes to internal events             |
| Event Dispatcher       | Route committed events                            |
| Subscription Manager   | Maintain client subscriptions                     |
| Subscription Matcher   | Determine which subscriptions match events        |
| Authorization Engine   | Determine whether clients may receive events      |
| Connection Manager     | Maintain WebSocket connections                    |
| Outbound Queue         | Buffer events per client                          |
| Protocol Handler       | Encode/decode WebSocket messages                  |
| Authentication Manager | Validate client credentials                       |
| LSN Manager            | Track processing/acknowledgement position         |
| Health Manager         | Determine service health/readiness                |
| Metrics Manager        | Expose operational metrics                        |
| Configuration Manager  | Manage service configuration                      |

---

# 18. Suggested Go Package Structure

```text
realtime/
├── cmd/
│   └── realtime/
│       └── main.go
│
├── internal/
│   ├── replication/
│   │   ├── reader.go
│   │   ├── slot.go
│   │   ├── publication.go
│   │   └── decoder.go
│   │
│   ├── changes/
│   │   ├── event.go
│   │   ├── normalizer.go
│   │   └── transaction.go
│   │
│   ├── subscriptions/
│   │   ├── subscription.go
│   │   ├── manager.go
│   │   └── matcher.go
│   │
│   ├── auth/
│   │   ├── authenticator.go
│   │   └── authorizer.go
│   │
│   ├── connections/
│   │   ├── connection.go
│   │   ├── manager.go
│   │   └── queue.go
│   │
│   ├── websocket/
│   │   ├── handler.go
│   │   └── protocol.go
│   │
│   ├── lsn/
│   │   └── manager.go
│   │
│   ├── health/
│   │   └── health.go
│   │
│   └── metrics/
│       └── metrics.go
│
├── pkg/
│   └── protocol/
│       └── events.go
│
└── migrations/
```

---

# 19. Event Lifecycle

A complete database change should follow:

```text
1. Application executes SQL
             │
             ▼
2. PostgreSQL commits transaction
             │
             ▼
3. WAL contains transaction
             │
             ▼
4. Logical replication exposes transaction
             │
             ▼
5. Replication Reader receives messages
             │
             ▼
6. Transaction Decoder reconstructs changes
             │
             ▼
7. Commit received
             │
             ▼
8. Change Normalizer creates events
             │
             ▼
9. Authorization/Subscription Matcher
             │
             ▼
10. Matching connections receive events
             │
             ▼
11. Event successfully queued
             │
             ▼
12. Replication position acknowledged
```

The critical rule is:

> **Never acknowledge PostgreSQL progress before the realtime system has safely processed the corresponding transaction.**

---

# 20. Initial MVP

The first implementation should deliberately be smaller.

### Phase 1

Implement:

- PostgreSQL logical replication connection.
- `pgoutput` decoding.
- One replication slot.
- One publication.
- `INSERT`.
- `UPDATE`.
- `DELETE`.
- Transaction handling.
- LSN tracking.
- WebSocket server.
- Authentication.
- Basic subscriptions.
- Schema/table filtering.
- Per-client queues.
- Automatic reconnect to PostgreSQL.
- Basic metrics.

Architecture:

```text
PostgreSQL
    │
    ▼
Logical Replication
    │
    ▼
Go Replication Reader
    │
    ▼
Decoder
    │
    ▼
Subscription Matcher
    │
    ▼
WebSocket Manager
    │
    ▼
Clients
```

Do **not** add Redis/Kafka/NATS yet.

---

# 21. Phase 2

Add:

- Row-level filters.
- Authorization policies.
- Initial-state synchronization.
- Improved reconnect semantics.
- Replica identity handling.
- TRUNCATE.
- Better transaction metadata.
- Rate limiting.
- Connection limits.
- Advanced metrics.
- Administrative API.

---

# 22. Phase 3

Add distributed operation:

```text
PostgreSQL
    │
    ▼
Replication Consumer
    │
    ▼
Durable Event Bus
    │
    ├───────────────┐
    ▼               ▼
Realtime Node A   Realtime Node B
    │               │
    ▼               ▼
Clients           Clients
```

Potential event buses:

- NATS JetStream
- Redis Streams
- Kafka

The choice should be based on required durability, throughput, operational complexity, and existing infrastructure.

---

# 23. Important Design Principle

The system should maintain a strict separation between:

```text
Database Change
```

and:

```text
Application Realtime Event
```

The database layer answers:

> What changed?

The realtime layer answers:

> Who should know about it?

The authorization layer answers:

> Is this client allowed to know?

The WebSocket layer answers:

> How do we deliver it?

This separation will make the service significantly easier to evolve.

---

# 24. Target Architecture

The final architecture should be capable of evolving from:

```text
              PostgreSQL
                   │
                   ▼
             WAL / Logical
             Replication
                   │
                   ▼
             Realtime Node
                   │
             ┌─────┴─────┐
             ▼           ▼
         WebSocket    WebSocket
             │           │
             ▼           ▼
          Clients      Clients
```

into:

```text
                    PostgreSQL
                        │
                        ▼
                 Logical Replication
                        │
                        ▼
                 Replication Engine
                        │
                        ▼
                  Durable Event Bus
                        │
             ┌──────────┼──────────┐
             ▼          ▼          ▼
          Node A     Node B      Node C
             │          │          │
          ┌──┴──┐    ┌──┴──┐    ┌──┴──┐
          ▼     ▼    ▼     ▼    ▼     ▼
       Clients Clients Clients Clients Clients
```

The replication layer should remain independent of client connections throughout the evolution.

---

# 25. Success Criteria

The implementation is considered production-ready when it can demonstrate:

1. A PostgreSQL transaction produces the correct realtime event.
2. Multiple subscribers receive only matching events.
3. Unauthorized subscribers cannot receive protected events.
4. Transaction ordering is preserved.
5. PostgreSQL disconnection does not lose events.
6. Realtime service restart resumes from the correct LSN.
7. Slow clients cannot block WAL processing.
8. Replication lag is observable.
9. WAL retention caused by the replication slot is observable.
10. Thousands of WebSocket connections can coexist without affecting replication processing.
11. PostgreSQL remains the authoritative source of state.
12. Client protocol remains independent of PostgreSQL's internal replication protocol.
13. The system can later introduce distributed realtime nodes without redesigning the replication layer.

---

## Core Architectural Principle

The most important design decision is:

```text
                 PostgreSQL
                     │
                     │ durable WAL
                     ▼
              Replication Engine
                     │
                     │ durable position
                     ▼
              Internal Events
                     │
             ┌───────┴────────┐
             ▼                ▼
      Authorization      Subscription
             │                │
             └───────┬────────┘
                     ▼
              Connection Layer
                     │
                     ▼
                  Clients
```

**PostgreSQL WAL is the durable event source.**

**The realtime service is the interpretation and fanout layer.**

**WebSockets are only the delivery mechanism.**

That separation is what allows the system to remain reliable, scalable, and independent of any particular frontend framework.

# ADR-0009: Event Subscription and Delivery

Status: Accepted

## Context

ADR-0003 defines the JSON-RPC 2.0 transport and mentions `module.subscribe` and backend-pushed `module.event` notifications, but Phase 1 left them unimplemented because no module produced events. Phase 2 (system monitoring) needs the backend to push live data to the UI without polling, so the semantics must be fixed before the first producer is written.

Constraints from earlier decisions:

- Modules never hold connections or talk to each other directly. Everything goes through the core (hub-and-spoke, ARCHITECTURE.md).
- A module may only use a capability it declares and the user grants (ADR-0007). Denial fails closed.
- The wire format stays `omnibar.ipc/v1`: newline-delimited JSON-RPC 2.0, 1 MiB frame limit (ADR-0003). New methods are additive.
- The backend must stay light when idle (< 1% CPU, < 20 MB RAM).

## Decision

### Components

- **Event hub** (`internal/events`): an in-process publish/subscribe registry owned by the core. Modules publish through it; the IPC layer subscribes to it on behalf of each client connection.
- **Session**: the IPC server's per-connection state. It holds the connection's subscriptions and a bounded outbound queue, and is the only code that writes to that connection.
- Handlers that need the calling connection receive it through the request `context.Context`. Existing handlers and `ipc.New` / `Dispatcher.Register` call sites keep working unchanged.

### Wire protocol

**Subscribe (request):**

```json
{"jsonrpc":"2.0","id":2,"method":"module.subscribe","params":{"module":"system_monitor","events":["cpu"]}}
```

- `module` is required.
- `events` is optional. When omitted, the client subscribes to every event the module publishes, including events added later.
- Subscribing is additive and idempotent: repeating a subscription is not an error.

**Subscribe (response):**

```json
{"jsonrpc":"2.0","id":2,"result":{"module":"system_monitor","events":["cpu"]}}
```

`events` echoes the active filter for that module on this connection after the call. `["*"]` means all events.

**Unsubscribe (request):** `module.unsubscribe` with the same params. Omitting `events` removes the whole module subscription. Removing something that is not subscribed succeeds. The result has the same shape as subscribe, with the remaining filter (`[]` when nothing remains).

**Event notification (backend → client, no `id`, no response):**

```json
{"jsonrpc":"2.0","method":"module.event","params":{"module":"system_monitor","event":"cpu","data":{"percent":4.2}}}
```

`data` is any JSON value defined by the module. This replaces the earlier illustrative `value` field in ADR-0003, which was never implemented.

### Errors

| Condition | Code | Notes |
|-----------|------|-------|
| Malformed or missing params, empty event name | -32602 | Invalid params |
| Unknown module | -32600 | Same as `module.getStatus`; `module` field set |
| Module declares capabilities that are not all granted | -32001 | "Permission denied"; fail-closed, `module` field set |
| Module disabled or not running | -32002 | "Module not available"; `module` field set |

Codes -32001 and -32002 are in the server-defined range and are assigned here so the UI can distinguish them.

A subscription is checked once, when it is made. Events a module publishes are the module's responsibility to gate with `Registry.HasCapability`; a module that is not allowed to read something must not publish it.

### Delivery guarantees

- **At most once, no replay.** There is no event history and no resend after a disconnect.
- **Per-module ordering.** Events from one module reach a given client in publish order. There is no ordering guarantee across modules.
- **Filtering happens in the backend.** A client only receives events matching its subscription.
- **State after reconnect.** A client that reconnects must re-handshake, re-subscribe, and then fetch current state with a request such as `system.getSnapshot`. Subscribing before fetching avoids a gap in which an update could be missed.
- **Subscriptions end with the connection.** They live in memory only and are not persisted. A backend restart also clears them, which is why the UI re-subscribes on every successful handshake.

### Backpressure and failure isolation

- Each session has a bounded outbound queue (default 64 messages) drained by a single writer goroutine. Responses and notifications share it, so frames can never interleave on the wire.
- `Publish` never blocks and never waits for a client. If a client's queue is full, that client is disconnected and a warning is logged. The client reconnects and resynchronizes as above. Other clients and the publishing module are unaffected.
- An event whose encoded frame would exceed the frame limit is dropped and logged at error level. It does not disconnect anyone.
- A panic in subscriber delivery is recovered at the hub boundary; it must not take down the publishing module.
- Graceful shutdown stops delivery, closes sessions, and exits 0 with no errors logged, even with active subscriptions.

### On-demand production

The hub exposes whether a module currently has subscribers, and notifies the core when that changes from none to some or some to none. Producers that sample periodically (ADR-0010) use this to sample only while someone is listening, which keeps the idle cost near zero.

### Versioning

The protocol stays `omnibar.ipc/v1`. The two methods and the notification are additive, and clients that do not use them are unaffected.

## Alternatives considered

### Clients poll with requests

Simple, no new protocol. Rejected: wasteful at 1-2 s refresh, adds latency, and the backend would have to compute results for clients that are not looking.

### Per-module sockets or a pub/sub broker

Rejected: more moving parts, more sockets to secure, and no benefit for one user on one machine.

### Block the publisher until a client catches up

Rejected: one stuck UI would stall the backend. Dropping the slow client is safe because every client can resynchronize by request.

### Durable event log with replay

Rejected for now: needs storage (SQLite is deliberately deferred, ADR-0006) and no feature requires missed events. Revisit with alerts and history.

## Consequences

**Positive:**

- UI gets live data with no polling.
- A slow or hung client cannot hurt the backend or other clients.
- Capability and module-state rules from Phase 1 apply to subscriptions without a new permission system.
- Idle cost stays low because producers can sleep when nobody is subscribed.

**Negative:**

- A disconnected-for-slowness client sees a reconnect cycle instead of a gap; the UI must handle resynchronization correctly.
- No history: anything that needs missed events must add storage later.
- Handlers that need the connection use the request context, a small extension to the Phase 1 dispatcher.

## References

- ADR-0003: IPC protocol
- ADR-0004: Module execution model
- ADR-0007: Permissions and security
- ADR-0010: System monitoring module

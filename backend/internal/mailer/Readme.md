# mailer

Outbound-email abstraction. Two leaf implementations (`SMTPMailer`,
`LogMailer`) deliver the message; three composable decorators add
cross-cutting concerns without touching the leaves.

## Composition

```
                       Send(ctx, msg)
                              |
                              v
                +-----------------------------+
                |  LoggingMailer (outermost)  |   one breadcrumb per Send
                +-----------------------------+
                              |
                              v
                +-----------------------------+
                |       MetricsMailer         |   one counter obs per Send
                +-----------------------------+
                              |
                              v
                +-----------------------------+
                |      RetryingMailer         |   up to 3 attempts, expo backoff
                +-----------------------------+
                              |
                              v
                +-----------------------------+
                |  SMTPMailer / LogMailer     |   actual delivery (or log)
                +-----------------------------+
```

The wiring lives in `cmd/web/main.go`. Order matters:

- **Retries innermost.** Metrics + logs see a single end-to-end outcome
  per `Send`, not one per attempt. Counting per-attempt would inflate
  the failure rate and obscure the user-visible success rate.
- **Metrics inside logs.** The log entry is the human-readable mirror of
  the metric tick; keeping them adjacent (metrics first, log last) means
  a noisy log line corresponds 1:1 with a counter increment.

## Adding a new decorator

1. New file `internal/mailer/<concern>.go`. Embed `inner Mailer`, expose
   a `New<Concern>` constructor.
2. Implement `Send(ctx, msg) error` — delegate to `inner.Send` and add
   exactly one behaviour around it. If the behaviour requires fan-out
   (alerting, archiving, dual-write to a queue), put each side behind
   its own decorator and compose them.
3. Unit-test with the in-package `fakeMailer` (records calls + returns
   a configured error sequence).
4. Wire it into `cmd/web/main.go` in the correct slot. Update the comment
   block + this diagram.

Reasonable candidates next: `RateLimitedMailer` (per-recipient leaky
bucket), `CircuitBreakerMailer` (open the breaker after N consecutive
SMTP failures), `SamplingMailer` (drop a configurable percentage in
load tests).

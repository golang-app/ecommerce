# Dependency

The dependency represents anything that's outside of the main binary and the application relays on.
Every dependency has to implement the following interface:

```go
type Dependency interface {
	Healthy(context.Context) bool
	Ready(context.Context) bool
	Close() error
}
```

`Healthy(context.Context) bool` method is used in Liveness Probe.

`Ready(context.Context) bool` method is used in Readiness Probe.


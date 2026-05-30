// Package integration holds tests that exercise the boundaries BETWEEN
// bounded contexts in this codebase: the event-driven subscribers, the
// outbox dispatcher, the inbox dedupe, and the cross-context wiring done
// at the composition root.
//
// Each context has its own unit tests inside its own directory; those
// verify the context's domain rules in isolation. The tests in this
// package verify what happens when contexts TALK to each other.
//
// If a test here breaks, the most likely cause is:
//   - someone changed the shape of an integration event
//   - someone added / removed / renamed a subscriber
//   - someone changed the at-least-once delivery contract
package integration_test

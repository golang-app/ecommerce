package app

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"time"

	"github.com/bkielbasa/go-ecommerce/backend/auth/adapter"
	"github.com/bkielbasa/go-ecommerce/backend/auth/domain"
	"github.com/bkielbasa/go-ecommerce/backend/internal/observability"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/crypto/bcrypt"
)

// tracer is the package-level tracer used by the auth application layer.
// Bound to the global TracerProvider; resolves to a no-op when no exporter
// is configured so the calls cost effectively nothing.
var tracer = observability.Tracer("github.com/bkielbasa/go-ecommerce/backend/auth/app")

var (
	ErrInvalidEmail  = fmt.Errorf("invalid email")
	ErrWrongPassword = fmt.Errorf("current password is incorrect")
)

// passwordResetTTL is the lifetime of a freshly-minted reset token. Thirty
// minutes is the OWASP-recommended ceiling for self-service password-reset
// links — long enough that the email arriving a few minutes late still works,
// short enough that a token leaked into history/logs is useless within an
// hour.
const passwordResetTTL = 30 * time.Minute

var emailRegexp = regexp.MustCompile("^[a-zA-Z0-9.!#$%&'*+/=?^_`{|}~-]+@[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?(?:\\.[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?)*$")

// CustomerStorage is the persistence boundary for the Customer aggregate.
// As of the customer/admin split (migration 000038) it no longer carries
// the IsAdmin / MustChangePassword surface — those moved to AdminStorage.
type CustomerStorage interface {
	Create(ctx context.Context, email, passwordHash string) error
	Find(ctx context.Context, email string) (adapter.Customer, error)
	UpdatePassword(ctx context.Context, email, passwordHash string) error
}

type SessStorage interface {
	Store(ctx context.Context, sess *domain.Session) error
	Find(ctx context.Context, token string) (*domain.Session, error)
}

// PasswordResetStorage is the persistence boundary for the password-reset
// flow. Begin mints a raw token (and stores only its hash); Consume validates
// and atomically burns it. Implementations live in auth/adapter alongside the
// customer/session stores.
type PasswordResetStorage interface {
	BeginPasswordReset(ctx context.Context, customerID string, ttl time.Duration) (string, error)
	ConsumePasswordReset(ctx context.Context, rawToken string) (string, error)
}

type auth struct {
	authStorage  CustomerStorage
	sessStorage  SessStorage
	resetStorage PasswordResetStorage
	passPolicies []domain.PasswordPolicy
}

// NewAuth wires the auth application service. The PasswordResetStorage is
// optional — pass nil from call sites that don't need the forgot-password
// flow yet and the service degrades gracefully (RequestPasswordReset returns
// nil silently, ResetPassword returns ErrInvalidResetToken).
func NewAuth(authStorage CustomerStorage, sessStorage SessStorage, resetStorage ...PasswordResetStorage) auth {
	var rs PasswordResetStorage
	if len(resetStorage) > 0 {
		rs = resetStorage[0]
	}
	return auth{
		authStorage: authStorage, sessStorage: sessStorage, resetStorage: rs,
		passPolicies: defaultPasswordPolicies(),
	}
}

// defaultPasswordPolicies returns the OWASP-aligned policy bundle that the
// customer and admin services both enforce. Centralising the list here
// keeps the two services in lockstep — there is no reason for admins to
// face a different password contract than customers.
//
// Source: https://cheatsheetseries.owasp.org/cheatsheets/Authentication_Cheat_Sheet.html#implement-proper-password-strength-controls
func defaultPasswordPolicies() []domain.PasswordPolicy {
	return []domain.PasswordPolicy{
		domain.MinLength(8),
		domain.MaxLength(64),
		domain.MustContainLowercase,
		domain.MustContainUppercase,
		domain.MustContainNumber,
		domain.MustContainSpecialChar,
	}
}

func (a auth) CreateNewCustomer(ctx context.Context, email, password string) (rerr error) {
	ctx, span := tracer.Start(ctx, "Auth.CreateNewCustomer")
	defer span.End()
	// Defer the metrics decision until the function actually exits so every
	// early return funnels through one place; named return rerr carries the
	// outcome.
	defer func() {
		outcome := "success"
		if rerr != nil {
			outcome = "failure"
		}
		observability.RegistrationsInc(ctx, outcome)
	}()

	if !emailRegexp.MatchString(email) {
		recordSpanError(span, ErrInvalidEmail)
		return ErrInvalidEmail
	}

	for _, policy := range a.passPolicies {
		if err := policy(password); err != nil {
			recordSpanError(span, err)
			return fmt.Errorf("cannot create customer: %w", err)
		}
	}

	passwordHash, err := hashPassword(password)
	if err != nil {
		recordSpanError(span, err)
		return fmt.Errorf("could not hash password: %w", err)
	}

	_, err = a.authStorage.Find(ctx, email)

	switch err {
	case domain.ErrCustomerNotFound:
	// we didn't find a customer with the same email
	// so we can create a new one

	case nil:
		recordSpanError(span, domain.ErrCustomerExists)
		return domain.ErrCustomerExists
	default:
		recordSpanError(span, err)
		return fmt.Errorf("could not check if customer already exists: %w", err)
	}

	if err := a.authStorage.Create(ctx, email, passwordHash); err != nil {
		recordSpanError(span, err)
		return err
	}
	return nil
}

func (a auth) FindByToken(ctx context.Context, sessToken string) (*domain.Session, error) {
	sess, err := a.sessStorage.Find(ctx, sessToken)
	if err != nil {
		return nil, fmt.Errorf("could not find session (%s): %w", sessToken, err)
	}

	return sess, nil
}

func (a auth) Logout(ctx context.Context, sessToken string) error {
	sess, err := a.sessStorage.Find(ctx, sessToken)
	if err != nil {
		return fmt.Errorf("could not find session (%s): %w", sessToken, err)
	}

	sess.Invalidate()

	if err := a.sessStorage.Store(ctx, sess); err != nil {
		return fmt.Errorf("could not store session: %w", err)
	}

	return nil
}

func (a auth) Login(ctx context.Context, email, password string) (_ *domain.Session, rerr error) {
	ctx, span := tracer.Start(ctx, "Auth.Login")
	defer span.End()
	// Login outcome is recorded once at function exit so the metric stays
	// consistent regardless of which guard fires (customer lookup, bcrypt
	// compare, session persist).
	defer func() {
		outcome := "success"
		if rerr != nil {
			outcome = "failure"
		}
		observability.LoginsInc(ctx, outcome)
	}()

	customer, err := a.authStorage.Find(ctx, email)
	if err != nil {
		recordSpanError(span, err)
		return nil, fmt.Errorf("could not find customer: %w", err)
	}

	if err := bcrypt.CompareHashAndPassword([]byte(customer.PasswordHash), []byte(password)); err != nil {
		recordSpanError(span, err)
		return nil, fmt.Errorf("could not compare password hash: %w", err)
	}

	c := domain.NewCustomer(customer.Username)

	sessID := domain.NewSessionID()

	// TODO: make session expiration configurable
	expires := time.Now().Add(7 * 24 * time.Hour)
	sess := domain.NewSession(sessID, c.Email(), expires)

	if err := a.sessStorage.Store(ctx, sess); err != nil {
		recordSpanError(span, err)
		return nil, fmt.Errorf("could not store session: %w", err)
	}

	return sess, nil
}

// ChangePassword verifies the customer's current password, enforces the
// password policy on the new one, and stores the new hash.
//
// Customers no longer carry a must_change_password flag — that gate
// moved to the Admin aggregate as part of the 000038 split — so this
// method has no "clear the flag" side effect today. The shape is kept
// (rather than reduced to UpdatePassword) so storefront and admin
// services stay symmetric.
func (a auth) ChangePassword(ctx context.Context, email, oldPassword, newPassword string) error {
	_, span := tracer.Start(ctx, "Auth.ChangePassword")
	defer span.End()

	customer, err := a.authStorage.Find(ctx, email)
	if err != nil {
		recordSpanError(span, err)
		return fmt.Errorf("could not find customer: %w", err)
	}

	if err := bcrypt.CompareHashAndPassword([]byte(customer.PasswordHash), []byte(oldPassword)); err != nil {
		recordSpanError(span, ErrWrongPassword)
		return ErrWrongPassword
	}

	for _, policy := range a.passPolicies {
		if err := policy(newPassword); err != nil {
			recordSpanError(span, err)
			return fmt.Errorf("cannot change password: %w", err)
		}
	}

	newHash, err := hashPassword(newPassword)
	if err != nil {
		recordSpanError(span, err)
		return fmt.Errorf("could not hash password: %w", err)
	}

	if err := a.authStorage.UpdatePassword(ctx, email, newHash); err != nil {
		recordSpanError(span, err)
		return fmt.Errorf("could not update password: %w", err)
	}

	return nil
}

func hashPassword(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(bytes), err
}

// RequestPasswordReset mints a fresh password-reset token for the customer
// identified by email. The returned token is the RAW value the caller must
// embed in the reset link sent over email; the storage layer only ever
// persists its sha256 hash.
//
// To prevent account-enumeration, RequestPasswordReset returns ("", nil) when
// no customer matches — the caller renders the same "if an account exists,
// an email is on its way" confirmation regardless. Any OTHER lookup error is
// returned verbatim so a transient DB outage still produces a 5xx rather
// than a silent success that leaks no information.
func (a auth) RequestPasswordReset(ctx context.Context, email string) (string, error) {
	ctx, span := tracer.Start(ctx, "Auth.RequestPasswordReset")
	defer span.End()

	if a.resetStorage == nil {
		return "", nil
	}
	if !emailRegexp.MatchString(email) {
		// Treat malformed input as a non-existent account — same
		// enumeration-prevention rationale as the not-found path.
		return "", nil
	}
	_, err := a.authStorage.Find(ctx, email)
	if errors.Is(err, domain.ErrCustomerNotFound) {
		return "", nil
	}
	if err != nil {
		recordSpanError(span, err)
		return "", fmt.Errorf("could not lookup customer: %w", err)
	}
	token, err := a.resetStorage.BeginPasswordReset(ctx, email, passwordResetTTL)
	if err != nil {
		recordSpanError(span, err)
		return "", err
	}
	return token, nil
}

// ResetPassword burns a reset token and replaces the customer's password
// with newPassword (after enforcing the same policy as ChangePassword).
// The token is consumed BEFORE the policy check on purpose: a policy
// failure should not let a leaked token stay reusable.
func (a auth) ResetPassword(ctx context.Context, rawToken, newPassword string) error {
	_, span := tracer.Start(ctx, "Auth.ResetPassword")
	defer span.End()

	if a.resetStorage == nil {
		recordSpanError(span, adapter.ErrInvalidResetToken)
		return adapter.ErrInvalidResetToken
	}
	customerID, err := a.resetStorage.ConsumePasswordReset(ctx, rawToken)
	if err != nil {
		recordSpanError(span, err)
		return err
	}

	for _, policy := range a.passPolicies {
		if err := policy(newPassword); err != nil {
			recordSpanError(span, err)
			return fmt.Errorf("cannot reset password: %w", err)
		}
	}

	newHash, err := hashPassword(newPassword)
	if err != nil {
		recordSpanError(span, err)
		return fmt.Errorf("could not hash password: %w", err)
	}
	if err := a.authStorage.UpdatePassword(ctx, customerID, newHash); err != nil {
		recordSpanError(span, err)
		return fmt.Errorf("could not update password: %w", err)
	}
	return nil
}

// recordSpanError marks the span as errored and records the underlying error
// so traces clearly identify failure paths without callers having to wire the
// two operations themselves.
func recordSpanError(span trace.Span, err error) {
	span.RecordError(err)
	span.SetStatus(codes.Error, err.Error())
}

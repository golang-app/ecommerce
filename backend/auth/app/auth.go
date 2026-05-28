package app

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"time"

	"github.com/bkielbasa/go-ecommerce/backend/auth/adapter"
	"github.com/bkielbasa/go-ecommerce/backend/auth/domain"
	"golang.org/x/crypto/bcrypt"
)

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

type CustomerStorage interface {
	Create(ctx context.Context, email, passwordHash string) error
	Find(ctx context.Context, email string) (adapter.Customer, error)
	UpdatePassword(ctx context.Context, email, passwordHash string) error
	// ClearMustChangePassword resets the must_change_password flag for the
	// given customer. Called by ChangePassword after a successful update so
	// the change-password gate stops firing on subsequent logins.
	ClearMustChangePassword(ctx context.Context, email string) error
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
		passPolicies: []domain.PasswordPolicy{

			// those values are recommended by OWASP https://cheatsheetseries.owasp.org/cheatsheets/Authentication_Cheat_Sheet.html#implement-proper-password-strength-controls
			domain.MinLength(8),
			domain.MaxLength(64),

			domain.MustContainLowercase,
			domain.MustContainUppercase,
			domain.MustContainNumber,
			domain.MustContainSpecialChar,
		},
	}
}

func (a auth) CreateNewCustomer(ctx context.Context, email, password string) error {
	if !emailRegexp.MatchString(email) {
		return ErrInvalidEmail
	}

	for _, policy := range a.passPolicies {
		if err := policy(password); err != nil {
			return fmt.Errorf("cannot create customer: %w", err)
		}
	}

	passwordHash, err := hashPassword(password)
	if err != nil {
		return fmt.Errorf("could not hash password: %w", err)
	}

	_, err = a.authStorage.Find(ctx, email)

	switch err {
	case domain.ErrCustomerNotFound:
	// we didn't find a customer with the same email
	// so we can create a new one

	case nil:
		return domain.ErrCustomerExists
	default:
		return fmt.Errorf("could not check if customer already exists: %w", err)
	}

	return a.authStorage.Create(ctx, email, passwordHash)
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

func (a auth) Login(ctx context.Context, email, password string) (*domain.Session, error) {
	customer, err := a.authStorage.Find(ctx, email)
	if err != nil {
		return nil, fmt.Errorf("could not find customer: %w", err)
	}

	if err := bcrypt.CompareHashAndPassword([]byte(customer.PasswordHash), []byte(password)); err != nil {
		return nil, fmt.Errorf("could not compare password hash: %w", err)
	}

	c := domain.NewCustomer(customer.Username)

	sessID := domain.NewSessionID()

	// TODO: make session expiration configurable
	expires := time.Now().Add(7 * 24 * time.Hour)
	sess := domain.NewSession(sessID, c.Email(), expires)

	if err := a.sessStorage.Store(ctx, sess); err != nil {
		return nil, fmt.Errorf("could not store session: %w", err)
	}

	return sess, nil
}

// ChangePassword verifies the customer's current password, enforces the
// password policy on the new one, and stores the new hash.
func (a auth) ChangePassword(ctx context.Context, email, oldPassword, newPassword string) error {
	customer, err := a.authStorage.Find(ctx, email)
	if err != nil {
		return fmt.Errorf("could not find customer: %w", err)
	}

	if err := bcrypt.CompareHashAndPassword([]byte(customer.PasswordHash), []byte(oldPassword)); err != nil {
		return ErrWrongPassword
	}

	for _, policy := range a.passPolicies {
		if err := policy(newPassword); err != nil {
			return fmt.Errorf("cannot change password: %w", err)
		}
	}

	newHash, err := hashPassword(newPassword)
	if err != nil {
		return fmt.Errorf("could not hash password: %w", err)
	}

	if err := a.authStorage.UpdatePassword(ctx, email, newHash); err != nil {
		return fmt.Errorf("could not update password: %w", err)
	}

	// A successful password change always clears the must-change-password
	// gate (no-op for users who weren't flagged in the first place).
	if err := a.authStorage.ClearMustChangePassword(ctx, email); err != nil {
		return fmt.Errorf("could not clear must-change-password flag: %w", err)
	}

	return nil
}

// IsAdmin reports whether the customer identified by email has the admin
// flag set. A missing customer is treated as a non-admin (false, nil); any
// other lookup error is returned.
func (a auth) IsAdmin(ctx context.Context, email string) (bool, error) {
	customer, err := a.authStorage.Find(ctx, email)
	if err == domain.ErrCustomerNotFound {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("could not find customer: %w", err)
	}

	return customer.IsAdmin, nil
}

// MustChangePassword reports whether the given customer is currently flagged
// for a forced password change. A missing customer is treated as false so
// anonymous lookups don't trip the gate; any other lookup error is returned.
func (a auth) MustChangePassword(ctx context.Context, email string) (bool, error) {
	customer, err := a.authStorage.Find(ctx, email)
	if err == domain.ErrCustomerNotFound {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("could not find customer: %w", err)
	}

	return customer.MustChangePassword, nil
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
		return "", fmt.Errorf("could not lookup customer: %w", err)
	}
	return a.resetStorage.BeginPasswordReset(ctx, email, passwordResetTTL)
}

// ResetPassword burns a reset token and replaces the customer's password
// with newPassword (after enforcing the same policy as ChangePassword).
// The token is consumed BEFORE the policy check on purpose: a policy
// failure should not let a leaked token stay reusable.
func (a auth) ResetPassword(ctx context.Context, rawToken, newPassword string) error {
	if a.resetStorage == nil {
		return adapter.ErrInvalidResetToken
	}
	customerID, err := a.resetStorage.ConsumePasswordReset(ctx, rawToken)
	if err != nil {
		return err
	}

	for _, policy := range a.passPolicies {
		if err := policy(newPassword); err != nil {
			return fmt.Errorf("cannot reset password: %w", err)
		}
	}

	newHash, err := hashPassword(newPassword)
	if err != nil {
		return fmt.Errorf("could not hash password: %w", err)
	}
	if err := a.authStorage.UpdatePassword(ctx, customerID, newHash); err != nil {
		return fmt.Errorf("could not update password: %w", err)
	}
	// A successful password reset clears any forced-change flag the same
	// way ChangePassword does — the user just proved they own the inbox.
	if err := a.authStorage.ClearMustChangePassword(ctx, customerID); err != nil {
		return fmt.Errorf("could not clear must-change-password flag: %w", err)
	}
	return nil
}

package app

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/bkielbasa/go-ecommerce/backend/auth/adapter"
	"github.com/bkielbasa/go-ecommerce/backend/auth/domain"
	"golang.org/x/crypto/bcrypt"
)

// AdminStorage is the persistence boundary for the Admin aggregate.
// Mirrors CustomerStorage but lives against the `auth_admin` table and
// includes the must-change-password gate (which is admin-only as of
// the 000038 split).
type AdminStorage interface {
	Upsert(ctx context.Context, id, email, passwordHash, role string, mustChangePassword bool) error
	FindByEmail(ctx context.Context, email string) (adapter.Admin, error)
	FindByID(ctx context.Context, id string) (adapter.Admin, error)
	UpdatePassword(ctx context.Context, email, passwordHash string) error
	ClearMustChangePassword(ctx context.Context, email string) error
}

// AdminRole is the default role string stamped on every seeded admin.
// The role column exists for forward-compat — today the panel does not
// distinguish further, but the column makes it cheap to add reviewer /
// fulfillment / etc. later without another migration.
const AdminRole = "admin"

// adminSessionTTL is how long a fresh admin login stays valid. Kept
// in lock-step with the customer side (7 days) so the two services
// have a single mental model for "session lifetime".
const adminSessionTTL = 7 * 24 * time.Hour

// AdminAuth is the application service for the Admin aggregate. It is
// deliberately a SEPARATE service from auth (customer): the commands
// look similar but the gates, lifecycles, and password-reset surface
// differ. Notably AdminAuth has NO RequestPasswordReset / ResetPassword
// today — admins are provisioned, and a forgot-by-email flow for
// operators is a follow-up.
type AdminAuth struct {
	adminStorage AdminStorage
	sessStorage  SessStorage
	passPolicies []domain.PasswordPolicy
}

func NewAdminAuth(adminStorage AdminStorage, sessStorage SessStorage) AdminAuth {
	return AdminAuth{
		adminStorage: adminStorage,
		sessStorage:  sessStorage,
		passPolicies: defaultPasswordPolicies(),
	}
}

// CreateAdmin idempotently provisions an admin account with
// must_change_password set true so the seeded operator is forced
// through the change-password gate on first login. Used by the cli
// seeds and by future provisioning UIs.
func (a AdminAuth) CreateAdmin(ctx context.Context, email, password string) error {
	ctx, span := tracer.Start(ctx, "AdminAuth.CreateAdmin")
	defer span.End()

	if !emailRegexp.MatchString(email) {
		recordSpanError(span, ErrInvalidEmail)
		return ErrInvalidEmail
	}
	passwordHash, err := hashPassword(password)
	if err != nil {
		recordSpanError(span, err)
		return fmt.Errorf("could not hash password: %w", err)
	}
	// id := email today. The column is plain text so a later
	// migration can swap in a synthetic id without changing the
	// service surface.
	if err := a.adminStorage.Upsert(ctx, email, email, passwordHash, AdminRole, true); err != nil {
		recordSpanError(span, err)
		return fmt.Errorf("could not upsert admin: %w", err)
	}
	return nil
}

// Login verifies the admin's credentials against the auth_admin row and
// mints an admin-scoped session. The session is stamped principal_kind='admin'
// at the storage layer so a customer token can never resolve here and vice
// versa.
func (a AdminAuth) Login(ctx context.Context, email, password string) (*domain.Session, error) {
	ctx, span := tracer.Start(ctx, "AdminAuth.Login")
	defer span.End()

	admin, err := a.adminStorage.FindByEmail(ctx, email)
	if err != nil {
		recordSpanError(span, err)
		return nil, fmt.Errorf("could not find admin: %w", err)
	}

	if err := bcrypt.CompareHashAndPassword([]byte(admin.PasswordHash), []byte(password)); err != nil {
		recordSpanError(span, err)
		return nil, fmt.Errorf("could not compare password hash: %w", err)
	}

	sessID := domain.NewSessionID()
	expires := time.Now().Add(adminSessionTTL)
	// The session's CustomerID() slot carries the admin id; the
	// principal_kind discriminator on the row distinguishes whose
	// id it is. Re-using the column keeps the session schema flat
	// while the two services still have separate cookies + scoped
	// storages so the strings never cross.
	sess := domain.NewSession(sessID, admin.ID, expires)

	if err := a.sessStorage.Store(ctx, sess); err != nil {
		recordSpanError(span, err)
		return nil, fmt.Errorf("could not store session: %w", err)
	}

	return sess, nil
}

// FindByToken resolves an admin-scoped session. The session storage this
// service was wired with is admin-scoped (principal_kind='admin'), so
// the call returns ErrSessionNotFound for any customer token.
func (a AdminAuth) FindByToken(ctx context.Context, token string) (*domain.Session, error) {
	sess, err := a.sessStorage.Find(ctx, token)
	if err != nil {
		return nil, fmt.Errorf("could not find admin session (%s): %w", token, err)
	}
	return sess, nil
}

// Logout invalidates the admin session in place. The cookie removal
// happens at the HTTP layer; this just expires the row so a stolen
// token cannot resurrect the session.
func (a AdminAuth) Logout(ctx context.Context, token string) error {
	sess, err := a.sessStorage.Find(ctx, token)
	if err != nil {
		return fmt.Errorf("could not find admin session (%s): %w", token, err)
	}
	sess.Invalidate()
	if err := a.sessStorage.Store(ctx, sess); err != nil {
		return fmt.Errorf("could not store admin session: %w", err)
	}
	return nil
}

// ChangePassword verifies the admin's current password, enforces the
// password policy on the new one, persists the new hash, and clears
// the must_change_password gate. Mirrors the customer ChangePassword
// shape but lives against auth_admin.
func (a AdminAuth) ChangePassword(ctx context.Context, email, oldPassword, newPassword string) error {
	ctx, span := tracer.Start(ctx, "AdminAuth.ChangePassword")
	defer span.End()

	admin, err := a.adminStorage.FindByEmail(ctx, email)
	if err != nil {
		recordSpanError(span, err)
		return fmt.Errorf("could not find admin: %w", err)
	}

	if err := bcrypt.CompareHashAndPassword([]byte(admin.PasswordHash), []byte(oldPassword)); err != nil {
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

	if err := a.adminStorage.UpdatePassword(ctx, email, newHash); err != nil {
		recordSpanError(span, err)
		return fmt.Errorf("could not update password: %w", err)
	}

	if err := a.adminStorage.ClearMustChangePassword(ctx, email); err != nil {
		recordSpanError(span, err)
		return fmt.Errorf("could not clear must-change-password flag: %w", err)
	}

	return nil
}

// MustChangePassword reports whether the admin identified by email is
// currently flagged for a forced password change. A missing admin is
// treated as false so the gate cannot trap a misrouted lookup.
func (a AdminAuth) MustChangePassword(ctx context.Context, email string) (bool, error) {
	admin, err := a.adminStorage.FindByEmail(ctx, email)
	if errors.Is(err, domain.ErrAdminNotFound) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("could not find admin: %w", err)
	}
	return admin.MustChangePassword, nil
}

// FindByID returns the admin row by natural id. Used by the layout
// layer when it resolves the current admin's email from the admin
// session (the session stores id; the topbar wants the email).
//
// NOTE on the missing RequestPasswordReset / ResetPassword surface:
// admins are provisioned today, not self-served, so the by-email
// reset flow is intentionally out of scope. A follow-up can wire the
// existing PasswordResetStorage against auth_admin if/when operators
// need it.
func (a AdminAuth) FindByID(ctx context.Context, id string) (adapter.Admin, error) {
	return a.adminStorage.FindByID(ctx, id)
}

package app

import (
	"context"
	"fmt"
	"regexp"
	"time"

	"github.com/bkielbasa/go-ecommerce/backend/auth/adapter"
	"github.com/bkielbasa/go-ecommerce/backend/auth/domain"
	"golang.org/x/crypto/bcrypt"
)

var (
	ErrInvalidEmail = fmt.Errorf("invalid email")
)

var emailRegexp = regexp.MustCompile("^[a-zA-Z0-9.!#$%&'*+/=?^_`{|}~-]+@[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?(?:\\.[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?)*$")

type CustomerStorage interface {
	Create(ctx context.Context, email, passwordHash string) error
	Find(ctx context.Context, email string) (adapter.Customer, error)
}

type SessStorage interface {
	Store(ctx context.Context, sess *domain.Session) error
	Find(ctx context.Context, token string) (*domain.Session, error)
}

type auth struct {
	authStorage  CustomerStorage
	sessStorage  SessStorage
	passPolicies []domain.PasswordPolicy
}

func NewAuth(authStorage CustomerStorage, sessStorage SessStorage) auth {
	return auth{
		authStorage: authStorage, sessStorage: sessStorage,
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

func hashPassword(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(bytes), err
}

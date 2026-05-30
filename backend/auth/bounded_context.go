package auth

import (
	"context"
	"database/sql"

	"github.com/bkielbasa/go-ecommerce/backend/auth/adapter"
	"github.com/bkielbasa/go-ecommerce/backend/auth/app"
	"github.com/bkielbasa/go-ecommerce/backend/auth/domain"
	"github.com/bkielbasa/go-ecommerce/backend/auth/port"
	"github.com/bkielbasa/go-ecommerce/backend/internal/application"
	"github.com/bkielbasa/go-ecommerce/backend/internal/https"
	"github.com/gorilla/mux"
)

// appService is the customer-side application surface exposed by
// auth.New for the layout/composition root. The pre-split surface
// (which included IsAdmin and MustChangePassword) is gone — those
// methods moved to the dedicated AdminAuth service returned alongside.
type appService interface {
	CreateNewCustomer(ctx context.Context, email, password string) error
	Login(ctx context.Context, username string, password string) (*domain.Session, error)
	Logout(ctx context.Context, sesionID string) error
	FindByToken(ctx context.Context, sessToken string) (*domain.Session, error)
	ChangePassword(ctx context.Context, email, oldPassword, newPassword string) error
	RequestPasswordReset(ctx context.Context, email string) (string, error)
	ResetPassword(ctx context.Context, rawToken, newPassword string) error
}

// adminAppService is the admin-side application surface exposed by
// auth.New. It is a distinct interface so the composition root cannot
// accidentally swap a customer service in where an admin service is
// expected (and vice versa). Notably there is NO password-reset surface
// today — admins are provisioned, not self-served.
type adminAppService interface {
	CreateAdmin(ctx context.Context, email, password string) error
	Login(ctx context.Context, email, password string) (*domain.Session, error)
	Logout(ctx context.Context, token string) error
	FindByToken(ctx context.Context, token string) (*domain.Session, error)
	ChangePassword(ctx context.Context, email, oldPassword, newPassword string) error
	MustChangePassword(ctx context.Context, email string) (bool, error)
	FindByID(ctx context.Context, id string) (adapter.Admin, error)
}

// New wires the auth bounded context. As of the customer/admin split
// it returns TWO services: the customer-side `appService` (commerce
// users) and the admin-side `adminAppService` (operators). They share
// the same database but never each other's data — separate storages,
// separate session scopes (principal_kind discriminator), and the
// layout layer keeps their cookies apart too.
func New(db *sql.DB) (application.BoundedContext, appService, adminAppService) {
	authStorage := adapter.NewPostgresAuthStorage(db)
	sessStorage := adapter.NewPostgresSessionStorage(db)
	resetStorage := adapter.NewPostgresPasswordResetStorage(db)
	appServ := app.NewAuth(authStorage, sessStorage, resetStorage)

	adminStorage := adapter.NewPostgresAdminStorage(db)
	adminSessStorage := adapter.NewPostgresAdminSessionStorage(db)
	adminServ := app.NewAdminAuth(adminStorage, adminSessStorage)

	return &boundedContext{
		httpHandler: port.NewHTTP(appServ),
	}, appServ, adminServ
}

type boundedContext struct {
	httpHandler port.HTTP
}

func (m boundedContext) MuxRegister(r *mux.Router) {
	r.HandleFunc("/api/v1/auth/login", m.httpHandler.Login).Methods("POST")
	r.HandleFunc("/api/v1/auth/login", https.EmptyHandler).Methods("OPTIONS")
	r.HandleFunc("/api/v1/auth/register", m.httpHandler.Register).Methods("POST")
	r.HandleFunc("/api/v1/auth/register", https.EmptyHandler).Methods("OPTIONS")
	r.HandleFunc("/api/v1/auth/me", m.httpHandler.Me).Methods("GET")
	r.HandleFunc("/api/v1/auth/me", https.EmptyHandler).Methods("OPTIONS")
	r.HandleFunc("/api/v1/auth/logout", m.httpHandler.Logout).Methods("DELETE")
	r.HandleFunc("/api/v1/auth/logout", https.EmptyHandler).Methods("OPTIONS")
}

package auth

import (
	"database/sql"

	"github.com/bkielbasa/go-ecommerce/backend/auth/adapter"
	"github.com/bkielbasa/go-ecommerce/backend/auth/app"
	"github.com/bkielbasa/go-ecommerce/backend/auth/port"
	"github.com/bkielbasa/go-ecommerce/backend/internal/application"
	"github.com/gorilla/mux"
)

func New(db *sql.DB) application.BoundedContext {
	authStorage := adapter.NewPostgresAuthStorage(db)
	sessStorage := adapter.NewPostgresSessionStorage(db)
	appServ := app.NewAuth(authStorage, sessStorage)

	return &boundedContext{
		httpHandler: port.NewHTTP(appServ),
	}
}

type boundedContext struct {
	httpHandler port.HTTP
}

func (m boundedContext) MuxRegister(r *mux.Router) {
	r.HandleFunc("/api/v1/auth/login", m.httpHandler.Login).Methods("POST")
	r.HandleFunc("/api/v1/auth/register", m.httpHandler.Register).Methods("POST")
	r.HandleFunc("/api/v1/auth/me", m.httpHandler.Me).Methods("GET")
	r.HandleFunc("/api/v1/auth", m.httpHandler.Logout).Methods("DELETE")
}

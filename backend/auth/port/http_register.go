package port

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/bkielbasa/go-ecommerce/backend/auth/domain"
	"github.com/bkielbasa/go-ecommerce/backend/internal/https"
)

type NewClient struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func (h HTTP) Register(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var c NewClient
	if err := json.NewDecoder(r.Body).Decode(&c); err != nil {
		https.BadRequest(w, "serialization-error", err.Error())
		return
	}

	if err := h.auth.CreateNewCustomer(ctx, c.Username, c.Password); err != nil {
		var e domain.PasswordPolicyError
		if errors.As(err, &e) {
			https.BadRequest(w, "password-policy", err.Error())
			return
		}

		if errors.Is(err, domain.ErrCustomerExists) {
			https.BadRequest(w, "register", err.Error())
			return
		}

		https.InternalError(w, "register-error", err.Error())
		return
	}

	https.NoContent(w)
}

package port

import (
	"encoding/json"
	"net/http"

	"github.com/bkielbasa/go-ecommerce/backend/internal/https"
)

type NewClient struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// @Router       /auth/register [post]
// @Accept       json
// @Produce      json
// @Param user  body NewClient true "NewClient"
// @Failure      500  {object}  https.ErrorResponse
// @Failure      404  {object}  https.ErrorResponse
func (h HTTP) Register(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var c NewClient
	if err := json.NewDecoder(r.Body).Decode(&c); err != nil {
		https.BadRequest(w, "serialization-error", err.Error())
		return
	}

	if err := h.auth.CreateNewCustomer(ctx, c.Username, c.Password); err != nil {
		https.InternalError(w, "register-error", err.Error())
		return
	}

	https.NoContent(w)
}

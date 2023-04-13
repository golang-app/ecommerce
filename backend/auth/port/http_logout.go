package port

import (
	"errors"
	"net/http"

	"github.com/bkielbasa/go-ecommerce/backend/internal/https"
)

// @Router       /auth/logout [delete]
// @Accept       json
// @Produce      json
// @Failure      500  {object}  https.ErrorResponse
// @Failure      401  {object}  https.ErrorResponse
func (h HTTP) Logout(w http.ResponseWriter, r *http.Request) {
	tokenCookie, err := r.Cookie("session_id")
	if errors.Is(err, http.ErrNoCookie) {
		https.Unauthorized(w, "cookie-error", "no cookie")
		return
	}

	if err != nil {
		https.InternalError(w, "cookie-error", err.Error())
		return
	}

	if err := h.auth.Logout(r.Context(), tokenCookie.Value); err != nil {
		https.InternalError(w, "logout-error", err.Error())
		return
	}

	https.NoContent(w)
}

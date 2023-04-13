package port

import (
	"net/http"
)

// @Router       /auth [delete]
// @Accept       json
// @Produce      json
// @Failure      500  {object}  https.ErrorResponse
func (h HTTP) Logout(w http.ResponseWriter, r *http.Request) {
}

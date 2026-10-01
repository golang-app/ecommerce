package layout

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/mux"
	"github.com/matryer/is"
)

func TestAdminTrailingSlashRedirect(t *testing.T) {
	is := is.New(t)

	bc := boundedContext{
		handler: httpHandler{},
	}

	router := mux.NewRouter()
	bc.MuxRegister(router)

	req := httptest.NewRequest(http.MethodGet, "/admin/", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	is.Equal(rec.Code, http.StatusMovedPermanently)
	is.Equal(rec.Header().Get("Location"), "/admin")

	// Verify query parameter preservation
	reqWithQuery := httptest.NewRequest(http.MethodGet, "/admin/?tab=stats", nil)
	recWithQuery := httptest.NewRecorder()
	router.ServeHTTP(recWithQuery, reqWithQuery)
	is.Equal(recWithQuery.Code, http.StatusMovedPermanently)
	is.Equal(recWithQuery.Header().Get("Location"), "/admin?tab=stats")
}

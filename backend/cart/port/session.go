package port

import (
	"encoding/base32"
	"net/http"
	"strings"

	"github.com/bkielbasa/go-ecommerce/backend/internal/https"
	"github.com/gorilla/securecookie"
	"github.com/gorilla/sessions"
)

var (
	// key must be 16, 24 or 32 bytes long (AES-128, AES-192 or AES-256)
	key        = []byte("go-ecommerce")
	store      = sessions.NewCookieStore(key)
	cookieName = "ecommerce-session"
)

func EnusreCartID(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		session, _ := store.Get(r, cookieName)

		if session.IsNew {
			session.Values["cartID"] = strings.TrimRight(
				base32.StdEncoding.EncodeToString(
					securecookie.GenerateRandomKey(32)), "=")
		}

		err := store.Save(r, w, session)
		if err != nil {
			https.InternalError(w, "cannot save session data")
			return
		}

		next(w, r)
	}
}

func cartID(r *http.Request) string {
	session, _ := store.Get(r, cookieName)
	return session.Values["cartID"].(string)
}

package layout

import (
	"errors"
	"html/template"
	"io"
	"net/http"
	"os"

	"github.com/bkielbasa/go-ecommerce/backend/auth/domain"
)

func (handler httpHandler) Login(w http.ResponseWriter, r *http.Request) {
	handler.renderTemplate(w, r, "auth/login", nil)
}

func (handler httpHandler) AuthMenuItem(w http.ResponseWriter, r *http.Request) {
	c, err := store.Get(r, "ecommerce")
	if err != nil {
		handler.logger.WithError(err).Error("cannot get session store")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	var loggedIn bool

	sessID, _ := c.Values["session_id"].(string)
	sess, err := handler.authSrv.FindByToken(r.Context(), sessID)
	if err != nil {
		loggedIn = false
	} else {
		loggedIn = !sess.Expired()
	}

	f, err := os.Open("./layout/tmpl/auth/menuItem.gohtml")
	if err != nil {
		handler.logger.WithError(err).Error("cannot open menu item template")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	defer func() { _ = f.Close() }()

	body, err := io.ReadAll(f)
	if err != nil {
		handler.logger.WithError(err).Error("cannot read menu item template")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	tmpl, err := template.New("").Parse(string(body))
	if err != nil {
		handler.logger.WithError(err).Error("cannot parse menu item template")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	err = tmpl.ExecuteTemplate(w, "", map[string]any{
		"LoggedIn": loggedIn,
	})
	if err != nil {
		handler.logger.WithError(err).Error("cannot execute menu item template")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

func (handler httpHandler) HandleLogout(w http.ResponseWriter, r *http.Request) {
	session, _ := store.Get(r, "ecommerce")

	sessID, _ := session.Values["session_id"].(string)
	err := handler.authSrv.Logout(r.Context(), sessID)
	if err != nil {
		session.AddFlash(err.Error(), "error")
		_ = session.Save(r, w)
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	session.AddFlash("You are logged out")
	delete(session.Values, "session_id")
	_ = session.Save(r, w)

	http.Redirect(w, r, "/", http.StatusSeeOther)
}

type Client struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// HandleLogin processes the customer login form. As of the customer/admin
// split there is no must_change_password gate on this path — that flag
// lives on the Admin aggregate now, and the admin login handler is the
// only place that branches on it.
func (handler httpHandler) HandleLogin(w http.ResponseWriter, r *http.Request) {
	session, _ := store.Get(r, "ecommerce")

	// Per-IP rate limit: 5/min. Credential stuffing is the threat model; a
	// human re-typing the wrong password a handful of times still gets
	// through, but a bot trying dozens of passwords from the same source
	// gets bounced back to the login form with a flash.
	if !loginLimiter.Allow(clientIP(r)) {
		session.AddFlash("Too many login attempts, please try again in a moment.", "error")
		_ = session.Save(r, w)
		http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
		return
	}

	var c Client
	c.Username = r.FormValue("email")
	c.Password = r.FormValue("password")

	sess, err := handler.authSrv.Login(r.Context(), c.Username, c.Password)
	if err != nil {
		session.AddFlash(err.Error(), "error")
		_ = session.Save(r, w)
		http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
		return
	}

	session.Values["session_id"] = sess.ID()
	session.AddFlash("You are logged in")
	_ = session.Save(r, w)

	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (handler httpHandler) Register(w http.ResponseWriter, r *http.Request) {
	handler.renderTemplate(w, r, "auth/register", nil)
}

type NewClient struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func (handler httpHandler) HandleRegister(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	session, _ := store.Get(r, "ecommerce")

	// Per-IP rate limit: 3/hour. Real account signups are infrequent; the
	// rate is deliberately tight enough to throttle automated account
	// creation but loose enough that a household behind one NAT can still
	// register a couple of customers.
	if !registerLimiter.Allow(clientIP(r)) {
		session.AddFlash("Too many registration attempts, please try again later.", "error")
		_ = session.Save(r, w)
		http.Redirect(w, r, "/auth/register", http.StatusSeeOther)
		return
	}

	var c NewClient

	c.Username = r.FormValue("email")
	c.Password = r.FormValue("password")

	if err := handler.authSrv.CreateNewCustomer(ctx, c.Username, c.Password); err != nil {
		var e domain.PasswordPolicyError
		if errors.As(err, &e) {
			session.AddFlash(err.Error(), "error")
			_ = session.Save(r, w)
			http.Redirect(w, r, "/auth/register", http.StatusSeeOther)
			return
		}

		if errors.Is(err, domain.ErrCustomerExists) {
			session.AddFlash(err.Error(), "error")
			_ = session.Save(r, w)
			http.Redirect(w, r, "/auth/register", http.StatusSeeOther)
			return
		}

		session.AddFlash(err.Error(), "error")
		_ = session.Save(r, w)
		http.Redirect(w, r, "/auth/register", http.StatusSeeOther)
		return
	}

	http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
	session.AddFlash("You are registered. You can log in now")
	_ = session.Save(r, w)
	http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
}

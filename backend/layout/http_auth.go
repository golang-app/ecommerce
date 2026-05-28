package layout

import (
  "errors"
	"html/template"
  "os"
  "io"
	"net/http"

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
  delete(session.Values,"session_id")
	_ = session.Save(r, w)

	http.Redirect(w, r, "/", http.StatusSeeOther)
}

type Client struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func (handler httpHandler) HandleLogin(w http.ResponseWriter, r *http.Request) {
	var c Client
	c.Username = r.FormValue("email")
	c.Password = r.FormValue("password")
	session, _ := store.Get(r, "ecommerce")

	sess, err := handler.authSrv.Login(r.Context(), c.Username, c.Password)
	if err != nil {
		session.AddFlash(err.Error(), "error")
		_ = session.Save(r, w)
		http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
		return
	}

	session.Values["session_id"] = sess.ID()

	// Forced password change: if the just-logged-in customer is flagged,
	// short-circuit the normal landing page and send them to the gate. A
	// lookup error is treated as "not flagged" so a transient DB hiccup
	// doesn't lock the user out of the storefront.
	mustChange, mcpErr := handler.authSrv.MustChangePassword(r.Context(), c.Username)
	if mcpErr == nil && mustChange {
		session.AddFlash("Please choose a new password to continue.")
		_ = session.Save(r, w)
		http.Redirect(w, r, "/auth/change-password", http.StatusSeeOther)
		return
	}

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
	var c NewClient

	c.Username = r.FormValue("email")
	c.Password = r.FormValue("password")
	session, _ := store.Get(r, "ecommerce")

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

// ChangePasswordPage renders the forced password-change form. It is only
// reachable for a logged-in customer whose must_change_password flag is true.
// Any other caller is redirected away so the page never serves as a stealth
// alternative to /account/details/password.
func (handler httpHandler) ChangePasswordPage(w http.ResponseWriter, r *http.Request) {
	email, ok := handler.requireLoginAllowMustChange(w, r)
	if !ok {
		return
	}
	must, err := handler.authSrv.MustChangePassword(r.Context(), email)
	if err != nil || !must {
		http.Redirect(w, r, "/account", http.StatusSeeOther)
		return
	}
	handler.renderTemplate(w, r, "auth/change_password", map[string]any{
		"Email": email,
	})
}

// HandleChangePassword processes the forced password-change form. On success
// the auth service clears the must_change_password flag and the user is sent
// to /admin (the only flag holders today are admins). On policy/old-password
// errors we flash and re-render the form.
func (handler httpHandler) HandleChangePassword(w http.ResponseWriter, r *http.Request) {
	email, ok := handler.requireLoginAllowMustChange(w, r)
	if !ok {
		return
	}
	must, err := handler.authSrv.MustChangePassword(r.Context(), email)
	if err != nil || !must {
		http.Redirect(w, r, "/account", http.StatusSeeOther)
		return
	}

	if err := r.ParseForm(); err != nil {
		handler.flash(w, r, err.Error(), "error")
		http.Redirect(w, r, "/auth/change-password", http.StatusSeeOther)
		return
	}

	oldPassword := r.FormValue("current_password")
	newPassword := r.FormValue("new_password")
	confirm := r.FormValue("confirm_password")

	if newPassword != confirm {
		handler.flash(w, r, "new password and confirmation do not match", "error")
		http.Redirect(w, r, "/auth/change-password", http.StatusSeeOther)
		return
	}

	if err := handler.authSrv.ChangePassword(r.Context(), email, oldPassword, newPassword); err != nil {
		handler.flash(w, r, err.Error(), "error")
		http.Redirect(w, r, "/auth/change-password", http.StatusSeeOther)
		return
	}

	handler.flash(w, r, "Password updated. Welcome to GoCommerce admin.", "info")
	http.Redirect(w, r, "/admin", http.StatusSeeOther)
}

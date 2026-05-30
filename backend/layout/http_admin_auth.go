package layout

import (
	"net/http"
)

// adminSessionCookie is the cookie name for the admin session. It is
// intentionally distinct from the customer cookie ("ecommerce") so a
// single browser can hold both an admin and a customer session at the
// same time — useful for QA-ing the whole demo without juggling
// incognito windows. The cookies share the same store secret and
// options; only the name differs.
const adminSessionCookie = "ecommerce_admin"

// adminSessionIDKey is the key under which the admin session token
// lives inside the gorilla session values map. Mirrors the customer
// side's "session_id" key but stays separate so a stray cookie
// collision (cookie name + key name) cannot leak across kinds.
const adminSessionIDKey = "session_id"

// currentAdminID resolves the currently logged-in admin's id from
// the admin session cookie, or "" if no valid admin session is present.
// Replaces the previous IsAdmin-on-customer flow.
func (handler httpHandler) currentAdminID(r *http.Request) string {
	c, err := store.Get(r, adminSessionCookie)
	if err != nil {
		return ""
	}
	sessID, _ := c.Values[adminSessionIDKey].(string)
	if sessID == "" {
		return ""
	}
	sess, err := handler.adminAuthSrv.FindByToken(r.Context(), sessID)
	if err != nil || sess == nil || sess.Expired() {
		return ""
	}
	return sess.CustomerID()
}

// currentAdminEmail resolves the email of the currently logged-in
// admin. The session row stores the admin's id (which today happens
// to equal the email, but the AdminAuth service is the authoritative
// translator) so we go through FindByID to get the email proper.
func (handler httpHandler) currentAdminEmail(r *http.Request) string {
	id := handler.currentAdminID(r)
	if id == "" {
		return ""
	}
	admin, err := handler.adminAuthSrv.FindByID(r.Context(), id)
	if err != nil {
		return ""
	}
	return admin.Email
}

// AdminLoginPage renders the admin login form. It is a separate template
// from the storefront login because the admin shell has its own visual
// language (and we never want to leak "are you an admin?" via the
// storefront login UX).
func (handler httpHandler) AdminLoginPage(w http.ResponseWriter, r *http.Request) {
	handler.renderTemplate(w, r, "admin/login", nil)
}

// HandleAdminLogin processes the admin login form. On success it mints
// a session, stores its id in the admin cookie, and redirects either to
// /admin/change-password (when the must-change-password gate is set)
// or to the admin dashboard.
func (handler httpHandler) HandleAdminLogin(w http.ResponseWriter, r *http.Request) {
	session, _ := store.Get(r, adminSessionCookie)
	csrfSession, _ := store.Get(r, "ecommerce")

	// Re-use the storefront login rate limiter. Admin logins are
	// infrequent enough that they will not exhaust the budget on
	// their own and credential stuffing against the admin form is
	// the same threat model.
	if !loginLimiter.Allow(clientIP(r)) {
		csrfSession.AddFlash("Too many login attempts, please try again in a moment.", "error")
		_ = csrfSession.Save(r, w)
		http.Redirect(w, r, "/admin/login", http.StatusSeeOther)
		return
	}

	email := r.FormValue("email")
	password := r.FormValue("password")

	sess, err := handler.adminAuthSrv.Login(r.Context(), email, password)
	if err != nil {
		csrfSession.AddFlash(err.Error(), "error")
		_ = csrfSession.Save(r, w)
		http.Redirect(w, r, "/admin/login", http.StatusSeeOther)
		return
	}

	session.Values[adminSessionIDKey] = sess.ID()
	if err := session.Save(r, w); err != nil {
		handler.logger.WithError(err).Error("cannot save admin session")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// Force the password change before any admin work. A lookup
	// error is treated as "not flagged" so a transient DB hiccup
	// does not lock the operator out of the panel entirely.
	if must, mcpErr := handler.adminAuthSrv.MustChangePassword(r.Context(), email); mcpErr == nil && must {
		csrfSession.AddFlash("Please choose a new password to continue.")
		_ = csrfSession.Save(r, w)
		http.Redirect(w, r, "/admin/change-password", http.StatusSeeOther)
		return
	}

	csrfSession.AddFlash("You are logged in")
	_ = csrfSession.Save(r, w)
	http.Redirect(w, r, "/admin", http.StatusSeeOther)
}

// HandleAdminLogout invalidates the admin session and clears the cookie
// value so the next request resolves as anonymous-admin. The customer
// session (if any) is untouched.
func (handler httpHandler) HandleAdminLogout(w http.ResponseWriter, r *http.Request) {
	session, _ := store.Get(r, adminSessionCookie)
	csrfSession, _ := store.Get(r, "ecommerce")

	sessID, _ := session.Values[adminSessionIDKey].(string)
	if sessID != "" {
		if err := handler.adminAuthSrv.Logout(r.Context(), sessID); err != nil {
			csrfSession.AddFlash(err.Error(), "error")
			_ = csrfSession.Save(r, w)
			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		}
	}

	delete(session.Values, adminSessionIDKey)
	_ = session.Save(r, w)
	csrfSession.AddFlash("You are logged out")
	_ = csrfSession.Save(r, w)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// AdminChangePasswordPage renders the forced password-change form for
// admins. Only reachable when the admin session is present AND the
// must_change_password flag is set; any other state redirects to the
// dashboard so the page does not double as a stealth password-change
// endpoint outside the gate.
func (handler httpHandler) AdminChangePasswordPage(w http.ResponseWriter, r *http.Request) {
	email := handler.currentAdminEmail(r)
	if email == "" {
		http.Redirect(w, r, "/admin/login", http.StatusSeeOther)
		return
	}
	must, err := handler.adminAuthSrv.MustChangePassword(r.Context(), email)
	if err != nil || !must {
		http.Redirect(w, r, "/admin", http.StatusSeeOther)
		return
	}
	handler.renderTemplate(w, r, "admin/change_password", map[string]any{
		"Email": email,
	})
}

// HandleAdminChangePassword processes the forced password-change form.
// On success the AdminAuth service clears the must_change_password flag
// and the operator is sent to /admin.
func (handler httpHandler) HandleAdminChangePassword(w http.ResponseWriter, r *http.Request) {
	email := handler.currentAdminEmail(r)
	if email == "" {
		http.Redirect(w, r, "/admin/login", http.StatusSeeOther)
		return
	}
	must, err := handler.adminAuthSrv.MustChangePassword(r.Context(), email)
	if err != nil || !must {
		http.Redirect(w, r, "/admin", http.StatusSeeOther)
		return
	}

	if err := r.ParseForm(); err != nil {
		handler.flash(w, r, err.Error(), "error")
		http.Redirect(w, r, "/admin/change-password", http.StatusSeeOther)
		return
	}

	oldPassword := r.FormValue("current_password")
	newPassword := r.FormValue("new_password")
	confirm := r.FormValue("confirm_password")

	if newPassword != confirm {
		handler.flash(w, r, "new password and confirmation do not match", "error")
		http.Redirect(w, r, "/admin/change-password", http.StatusSeeOther)
		return
	}

	if err := handler.adminAuthSrv.ChangePassword(r.Context(), email, oldPassword, newPassword); err != nil {
		handler.flash(w, r, err.Error(), "error")
		http.Redirect(w, r, "/admin/change-password", http.StatusSeeOther)
		return
	}

	handler.flash(w, r, "Password updated. Welcome to GoCommerce admin.", "info")
	http.Redirect(w, r, "/admin", http.StatusSeeOther)
}

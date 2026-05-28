package layout

import (
	"errors"
	"net/http"

	"github.com/bkielbasa/go-ecommerce/backend/auth/adapter"
)

// passwordResetTTLMinutes mirrors the auth-app TTL (30 minutes) so the email
// body can advertise how long the link stays valid without dragging the app
// constant into the template layer.
const passwordResetTTLMinutes = 30

// genericForgotConfirmation is the single message every /auth/forgot POST
// flashes back. The wording deliberately does NOT confirm whether the
// account existed — that would turn the form into an account-enumeration
// oracle.
const genericForgotConfirmation = "If an account exists, an email is on its way."

// ForgotPasswordPage renders the request form. It is always reachable —
// there is no point in gating it on session state, since the user is
// definitionally locked out at this point.
func (handler httpHandler) ForgotPasswordPage(w http.ResponseWriter, r *http.Request) {
	handler.renderTemplate(w, r, "auth/forgot", nil)
}

// HandleForgotPassword processes the request form. The flow is intentionally
// stateless from the caller's perspective: regardless of whether the email
// matches an account, we flash the same generic confirmation and redirect
// to /auth/login. The mailer call only fires when a token was actually
// minted (i.e. the account existed); failures inside the mailer are logged
// but never surfaced to the caller — leaking "we tried to email you but it
// bounced" would re-introduce the enumeration leak.
func (handler httpHandler) HandleForgotPassword(w http.ResponseWriter, r *http.Request) {
	session, _ := store.Get(r, "ecommerce")

	if !forgotPasswordLimiter.Allow(clientIP(r)) {
		session.AddFlash("Too many reset requests, please try again later.", "error")
		_ = session.Save(r, w)
		http.Redirect(w, r, "/auth/forgot", http.StatusSeeOther)
		return
	}

	email := r.FormValue("email")
	rawToken, err := handler.authSrv.RequestPasswordReset(r.Context(), email)
	if err != nil {
		// A transient DB error gets logged and surfaced as the generic
		// confirmation — we still don't want to confirm or deny the
		// account's existence. The operator sees the real cause in
		// the logs.
		handler.logger.WithError(err).Warn("RequestPasswordReset failed")
	}

	if rawToken != "" {
		msg, rerr := RenderPasswordReset(email, rawToken, handler.baseURL, passwordResetTTLMinutes)
		if rerr != nil {
			handler.logger.WithError(rerr).Error("cannot render password reset email")
		} else if handler.mailer != nil {
			if serr := handler.mailer.Send(r.Context(), msg); serr != nil {
				handler.logger.WithError(serr).Error("cannot send password reset email")
			}
		}
	}

	session.AddFlash(genericForgotConfirmation)
	_ = session.Save(r, w)
	http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
}

// ResetPasswordPage renders the new-password form. The token from the URL
// is passed through as a hidden input so the POST has it without us having
// to round-trip it via the session.
func (handler httpHandler) ResetPasswordPage(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	handler.renderTemplate(w, r, "auth/reset", map[string]any{
		"Token": token,
	})
}

// HandleResetPassword processes the new-password form. On any token
// failure (unknown / expired / already-used) the caller is bounced back
// to /auth/forgot with a generic message; the underlying ErrInvalidResetToken
// from the storage layer is intentionally opaque.
func (handler httpHandler) HandleResetPassword(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		handler.flash(w, r, "could not parse form", "error")
		http.Redirect(w, r, "/auth/forgot", http.StatusSeeOther)
		return
	}

	token := r.FormValue("token")
	newPassword := r.FormValue("new_password")
	confirm := r.FormValue("confirm_password")

	if newPassword != confirm {
		handler.flash(w, r, "new password and confirmation do not match", "error")
		http.Redirect(w, r, "/auth/reset?token="+token, http.StatusSeeOther)
		return
	}

	if err := handler.authSrv.ResetPassword(r.Context(), token, newPassword); err != nil {
		if errors.Is(err, adapter.ErrInvalidResetToken) {
			handler.flash(w, r, "this reset link is invalid or has expired. Please request a new one.", "error")
			http.Redirect(w, r, "/auth/forgot", http.StatusSeeOther)
			return
		}
		// Any other error (policy failure, hash failure, storage
		// failure) is surfaced verbatim — these are all things the
		// user can either fix (policy) or report (everything else).
		handler.flash(w, r, err.Error(), "error")
		http.Redirect(w, r, "/auth/reset?token="+token, http.StatusSeeOther)
		return
	}

	handler.flash(w, r, "Your password has been updated. Please sign in.", "info")
	http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
}

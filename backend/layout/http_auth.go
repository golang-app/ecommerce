package layout

import (
  "errors"
	"html/template"
	"log"
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
		log.Print(err.Error())
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
		log.Print(err.Error())
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
  defer f.Close()

  body, err := io.ReadAll(f)
	if err != nil {
		log.Print(err.Error())
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

  tmpl, err := template.New("").Parse(string(body))
	if err != nil {
		log.Print(err.Error())
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	err = tmpl.ExecuteTemplate(w, "", map[string]any{
    "LoggedIn": loggedIn,
  })
	if err != nil {
		log.Print(err.Error())
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

	session.AddFlash("You are logged in")
  session.Values["session_id"] = sess.ID()
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

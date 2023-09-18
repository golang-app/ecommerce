package homepage

import (
	_ "embed"
	"html/template"
	"net/http"

	"github.com/bkielbasa/go-ecommerce/backend/internal/application"
	"github.com/gorilla/mux"
)

func New() application.BoundedContext {
	return &boundedContext{}
}

type boundedContext struct {
}

func (m boundedContext) MuxRegister(r *mux.Router) {
	r.HandleFunc("/", HomePage)
}

//go:embed tmpl/home.gohtml
var homePageTmpl string

func HomePage(w http.ResponseWriter, r *http.Request) {
	tmpl, err := template.New("homepage").Parse(homePageTmpl)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	err = tmpl.Execute(w, nil)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

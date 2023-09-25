package layout

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

var tmpl *template.Template

func init() {
	tmpl = template.Must(template.ParseGlob("layout/tmpl/*.gotmpl"))
}

func HomePage(w http.ResponseWriter, r *http.Request) {
	renderTemplate(w, "home", nil)
}

func renderTemplate(w http.ResponseWriter, templateName string, data interface{}) {
	// Execute the content template
	err := tmpl.ExecuteTemplate(w, templateName+".gotmpl", data)
	if err != nil {
		http.Error(w, "Failed to render template", http.StatusInternalServerError)
	}

	// Execute the layout with the rendered content embedded
	err = tmpl.ExecuteTemplate(w, "layout.gotmpl", data)
	if err != nil {
		http.Error(w, "Failed to render layout", http.StatusInternalServerError)
	}
}

package layout

import (
	"net/http"
	"sort"
	"strconv"

	"github.com/gorilla/mux"
)

// orderedMembers reads the submitted member selection: the checked "member"
// values (attribute type ids) sorted ascending by their companion
// "order_<typeID>" numeric input. Missing/blank order values sort last (a large
// default), ties broken by the order ids appear in the form. The returned slice
// is the ordered attribute type ids to assign to the set.
func orderedMembers(r *http.Request) []string {
	const defaultOrder = 1 << 30
	members := r.Form["member"]
	type entry struct {
		id    string
		order int
		idx   int
	}
	entries := make([]entry, 0, len(members))
	for i, id := range members {
		order := defaultOrder
		if raw := r.FormValue("order_" + id); raw != "" {
			if v, err := strconv.Atoi(raw); err == nil {
				order = v
			}
		}
		entries = append(entries, entry{id: id, order: order, idx: i})
	}
	sort.SliceStable(entries, func(i, j int) bool {
		if entries[i].order != entries[j].order {
			return entries[i].order < entries[j].order
		}
		return entries[i].idx < entries[j].idx
	})
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		out = append(out, e.id)
	}
	return out
}

// AdminAttributeSets renders the attribute sets list page.
func (handler httpHandler) AdminAttributeSets(w http.ResponseWriter, r *http.Request) {
	email, ok := handler.requireAdmin(w, r)
	if !ok {
		return
	}
	sets, err := handler.catalogSrv.AttributeSets(r.Context())
	if err != nil {
		sets = nil
	}
	handler.renderAdminTemplate(w, r, "admin/attribute_sets", map[string]any{
		"Active": "attribute-sets",
		"Email":  email,
		"Sets":   sets,
	})
}

// AdminNewAttributeSetForm renders the create-set form (all attribute types
// listed, none checked).
func (handler httpHandler) AdminNewAttributeSetForm(w http.ResponseWriter, r *http.Request) {
	email, ok := handler.requireAdmin(w, r)
	if !ok {
		return
	}
	types, err := handler.catalogSrv.AllAttributeTypes(r.Context())
	if err != nil {
		types = nil
	}
	rows := make([]map[string]any, 0, len(types))
	for i := range types {
		rows = append(rows, map[string]any{
			"Type":    types[i],
			"Checked": false,
			"Order":   i + 1,
		})
	}
	handler.renderAdminTemplate(w, r, "admin/attribute_set_edit", map[string]any{
		"Active":  "attribute-sets",
		"Email":   email,
		"IsNew":   true,
		"SetName": "",
		"Members": rows,
	})
}

// AdminCreateAttributeSet handles the create-set form submission.
func (handler httpHandler) AdminCreateAttributeSet(w http.ResponseWriter, r *http.Request) {
	if _, ok := handler.requireAdmin(w, r); !ok {
		return
	}
	_ = r.ParseForm()
	ids := orderedMembers(r)
	err := handler.catalogSrv.CreateAttributeSet(r.Context(), r.FormValue("name"), ids)
	if err != nil {
		handler.flash(w, r, err.Error(), "error")
		http.Redirect(w, r, "/admin/attribute-sets/new", http.StatusSeeOther)
		return
	}
	handler.flash(w, r, "Attribute set created", "info")
	http.Redirect(w, r, "/admin/attribute-sets", http.StatusSeeOther)
}

// AdminEditAttributeSetForm renders the edit form: members first (in their
// stored order), then the remaining attribute types unchecked.
func (handler httpHandler) AdminEditAttributeSetForm(w http.ResponseWriter, r *http.Request) {
	email, ok := handler.requireAdmin(w, r)
	if !ok {
		return
	}
	id := mux.Vars(r)["id"]
	set, err := handler.catalogSrv.FindAttributeSet(r.Context(), id)
	if err != nil {
		handler.flash(w, r, "Attribute set not found", "error")
		http.Redirect(w, r, "/admin/attribute-sets", http.StatusSeeOther)
		return
	}
	types, err := handler.catalogSrv.AllAttributeTypes(r.Context())
	if err != nil {
		types = nil
	}

	memberOrder := map[string]int{}
	for i, m := range set.Members() {
		memberOrder[m.ID()] = i + 1
	}

	// Members first (in stored order), then the rest unchecked.
	rows := make([]map[string]any, 0, len(types))
	for _, m := range set.Members() {
		rows = append(rows, map[string]any{
			"Type":    m,
			"Checked": true,
			"Order":   memberOrder[m.ID()],
		})
	}
	next := len(set.Members()) + 1
	for i := range types {
		if _, isMember := memberOrder[types[i].ID()]; isMember {
			continue
		}
		rows = append(rows, map[string]any{
			"Type":    types[i],
			"Checked": false,
			"Order":   next,
		})
		next++
	}

	handler.renderAdminTemplate(w, r, "admin/attribute_set_edit", map[string]any{
		"Active":  "attribute-sets",
		"Email":   email,
		"IsNew":   false,
		"SetID":   set.ID(),
		"SetName": set.Name(),
		"Members": rows,
	})
}

// AdminUpdateAttributeSet handles the edit-set form submission.
func (handler httpHandler) AdminUpdateAttributeSet(w http.ResponseWriter, r *http.Request) {
	if _, ok := handler.requireAdmin(w, r); !ok {
		return
	}
	id := mux.Vars(r)["id"]
	_ = r.ParseForm()
	ids := orderedMembers(r)
	err := handler.catalogSrv.UpdateAttributeSet(r.Context(), id, r.FormValue("name"), ids)
	if err != nil {
		handler.flash(w, r, err.Error(), "error")
		http.Redirect(w, r, "/admin/attribute-sets/"+id+"/edit", http.StatusSeeOther)
		return
	}
	handler.flash(w, r, "Attribute set updated", "info")
	http.Redirect(w, r, "/admin/attribute-sets", http.StatusSeeOther)
}

// AdminDeleteAttributeSet deletes an attribute set (its members cascade).
func (handler httpHandler) AdminDeleteAttributeSet(w http.ResponseWriter, r *http.Request) {
	if _, ok := handler.requireAdmin(w, r); !ok {
		return
	}
	id := mux.Vars(r)["id"]
	if err := handler.catalogSrv.DeleteAttributeSet(r.Context(), id); err != nil {
		handler.flash(w, r, err.Error(), "error")
	} else {
		handler.flash(w, r, "Attribute set deleted", "info")
	}
	http.Redirect(w, r, "/admin/attribute-sets", http.StatusSeeOther)
}

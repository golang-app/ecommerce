package domain

import "errors"

var (
	ErrAdminExists   = errors.New("admin already exists")
	ErrAdminNotFound = errors.New("admin not found")
)

// Admin is the operator identity inside the auth bounded context. It is a
// SEPARATE aggregate from Customer because:
//   - the commands are different (admins are provisioned, customers self-
//     register; admins never reset by email today, customers can),
//   - the gates are different (admins carry must_change_password, customers
//     do not),
//   - the lifecycles are different (customers can be created in bulk via
//     the storefront register form; admins arrive through seeds / CLI).
//
// The password hash is intentionally NOT modeled here — it is a storage
// concern that lives on the adapter DTO. Email is the natural-key identity;
// role exists for forward-compat (today every admin is "admin", later we
// can split out reviewers / fulfillment / etc.).
type Admin struct {
	id    string
	email string
	role  string
}

func NewAdmin(id, email, role string) *Admin {
	return &Admin{id: id, email: email, role: role}
}

func (a Admin) ID() string    { return a.id }
func (a Admin) Email() string { return a.email }
func (a Admin) Role() string  { return a.role }

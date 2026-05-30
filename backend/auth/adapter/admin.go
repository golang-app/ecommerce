package adapter

// Admin is the DTO mirror of domain.Admin plus the storage-only fields
// (password hash, must_change_password gate). The domain aggregate
// deliberately does not carry these — they are operational concerns of
// the storage layer, surfaced via dedicated commands on the AdminAuth
// service.
type Admin struct {
	ID                 string
	Email              string
	PasswordHash       string
	Role               string
	MustChangePassword bool
}

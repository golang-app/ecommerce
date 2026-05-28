package adapter

// we don't create a domain.Customer here because we don't want to expose the password hash
// it's too low level detail for the domain.
// The Customer is treat as a DTO (Data Transfer Object) here
// and after the password hash is verified, we can create a domain.Customer
type Customer struct {
	Username     string
	PasswordHash string
	IsAdmin      bool
	// MustChangePassword is true when the customer has to set a new password
	// before they can use the site (the seeded admin starts true; the change
	// is cleared after a successful ChangePassword).
	MustChangePassword bool
}

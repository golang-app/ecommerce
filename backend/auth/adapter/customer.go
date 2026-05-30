package adapter

// we don't create a domain.Customer here because we don't want to expose the password hash
// it's too low level detail for the domain.
// The Customer is treat as a DTO (Data Transfer Object) here
// and after the password hash is verified, we can create a domain.Customer
//
// Note: as of the customer/admin split (migration 000038), is_admin and
// must_change_password are NO LONGER fields on the Customer aggregate.
// Operators live in `auth_admin` and carry their own must_change_password
// gate there.
type Customer struct {
	Username     string
	PasswordHash string
}

package domain

type PasswordPolicy func(string) error

type PasswordPolicyError string

func (e PasswordPolicyError) Error() string {
	return string(e)
}

// seee: https://cheatsheetseries.owasp.org/cheatsheets/Authentication_Cheat_Sheet.html#implement-proper-password-strength-controls
func MinLength(n int) PasswordPolicy {
	return func(password string) error {
		if len(password) < n {
			return PasswordPolicyError("password is too short")
		}
		return nil
	}
}

// see: https://cheatsheetseries.owasp.org/cheatsheets/Authentication_Cheat_Sheet.html#implement-proper-password-strength-controls
func MaxLength(n int) PasswordPolicy {
	return func(password string) error {
		if len(password) > n {
			return PasswordPolicyError("password is too long")
		}
		return nil
	}
}

func MustContainLowercase(password string) error {
	for _, c := range password {
		if c >= 'a' && c <= 'z' {
			return nil
		}
	}

	return PasswordPolicyError("password does not contain lowercase")
}

func MustContainUppercase(password string) error {
	for _, c := range password {
		if c >= 'A' && c <= 'Z' {
			return nil
		}
	}

	return PasswordPolicyError("password does not contain uppercase")
}

func MustContainNumber(password string) error {
	for _, c := range password {
		if c >= '0' && c <= '9' {
			return nil
		}
	}

	return PasswordPolicyError("password does not contain number")
}

func MustContainSpecialChar(password string) error {
	for _, c := range password {
		if (c >= '!' && c <= '/') || (c >= ':' && c <= '@') || (c >= '[' && c <= '`') || (c >= '{' && c <= '~') {
			return nil
		}
	}

	return PasswordPolicyError("password does not contain special character")
}

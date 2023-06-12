package domain

import "errors"

type PasswordPolicy func(string) error

var ErrPasswordTooShort = errors.New("password is too short")
var ErrPasswordLeaked = errors.New("password leaked")
var ErrPasswordTooLong = errors.New("password too long")
var ErrPasswordDoesNotContainLowercase = errors.New("password does not contain lowercase letter")
var ErrPasswordDoesNotContainUppercase = errors.New("password does not contain uppercase letter")
var ErrPasswordDoesNotContainNumber = errors.New("password does not contain number")
var ErrPasswordDoesNotContainSpecialChar = errors.New("password does not contain special character")

// seee: https://cheatsheetseries.owasp.org/cheatsheets/Authentication_Cheat_Sheet.html#implement-proper-password-strength-controls
func MinLength(n int) PasswordPolicy {
	return func(password string) error {
		if len(password) < n {
			return ErrPasswordTooShort
		}
		return nil
	}
}

// see: https://cheatsheetseries.owasp.org/cheatsheets/Authentication_Cheat_Sheet.html#implement-proper-password-strength-controls
func MaxLength(n int) PasswordPolicy {
	return func(password string) error {
		if len(password) > n {
			return ErrPasswordTooLong
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

	return ErrPasswordDoesNotContainLowercase
}

func MustContainUppercase(password string) error {
	for _, c := range password {
		if c >= 'A' && c <= 'Z' {
			return nil
		}
	}

	return ErrPasswordDoesNotContainUppercase
}

func MustContainNumber(password string) error {
	for _, c := range password {
		if c >= '0' && c <= '9' {
			return nil
		}
	}

	return ErrPasswordDoesNotContainNumber
}

func MustContainSpecialChar(password string) error {
	for _, c := range password {
		if (c >= '!' && c <= '/') || (c >= ':' && c <= '@') || (c >= '[' && c <= '`') || (c >= '{' && c <= '~') {
			return nil
		}
	}

	return ErrPasswordDoesNotContainSpecialChar
}

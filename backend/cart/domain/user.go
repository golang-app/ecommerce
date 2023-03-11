package domain

type User struct {
	id string
}

func NewUser(id string) User {
	return User{id: id}
}

func (u User) ID() string {
	return u.id
}

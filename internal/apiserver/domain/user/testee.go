package user

import "time"

type Testee struct {
	UserID   UserID
	Name     string
	Sex      uint8
	Birthday time.Time
}

func NewTestee(userID UserID, name string) *Testee {
	return &Testee{UserID: userID, Name: name}
}

func (t *Testee) GetUserID() UserID {
	return t.UserID
}

func (t *Testee) GetName() string {
	return t.Name
}

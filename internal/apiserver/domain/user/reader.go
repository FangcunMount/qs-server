package user

type Reader struct {
	UserID UserID
	Name   string
}

func NewReader(userID UserID, name string) *Reader {
	return &Reader{UserID: userID, Name: name}
}

func (r *Reader) GetUserID() UserID {
	return r.UserID
}

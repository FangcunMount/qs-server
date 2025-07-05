package user

type Writer struct {
	UserID UserID
	Name   string
}

func NewWriter(userID UserID, name string) *Writer {
	return &Writer{UserID: userID, Name: name}
}

func (w *Writer) GetUserID() UserID {
	return w.UserID
}

func (w *Writer) GetName() string {
	return w.Name
}

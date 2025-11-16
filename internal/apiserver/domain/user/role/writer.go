package role

import "github.com/FangcunMount/qs-server/internal/apiserver/domain/user"

type Writer struct {
	UserID user.UserID
	Name   string
}

func NewWriter(userID user.UserID, name string) *Writer {
	return &Writer{UserID: userID, Name: name}
}

func (w *Writer) GetUserID() user.UserID {
	return w.UserID
}

func (w *Writer) GetName() string {
	return w.Name
}

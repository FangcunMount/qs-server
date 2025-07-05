package code

// apiserver: user errors.
const (
	// ErrUserNotFound - 404: User not found.
	ErrUserNotFound int = iota + 110001

	// ErrUserAlreadyExists- 400: User already exist.
	ErrUserAlreadyExists

	// ErrUserBasicInfoInvalid - 400: User basic info is invalid.
	ErrUserBasicInfoInvalid

	// ErrUserStatusInvalid - 400: User status is invalid.
	ErrUserStatusInvalid

	// ErrUserInvalid - 400: User is invalid.
	ErrUserInvalid

	// ErrUserBlocked - 403: User is blocked.
	ErrUserBlocked

	// ErrUserInactive - 403: User is inactive.
	ErrUserInactive
)

package domain

// Error is a safe message for API clients. Internal causes stay in logs.
type Error struct {
	Code    string
	Message string
	Status  int
}

func (e *Error) Error() string { return e.Message }

func E(status int, code, msg string) *Error {
	return &Error{Status: status, Code: code, Message: msg}
}

func As(err error) *Error {
	if err == nil {
		return nil
	}
	if e, ok := err.(*Error); ok {
		return e
	}
	return nil
}

package melody

import "errors"

// Errors
var (
	ErrMelodyClosed        = errors.New("melody instance is closed")
	ErrMelodyAlreadyClosed = errors.New("melody instance is already closed")

	ErrWriteToClosedSession = errors.New("tried to write to closed a session")
	ErrMessageBufferFull    = errors.New("session message buffer is full")
	ErrSessionClosed        = errors.New("session is closed")
	ErrSessionAlreadyClosed = errors.New("session is already closed")
)

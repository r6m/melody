package melody

import (
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

type Session interface {
	writeMessage(*envelope)
	writeRaw(*envelope) error
	closed() bool
	close()
	ping()
	writePump()
	readPump()
	Write([]byte) error
	WriteBinary([]byte) error
	Close() error
	CloseWithMsg([]byte) error
	Set(string, interface{})
	Get(string) (interface{}, bool)
	MustGet(string) interface{}
	IsClosed() bool
}

// session wrapper around websocket connections.
type session struct {
	Request *http.Request
	Keys    map[string]interface{}
	conn    *websocket.Conn
	output  chan *envelope
	melody  *Melody
	open    bool
	rwmutex *sync.RWMutex
}

func (s *session) writeMessage(message *envelope) {
	if s.closed() {
		s.melody.errorHandler(s, ErrWriteToClosedSession)
		return
	}

	select {
	case s.output <- message:
	default:
		s.melody.errorHandler(s, ErrMessageBufferFull)
	}
}

func (s *session) writeRaw(message *envelope) error {
	if s.closed() {
		return ErrWriteToClosedSession
	}

	s.conn.SetWriteDeadline(time.Now().Add(s.melody.Config.WriteWait))
	err := s.conn.WriteMessage(message.t, message.msg)

	if err != nil {
		return err
	}

	return nil
}

func (s *session) closed() bool {
	s.rwmutex.RLock()
	defer s.rwmutex.RUnlock()

	return !s.open
}

func (s *session) close() {
	if !s.closed() {
		s.rwmutex.Lock()
		s.open = false
		s.conn.Close()
		close(s.output)
		s.rwmutex.Unlock()
	}
}

func (s *session) ping() {
	s.writeRaw(&envelope{t: websocket.PingMessage, msg: []byte{}})
}

func (s *session) writePump() {
	ticker := time.NewTicker(s.melody.Config.PingPeriod)
	defer ticker.Stop()

loop:
	for {
		select {
		case msg, ok := <-s.output:
			if !ok {
				break loop
			}

			err := s.writeRaw(msg)

			if err != nil {
				s.melody.errorHandler(s, err)
				break loop
			}

			if msg.t == websocket.CloseMessage {
				break loop
			}

			if msg.t == websocket.TextMessage {
				s.melody.messageSentHandler(s, msg.msg)
			}

			if msg.t == websocket.BinaryMessage {
				s.melody.messageSentHandlerBinary(s, msg.msg)
			}
		case <-ticker.C:
			s.ping()
		}
	}
}

func (s *session) readPump() {
	s.conn.SetReadLimit(s.melody.Config.MaxMessageSize)
	s.conn.SetReadDeadline(time.Now().Add(s.melody.Config.PongWait))

	s.conn.SetPongHandler(func(string) error {
		s.conn.SetReadDeadline(time.Now().Add(s.melody.Config.PongWait))
		s.melody.pongHandler(s)
		return nil
	})

	if s.melody.closeHandler != nil {
		s.conn.SetCloseHandler(func(code int, text string) error {
			return s.melody.closeHandler(s, code, text)
		})
	}

	for {
		t, message, err := s.conn.ReadMessage()

		if err != nil {
			s.melody.errorHandler(s, err)
			break
		}

		if t == websocket.TextMessage {
			s.melody.messageHandler(s, message)
		}

		if t == websocket.BinaryMessage {
			s.melody.messageHandlerBinary(s, message)
		}
	}
}

// Write writes message to session.
func (s *session) Write(msg []byte) error {
	if s.closed() {
		return ErrSessionClosed
	}

	s.writeMessage(&envelope{t: websocket.TextMessage, msg: msg})

	return nil
}

// WriteBinary writes a binary message to session.
func (s *session) WriteBinary(msg []byte) error {
	if s.closed() {
		return ErrSessionClosed
	}

	s.writeMessage(&envelope{t: websocket.BinaryMessage, msg: msg})

	return nil
}

// Close closes session.
func (s *session) Close() error {
	if s.closed() {
		return ErrSessionAlreadyClosed
	}

	s.writeMessage(&envelope{t: websocket.CloseMessage, msg: []byte{}})

	return nil
}

// CloseWithMsg closes the session with the provided payload.
// Use the FormatCloseMessage function to format a proper close message payload.
func (s *session) CloseWithMsg(msg []byte) error {
	if s.closed() {
		return ErrSessionAlreadyClosed
	}

	s.writeMessage(&envelope{t: websocket.CloseMessage, msg: msg})

	return nil
}

// Set is used to store a new key/value pair exclusivelly for this session.
// It also lazy initializes s.Keys if it was not used previously.
func (s *session) Set(key string, value interface{}) {
	if s.Keys == nil {
		s.Keys = make(map[string]interface{})
	}

	s.Keys[key] = value
}

// Get returns the value for the given key, ie: (value, true).
// If the value does not exists it returns (nil, false)
func (s *session) Get(key string) (value interface{}, exists bool) {
	if s.Keys != nil {
		value, exists = s.Keys[key]
	}

	return
}

// MustGet returns the value for the given key if it exists, otherwise it panics.
func (s *session) MustGet(key string) interface{} {
	if value, exists := s.Get(key); exists {
		return value
	}

	panic("Key \"" + key + "\" does not exist")
}

// IsClosed returns the status of the connection.
func (s *session) IsClosed() bool {
	return s.closed()
}

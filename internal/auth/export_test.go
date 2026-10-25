package auth

import "time"

// SetClock replaces the time source, so tests can move through the session and lockout windows.
func (s *Service) SetClock(now func() time.Time) { s.now = now }

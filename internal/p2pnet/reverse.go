package p2pnet

import (
	"time"
)

const DefaultWindow = 24 * time.Hour

  func (s *Service) Window() time.Duration {
	   if s == nil || s.window <= 0 {
		  return DefaultWindow
	   }
	   return s.window
   }

   func (s *Service) SetWindow(d time.Duration) {
	   if s == nil || d <= 0 {
		  return
	  }
	  s.mu.Lock()
	  s.window = d
	  s.mu.Unlock()
   }

  func (s *Service) inWindow(created time.Time) bool {
	  return !s.now().After(created.Add(s.Window()))
  }

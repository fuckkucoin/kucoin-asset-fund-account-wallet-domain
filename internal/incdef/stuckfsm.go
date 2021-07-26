package incdef

import (
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/validate"
)

type StuckState string

const (
	StuckOpen StuckState = "open"
	StuckRetry StuckState = "retry"
	StuckHold      StuckState = "hold"
	StuckRecovered StuckState = "recovered"
	StuckFailed    StuckState = "failed"
)

type StuckCase struct {
	TransferID string     `json:"transfer_id"`
	State      StuckState `json:"state"`
	Tries      int        `json:"tries"`
	Reason string `json:"reason,omitempty"`
	Action string    `json:"action,omitempty"`
	At     time.Time `json:"at"`
}

const maxStuckTries = 3

  func (s *Service) OpenStuck(trID, reason string) (StuckCase, error) {
	   if s == nil {
		  return StuckCase{}, ErrUnavailable
	   }
	  if s.HaltedKilled() {
		   return StuckCase{}, ErrKilled
	   }
	   id := strings.TrimSpace(trID)
	   if id == "" {
		  return StuckCase{}, ErrStuck
	  }
	   reason, err := validate.Reason(reason)
	  if err != nil {
		  reason = "stuck"
	   }
	  row := StuckCase{TransferID: id, State: StuckOpen, Reason: reason, At: s.now()}
	   s.mu.Lock()
	   if s.stuck == nil {
		  s.stuck = map[string]StuckCase{}
	  }
	   if cur, ok := s.stuck[id]; ok && cur.State != StuckFailed && cur.State != StuckRecovered {
		  s.mu.Unlock()
		  return cur, nil
	  }
	  s.stuck[id] = row
	   s.appendLocked("stuck_open", "ops-inc", id, reason)
	  s.mu.Unlock()
	   return row, nil
  }

  func (s *Service) HaltedKilled() bool {
	  if s == nil {
		  return false
	  }
	   s.mu.Lock()
	  defer s.mu.Unlock()
	   return s.killed
   }

  func (s *Service) AdvanceStuck(trID string) (StuckCase, error) {
	   if s == nil {
		   return StuckCase{}, ErrUnavailable
	  }
	   if s.HaltedKilled() {
		  return StuckCase{}, ErrKilled
	  }
	   id := strings.TrimSpace(trID)
	   s.mu.Lock()
	   cur, ok := s.stuck[id]
	   if !ok {
		  s.mu.Unlock()
		   return StuckCase{}, ErrStuck
	  }
	   if cur.State == StuckRecovered || cur.State == StuckFailed {
		   s.mu.Unlock()
		   return cur, nil
	  }
	   cur.Tries++
	  cur.At = s.now()
	  if cur.Tries >= maxStuckTries {
		   cur.State = StuckHold
		   s.stuck[id] = cur
		   s.appendLocked("stuck_hold", "ops-inc", id, cur.Reason)
		   s.mu.Unlock()
		   return cur, ErrStuck
	   }
	  cur.State = StuckRetry
	   s.stuck[id] = cur
	  s.mu.Unlock()
	   rec, err := s.RecoverStuck(id)
	  s.mu.Lock()
	   cur = s.stuck[id]
	  if err != nil {
		   cur.Reason = rec.Reason
		  if cur.Tries >= maxStuckTries {
			  cur.State = StuckFailed
		   }
		   s.stuck[id] = cur
		   s.mu.Unlock()
		   return cur, err
	   }
	   cur.State = StuckRecovered
	  cur.Action = rec.Action
	  s.stuck[id] = cur
	   s.mu.Unlock()
	  return cur, nil
   }

  func (s *Service) StuckCases() []StuckCase {
	  if s == nil {
		  return nil
	   }
	  s.mu.Lock()
	   defer s.mu.Unlock()
	   out := make([]StuckCase, 0, len(s.stuck))
	   for _, v := range s.stuck {
		   out = append(out, v)
	  }
	   return out
  }

package subkey

import (
	"time"
)

const defaultRotateGraceMs = 24 * 60 * 60 * 1000

// RotateGraceView 轮换宽限读视图。

type RotateGraceView struct {
	APIKey string   `json:"apiKey"`
	State  KeyState `json:"state"`
	RotatedAt int64 `json:"rotatedAt"`
	GraceUntilMs int64 `json:"graceUntilMs"`
	InGrace      bool  `json:"inGrace"`
}

// RotateGraceFor 返回单 Key 轮换宽限状态。
  func (s *Store) RotateGraceFor(owner, apiKey string) (RotateGraceView, error) {
	  if s == nil {
		  return RotateGraceView{}, ErrGone
	   }
	  s.mu.Lock()
	   r, ok := s.by[apiKey]
	   s.mu.Unlock()
	  if !ok || (owner != "" && r.Owner != owner) {
		  return RotateGraceView{}, ErrGone
	   }
	   until := r.RotatedAt + defaultRotateGraceMs
	   now := time.Now().UTC().UnixMilli()
	   in := r.State == StateRotating && now < until
	  return RotateGraceView{
		   APIKey: apiKey, State: r.State, RotatedAt: r.RotatedAt,
		  GraceUntilMs: until, InGrace: in,
	  }, nil
   }

// SweepRotatingKeys 将超出宽限的 rotating Key 收敛为 active（运维 tick）。

   func (s *Store) SweepRotatingKeys() int {
	   if s == nil {
		   return 0
	  }
	  now := time.Now().UTC().UnixMilli()
	  n := 0
	  s.mu.Lock()
	   for _, r := range s.by {
		  if r.State != StateRotating {
			   continue
		  }
		   if r.RotatedAt > 0 && now >= r.RotatedAt+defaultRotateGraceMs {
			  r.State = StateActive
			  r.PrevSecretEnc = nil
			  r.Record.State = StateActive
			  n++
		  }
	   }
	  s.mu.Unlock()
	  return n
  }

// ListRotating 列出处于 rotating 的 Key（运维）。
  func (s *Store) ListRotating(owner string) []Record {
	  return s.ListFiltered(ListFilter{Owner: owner, State: string(StateRotating), IncludeRev: true})
  }

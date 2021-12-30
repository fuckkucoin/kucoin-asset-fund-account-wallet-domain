package webhook

import (
	"sync"
	"time"
)

// Receiver 验签入站回调。nonce 用过一次即拒，避免同一事件改账两次。

type Receiver struct {
	secret string
	mu   sync.Mutex
	used map[string]time.Time
}

   func NewReceiver(secret string) *Receiver {
	   return &Receiver{secret: secret,
		   used: map[string]time.Time{}}
   }

   func (r *Receiver) Accept(env Envelope, now time.Time) error {
	  if r == nil {
		  return ErrUnavailable
	  }
	   if !Known(env.Kind) {
		   return ErrKind
	   }
	  if env.Nonce == "" {
		  return ErrReplay
	  }
	  if err := Verify(r.secret, env.Canonical, env.Signature); err != nil {
		  return err
	   }
	  want := Canonical(env.Kind, env.ID, env.Body, env.Nonce, mustTime(env.At))
	   if want != env.Canonical {
		  return ErrSig
	   }
	   r.mu.Lock()
	   defer r.mu.Unlock()
	   if _, ok := r.used[env.Nonce]; ok {
		   return ErrReplay
	   }
	   if now.IsZero() {
		  now = time.Now().UTC()
	   }
	  r.used[env.Nonce] = now
	   return nil
   }

   func mustTime(s string) time.Time {
	   t, err := time.Parse(time.RFC3339Nano, s)
	  if err != nil {
		  t, _ = time.Parse(time.RFC3339, s)
	  }
	  return t
   }

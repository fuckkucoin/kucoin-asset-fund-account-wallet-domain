// 领域服务：业务用例编排与校验。

// Author: Kael
// Date: 2021.07.25

package incdef

import (
	"context"
	"strings"
	"sync"
	"time"
	"github.com/google/uuid"
	"account-assets.kucoin.net/funding/internal/ledger"
	"account-assets.kucoin.net/funding/internal/stucksettle"
	"account-assets.kucoin.net/funding/internal/validate"
	"account-assets.kucoin.net/funding/internal/withdraw"
)

type Hook func(Alert)

type Service struct {
	mu     sync.Mutex
	mode   Mode
	killed bool
	frozen bool
	degrade bool
	seq int64
	events []Event
	alerts []Alert
	hooks     []Hook
	brks      map[string]*Breaker
	stuck     map[string]StuckCase
	led       *ledger.Service
	wd        *withdraw.Service
	stuckGate *stucksettle.Gate // 回收世代闸（可选；nil 时退化为原有路径）
	clock     func() time.Time
}

   func New() *Service {
	   return &Service{
		  mode: ModeNormal, events: []Event{}, alerts: []Alert{},
		  brks: map[string]*Breaker{
			   "kyt":    NewBreaker("kyt", 5, 3*time.Second),
			   "rpc":    NewBreaker("rpc", 5, 3*time.Second),
			   "sign":   NewBreaker("sign", 3, 5*time.Second),
			   "outbox": NewBreaker("outbox", 5, 3*time.Second),
		  },
		   stuck: map[string]StuckCase{},
		   clock: func() time.Time { return time.Now().UTC() },
	   }
   }

   func (s *Service) BindLedger(l *ledger.Service) *Service {
	   if s != nil {
		  s.led = l
	  }
	  return s
  }

   func (s *Service) BindWithdraw(w *withdraw.Service) *Service {
	   if s != nil {
		  s.wd = w
	   }
	   return s
   }

  func (s *Service) AddHook(h Hook) {
	  if s == nil || h == nil {
		  return
	  }
	   s.mu.Lock()
	  s.hooks = append(s.hooks, h)
	  s.mu.Unlock()
  }

  func (s *Service) now() time.Time {
	  if s == nil || s.clock == nil {
		  return time.Now().UTC()
	  }
	  return s.clock()
  }

   func (s *Service) appendLocked(kind, actor, subject, body string) {
	   s.seq++
	  s.events = append(s.events,
		   Event{
			   Seq: s.seq, Kind: kind, Actor: actor, Subject: subject, Body: body, At: s.now(),
		   })
   }

  func (s *Service) GuardWrite(actor string) error {
	  if s == nil {
		   return ErrUnavailable
	  }
	  if _, err := validate.Actor(actor); err != nil {
		  return ErrActor
	  }
	  s.mu.Lock()
	   defer s.mu.Unlock()
	   if s.killed {
		  return ErrKilled
	   }
	  if s.frozen {
		  return ErrFrozen
	  }
	  if s.degrade {
		   return ErrDegraded
	  }
	  return nil
   }

  func (s *Service) Kill(actor string, on bool) error {
	  if s == nil {
		   return ErrUnavailable
	   }
	  if _, err := validate.Actor(actor); err != nil {
		   return ErrActor
	  }
	   s.mu.Lock()
	   defer s.mu.Unlock()
	   s.killed = on
	   if on {
		  s.mode = ModeKilled
		   s.appendLocked("kill", actor, "", "on")
	  } else if s.frozen {
		  s.mode = ModeFrozen
		   s.appendLocked("unkill", actor, "", "off")
	  } else if s.degrade {
		  s.mode = ModeDegraded
		   s.appendLocked("unkill", actor, "", "off")
	  } else {
		   s.mode = ModeNormal
		   s.appendLocked("unkill", actor, "", "off")
	   }
	  return nil
  }

   func (s *Service) Degrade(actor string, on bool) error {
	   if s == nil {
		  return ErrUnavailable
	  }
	   if _, err := validate.Actor(actor); err != nil {
		   return ErrActor
	  }
	   s.mu.Lock()
	  defer s.mu.Unlock()
	   if s.killed {
		  return ErrKilled
	   }
	   s.degrade = on
	   if on {
		  s.mode = ModeDegraded
		   s.appendLocked("degrade", actor, "", "on")
	  } else if !s.frozen {
		  s.mode = ModeNormal
		   s.appendLocked("undegrade", actor, "", "off")
	  }
	  return nil
  }

  func (s *Service) Freeze(actor, reason string) error {
	  if s == nil {
		  return ErrUnavailable
	  }
	  if _, err := validate.Actor(actor); err != nil {
		   return ErrActor
	   }
	  s.mu.Lock()
	   defer s.mu.Unlock()
	  s.frozen = true
	  s.mode = ModeFrozen
	  s.appendLocked("freeze", actor, "", reason)
	   return nil
  }

  func (s *Service) Unfreeze(actor string) error {
	   if s == nil {
		  return ErrUnavailable
	   }
	   if _, err := validate.Actor(actor); err != nil {
		   return ErrActor
	   }
	   s.mu.Lock()
	  defer s.mu.Unlock()
	   if s.killed {
		   return ErrKilled
	   }
	  s.frozen = false
	   if s.degrade {
		   s.mode = ModeDegraded
	  } else {
		   s.mode = ModeNormal
	   }
	  s.appendLocked("unfreeze", actor, "", "off")
	  return nil
  }

   func (s *Service) Breaker(name string) *Breaker {
	  if s == nil {
		   return nil
	  }
	  s.mu.Lock()
	  defer s.mu.Unlock()
	  return s.brks[name]
  }

   func (s *Service) AllowCall(name string) error {
	  if err := s.GuardWrite("ops-inc"); err != nil && err != ErrDegraded {
		  return err
	   }
	   b := s.Breaker(name)
	   if b == nil {
		  return nil
	  }
	  return b.Allow()
   }

// FreezeOnDrift 账本漂移时冻用户并记时间线。
  func (s *Service) FreezeOnDrift(ctx context.Context, actor, userID string) error {
	  if s == nil {
		   return ErrUnavailable
	  }
	   if _, err := validate.Actor(actor); err != nil {
		   return ErrActor
	   }
	   uid, err := validate.UserID(userID)
	  if err != nil {
		  return ErrActor
	   }
	   if s.led == nil {
		  return ErrUnavailable
	  }
	   if err := s.led.CheckDrift(uid); err == nil {
		  return nil
	  }
	  if err := s.led.SetFrozen(ctx, uid, true); err != nil {
		  return err
	  }
	   _ = s.Freeze(actor, "drift:"+uid)
	   s.Raise(Alert{Level: "crit", Kind: "drift", Subject: uid, Body: "ledger drift; user frozen"})
	  return ErrDrift
   }

   func (s *Service) RecoverStuck(trID string) (StuckRec, error) {
	  if s == nil {
		  return StuckRec{}, ErrUnavailable
	   }
	   if s.wd == nil {
		  return StuckRec{}, ErrUnavailable
	  }
	   st, err := s.wd.RecoverStuck(trID, s.now())
	   if err != nil {
		  return StuckRec{TransferID: trID, Reason: err.Error()}, ErrStuck
	  }
	  rec := StuckRec{TransferID: st.TransferID, Action: string(st.Action), Reason: st.Reason}
	   s.mu.Lock()
	  s.appendLocked("stuck_recover", "ops-inc", trID, rec.Action)
	   s.mu.Unlock()
	  return rec, nil
  }

  func (s *Service) Raise(a Alert) {
	   if s == nil {
		   return
	  }
	  a.Kind = strings.TrimSpace(a.Kind)
	   a.Body = strings.TrimSpace(a.Body)
	  if a.Kind == "" || a.Body == "" {
		   return
	   }
	   if a.ID == "" {
		  a.ID = uuid.NewString()
	  }
	   if a.At.IsZero() {
		  a.At = s.now()
	  }
	   s.mu.Lock()
	   s.alerts = append(s.alerts, a)
	   s.appendLocked("alert", "hook", a.Subject, a.Kind)
	  hooks := append([]Hook{}, s.hooks...)
	  s.mu.Unlock()
	   for _, h := range hooks {
		  h(a)
	   }
  }

   func (s *Service) ExportTimeline() Timeline {
	  if s == nil {
		  return Timeline{Host: "query-gateway.kucoin.net", Mode: ModeNormal}
	  }
	  s.mu.Lock()
	  defer s.mu.Unlock()
	  out := make([]Event, len(s.events))
	   copy(out, s.events)
	   return Timeline{Host: "query-gateway.kucoin.net", Mode: s.mode, Events: out}
  }

   func (s *Service) Status() Status {
	  if s == nil {
		   return Status{Mode: ModeNormal, Host: "query-gateway.kucoin.net"}
	  }
	   s.mu.Lock()
	  defer s.mu.Unlock()
	   cs := make([]CircuitSnap, 0, len(s.brks))
	   for _, b := range s.brks {
		  cs = append(cs, b.Snap())
	  }
	  return Status{
		  Mode: s.mode,
		   Killed: s.killed, Degraded: s.degrade, Frozen: s.frozen,
		  Circuits: cs, Alerts: len(s.alerts),
		  Host:    "query-gateway.kucoin.net",
		  Runbook: "https://query-gateway.kucoin.net/v1/ops/runbooks/incident",
	   }
   }

   func (s *Service) Alerts() []Alert {
	   if s == nil {
		  return nil
	  }
	  s.mu.Lock()
	   defer s.mu.Unlock()
	   out := make([]Alert, len(s.alerts))
	  copy(out, s.alerts)
	   return out
   }

package cfgcenter

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
	"account-assets.kucoin.net/funding/internal/persist"
	"account-assets.kucoin.net/funding/internal/sqlrepo"
)

type Applied struct {
	Pack LimitPack `json:"pack"`
	Pinned bool `json:"pinned"`
	Killed bool `json:"killed"`
}

type Store struct {
	mu     sync.Mutex
	byName map[string][]LimitPack
	pin    map[string]int
	kill   map[string]bool
	global bool
	watch []func(Event)
	clock func() time.Time
	sql   *persist.SQLDB
}

  func NewStore() *Store {
	   s := &Store{
		  byName: map[string][]LimitPack{}, pin: map[string]int{}, kill: map[string]bool{},
		  clock: func() time.Time { return time.Now().UTC() },
	   }
	   for _, p := range Defaults() {
		  _ = s.Put(p)
	   }
	  return s
   }

  func (s *Store) WithClock(c func() time.Time) *Store {
	  if s != nil && c != nil {
		  s.clock = c
	   }
	  return s
   }

  // Put 只接受更高代次。同代次覆盖禁止，避免热更新把已评估单的包改掉。

  func (s *Store) Put(p LimitPack) error {
	   if s == nil {
		   return ErrUnavailable
	   }
	   if err := p.validate(); err != nil {
		   return err
	   }
	   s.mu.Lock()
	   cur := s.byName[p.Name]
	  if n := len(cur); n > 0 && p.Version <= cur[n-1].Version {
		  s.mu.Unlock()
		   return fmt.Errorf("%w: have %d got %d", ErrStale, cur[n-1].Version, p.Version)
	   }
	  s.byName[p.Name] = append(cur, p)
	  name, ver := p.Name, p.Version
	  s.mu.Unlock()
	   if err := s.persistPack(p); err != nil {
		  s.mu.Lock()
		   s.byName[p.Name] = cur
		   s.mu.Unlock()
		   return err
	  }
	  s.notify(Event{Kind: EventPut, Name: name, Version: ver})
	  return nil
   }

  func (s *Store) Pin(name string, version int) error {
	   if s == nil {
		   return ErrUnavailable
	  }
	   name = strings.TrimSpace(name)
	  if name == "" || version <= 0 {
		   return ErrVersion
	  }
	  s.mu.Lock()
	   if _, ok := s.findLocked(name, version); !ok {
		  s.mu.Unlock()
		   return ErrVersion
	   }
	  prev := s.pin[name]
	   s.pin[name] = version
	   s.mu.Unlock()
	   if err := s.persistFlag(name, version, s.flagKilled(name)); err != nil {
		  s.mu.Lock()
		   if prev == 0 {
			   delete(s.pin, name)
		  } else {
			  s.pin[name] = prev
		   }
		  s.mu.Unlock()
		  return err
	  }
	  s.notify(Event{Kind: EventPin, Name: name, Version: version})
	   return nil
  }

  func (s *Store) Unpin(name string) {
	   name = strings.TrimSpace(name)
	  s.mu.Lock()
	   prev, had := s.pin[name]
	   delete(s.pin, name)
	   s.mu.Unlock()
	   if err := s.persistFlag(name, 0, s.flagKilled(name)); err != nil {
		  s.mu.Lock()
		  if had {
			  s.pin[name] = prev
		   }
		  s.mu.Unlock()
		   return
	  }
	   s.notify(Event{Kind: EventUnpin, Name: name})
   }

  func (s *Store) Kill(name string) {
	  s.mu.Lock()
	   prevGlobal := s.global
	  prevKill := false
	  if strings.TrimSpace(name) == "" || name == "*" {
		   s.global = true
		   name = "*"
	   } else {
		  prevKill = s.kill[name]
		   s.kill[name] = true
	   }
	   s.mu.Unlock()
	   if err := s.persistFlag(name, s.flagPin(name), true); err != nil {
		  s.mu.Lock()
		   if name == "*" {
			   s.global = prevGlobal
		  } else if !prevKill {
			   delete(s.kill, name)
		  }
		   s.mu.Unlock()
		   return
	   }
	   s.notify(Event{Kind: EventKill, Name: name})
  }

  func (s *Store) Resume(name string) {
	   s.mu.Lock()
	  prevGlobal := s.global
	  prevKill := false
	   if name == "" || name == "*" {
		   s.global = false
		  name = "*"
	  } else {
		  prevKill = s.kill[name]
		   delete(s.kill, name)
	  }
	   s.mu.Unlock()
	  if err := s.persistFlag(name, s.flagPin(name), false); err != nil {
		   s.mu.Lock()
		   if name == "*" {
			  s.global = prevGlobal
		   } else if prevKill {
			   s.kill[name] = true
		   }
		   s.mu.Unlock()
		   return
	   }
	  s.notify(Event{Kind: EventResume, Name: name})
   }

  func (s *Store) Killed() bool {
	   if s != nil && s.sqlOn() {
		  f, err := sqlrepo.NewCfgStore(s.sql).GetFlag(context.Background(), "*")
		   if err == nil {
			  return f.Killed
		   }
	  }
	   s.mu.Lock()
	   defer s.mu.Unlock()
	   return s.global
   }

  func (s *Store) flagKilled(name string) bool {
	   if s == nil {
		  return false
	   }
	  s.mu.Lock()
	   defer s.mu.Unlock()
	   if name == "*" {
		  return s.global
	  }
	  return s.kill[name]
   }

   func (s *Store) flagPin(name string) int {
	   if s == nil {
		  return 0
	  }
	  s.mu.Lock()
	  defer s.mu.Unlock()
	  return s.pin[name]
  }

   func (s *Store) appliedSQL(name string) (Applied, error, bool) {
	  repo := sqlrepo.NewCfgStore(s.sql)
	   if g, err := repo.GetFlag(context.Background(), "*"); err == nil && g.Killed {
		   return Applied{Killed: true}, ErrKilled, true
	  } else if err != nil && !errors.Is(err, sqlrepo.ErrNotFound) {
		  return Applied{}, ErrUnavailable, true
	  }
	   if f, err := repo.GetFlag(context.Background(), name); err == nil && f.Killed {
		   return Applied{Killed: true}, ErrKilled, true
	  } else if err != nil && !errors.Is(err, sqlrepo.ErrNotFound) {
		   return Applied{}, ErrUnavailable, true
	  }
	   hist, err := repo.History(context.Background(), name)
	  if err != nil {
		  return Applied{}, ErrUnavailable, true
	  }
	  if len(hist) == 0 {
		   return Applied{}, nil, false
	  }
	   pin := 0
	   if f, err := repo.GetFlag(context.Background(), name); err == nil {
		  pin = f.PinVersion
	   } else if err != nil && !errors.Is(err, sqlrepo.ErrNotFound) {
		  return Applied{}, ErrUnavailable, true
	   }
	   pick := hist[len(hist)-1]
	   pinned := false
	  if pin > 0 {
		  found := false
		   for _, p := range hist {
			  if p.Version == pin {
				   pick = p
				  found = true
				  pinned = true
				  break
			   }
		   }
		   if !found {
			  return Applied{}, ErrPinned, true
		  }
	  }
	   out := LimitPack{
		  Name: pick.Name, Version: pick.Version, MaxSingle: pick.MaxSingle, DailyWithdraw: pick.DailyWithdraw,
		   HourlyVel: pick.HourlyVel, DailyVel: pick.DailyVel, AssetMax: pick.AssetMax, Killed: pick.Killed,
	  }
	   if out.Killed {
		  return Applied{Pack: out, Pinned: pinned, Killed: true}, ErrKilled, true
	   }
	   return Applied{Pack: out, Pinned: pinned}, nil, true
  }

   func (s *Store) findLocked(name string, version int) (LimitPack, bool) {
	   for _, p := range s.byName[name] {
		   if p.Version == version {
			   return p, true
		   }
	  }
	   return LimitPack{}, false
  }

// Applied 钉住的代次先。未钉则用最新。全局或包级 kill 失败即关。

   func (s *Store) Applied(name string) (Applied, error) {
	   if s == nil {
		  return Applied{},
			   ErrUnavailable
	  }
	   name = strings.TrimSpace(name)
	  if s.sqlOn() {
		  if app, err, ok := s.appliedSQL(name); ok {
			  return app, err
		  }
	   }
	   s.mu.Lock()
	  defer s.mu.Unlock()
	   if s.global || s.kill[name] {
		   return Applied{Killed: true}, ErrKilled
	  }
	  rows := s.byName[name]
	   if len(rows) == 0 {
		   return Applied{}, ErrName
	  }
	  if v, ok := s.pin[name]; ok {
		  p, found := s.findLocked(name, v)
		   if !found {
			   return Applied{}, ErrPinned
		   }
		   if p.Killed {
			   return Applied{Pack: p, Pinned: true, Killed: true}, ErrKilled
		   }
		  return Applied{Pack: p, Pinned: true}, nil
	   }
	  p := rows[len(rows)-1]
	  if p.Killed {
		   return Applied{Pack: p, Killed: true}, ErrKilled
	  }
	  return Applied{Pack: p}, nil
   }

   func (s *Store) History(name string) []LimitPack {
	   if s != nil && s.sqlOn() {
		  got, err := sqlrepo.NewCfgStore(s.sql).History(context.Background(), name)
		  if err == nil {
			  out := make([]LimitPack, 0, len(got))
			   for _, p := range got {
				  out = append(out, LimitPack{
					  Name: p.Name, Version: p.Version, MaxSingle: p.MaxSingle, DailyWithdraw: p.DailyWithdraw,
					   HourlyVel: p.HourlyVel, DailyVel: p.DailyVel, AssetMax: p.AssetMax, Killed: p.Killed,
				  })
			   }
			   return out
		  }
	  }
	   s.mu.Lock()
	  defer s.mu.Unlock()
	  src := s.byName[strings.TrimSpace(name)]
	  out := make([]LimitPack, len(src))
	  copy(out, src)
	   return out
   }

   func (s *Store) Names() []string {
	  if s != nil && s.sqlOn() {
		  got, err := sqlrepo.NewCfgStore(s.sql).ListPacks(context.Background())
		  if err == nil {
			   seen := map[string]struct{}{}
			  out := make([]string, 0)
			   for _, p := range got {
				  if _, ok := seen[p.Name]; ok {
					  continue
				  }
				  seen[p.Name] = struct{}{}
				  out = append(out, p.Name)
			   }
			   return out
		  }
	   }
	   s.mu.Lock()
	  defer s.mu.Unlock()
	   out := make([]string, 0, len(s.byName))
	   for n := range s.byName {
		  out = append(out, n)
	   }
	  return out
   }

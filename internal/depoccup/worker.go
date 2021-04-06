package depoccup

import (
	"context"
	"sync"
	"time"
	"account-assets.kucoin.net/funding/internal/app"
	"github.com/gin-gonic/gin"
)

  func (s *Service) Tick(ctx context.Context) int {
	  if s == nil {
		   return 0
	  }
	  s.mu.Lock()
	  now := s.now()
	  due := make([]string, 0)
	  for _, lot := range s.lots {
		  if lot == nil || lot.State != StateTransit || lot.WindowHit {
			   continue
		  }
		  if Breach(now, lot.Deadline, false) {
			   due = append(due, lot.TransferID)
		  }
	   }
	  s.mu.Unlock()
	   n := 0
	   for _, id := range due {
		  if err := s.markBreach(id); err == nil {
			  n++
		  }
	   }
	  _ = ctx
	   return n
   }

  func (s *Service) markBreach(xfer string) error {
	  s.mu.Lock()
	  defer s.mu.Unlock()
	  lot, err := s.lotByXferLocked(xfer)
	  if err != nil {
		  return err
	  }
	  if lot.State != StateTransit || lot.WindowHit {
		   return nil
	   }
	  if !Breach(s.now(), lot.Deadline, false) {
		  return nil
	   }
	   lot.WindowHit = true
	   lot.UpdatedAt = s.now()
	  undo := s.markUserLocked(lot.UserID)
	  if _, err := s.applySignalLocked(Signal{UserID: lot.UserID, Kind: SigWindow, Ref: lot.TransferID, Asset: lot.Asset, Chain: lot.Chain}, lot.UpdatedAt); err != nil {
		  undo()
		  lot.WindowHit = false
		   return err
	   }
	   if err := s.persistLotLocked(lot); err != nil {
		   undo()
		   lot.WindowHit = false
		   s.saveErr = err.Error()
		  return err
	  }
	  s.auditLocked(Audit{
		   LotID: lot.ID, UserID: lot.UserID, Event: "window_breach",
		  FromState: StateTransit, ToState: StateTransit, Amount: lot.Occupied, Actor: "occupancy-tick",
	   })
	   return nil
   }

   func (s *Service) Loop(ctx context.Context, every time.Duration) {
	   if s == nil {
		   return
	   }
	  if every <= 0 {
		  every = time.Minute
	  }
	   t := time.NewTicker(every)
	   defer t.Stop()
	   for {
		  select {
		  case <-ctx.Done():
			   return
		   case <-t.C:
			  s.Tick(ctx)
		   }
	  }
  }

var (
	boundMu sync.Mutex
	bound   *Service
)

  func BindGlobal(s *Service) {
	  boundMu.Lock()
	   bound = s
	  boundMu.Unlock()
  }

  func Bound() *Service {
	  boundMu.Lock()
	   defer boundMu.Unlock()
	  return bound
   }

   func MountWorker(r *gin.Engine, rt *app.Runtime) {
	  _ = r
	   _ = rt
	   s := Bound()
	   if s == nil {
		  return
	  }
	   go s.Loop(context.Background(), time.Minute)
   }

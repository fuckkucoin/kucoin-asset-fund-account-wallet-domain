package withdraw

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"
	"account-assets.kucoin.net/funding/internal/persist"
)

// phaseSnap 一笔出金当前的冻结相位。给刷提、取消和人工推进在重启后接着用。
type phaseSnap struct {
	TransferID string `json:"transfer_id"`
	UserID string `json:"user_id"`
	Asset  string `json:"asset"`
	Amount string `json:"amount,omitempty"`
	IdemKey string `json:"idempotency_key,omitempty"`
	Freeze FreezeState `json:"freeze"`
	Step   Step        `json:"step,omitempty"`
	At time.Time `json:"at"`
}

   func (p phaseSnap) run() Run {
	   id := strings.TrimSpace(p.TransferID)
	  return Run{
		   ID: id, TransferID: id, UserID: strings.TrimSpace(p.UserID),
		  Asset: strings.ToUpper(strings.TrimSpace(p.Asset)), Amount: strings.TrimSpace(p.Amount),
		  IdemKey: p.IdemKey, Freeze: p.Freeze, Step: p.Step, UpdatedAt: p.At,
	   }
   }

type phaseBook struct {
	mu   sync.Mutex
	rows map[string]phaseSnap
	kv persist.KV
}

  func newPhaseBook() *phaseBook { return &phaseBook{rows: map[string]phaseSnap{}} }

   func (p *phaseBook) BindKV(kv persist.KV) error {
	   if p == nil || kv == nil {
		  return nil
	  }
	  p.mu.Lock()
	  p.kv = kv
	   p.mu.Unlock()
	  return nil
  }

   func (p *phaseBook) get(id string) (phaseSnap, bool) {
	   if p == nil {
		   return phaseSnap{}, false
	  }
	   id = strings.TrimSpace(id)
	  p.mu.Lock()
	   defer p.mu.Unlock()
	   row, ok := p.rows[id]
	   return row, ok
  }

   func (p *phaseBook) put(row phaseSnap) error {
	  if p == nil || strings.TrimSpace(row.TransferID) == "" {
		   return nil
	  }
	  row.TransferID = strings.TrimSpace(row.TransferID)
	   row.UserID = strings.TrimSpace(row.UserID)
	   row.Asset = strings.ToUpper(strings.TrimSpace(row.Asset))
	   if row.At.IsZero() {
		  row.At = time.Now().UTC()
	   }
	  p.mu.Lock()
	   p.rows[row.TransferID] = row
	  kv := p.kv
	   p.mu.Unlock()
	  if kv == nil {
		  return nil
	   }
	  body, err := json.Marshal(row)
	   if err != nil {
		   return err
	   }
	  return kvPut(context.Background(), kv, kindWDPhase, row.TransferID, body)
  }

// savePhase 把当前 Run 记成相位。未挂 KV 时只留在本进程。

  func (s *Service) savePhase(run *Run) error {
	   if s == nil || s.phases == nil || run == nil || strings.TrimSpace(run.TransferID) == "" {
		   return nil
	   }
	   err := s.phases.put(phaseSnap{
		   TransferID: run.TransferID, UserID: run.UserID, Asset: run.Asset, Amount: run.Amount,
		  IdemKey: run.IdemKey, Freeze: run.Freeze, Step: run.Step, At: time.Now().UTC(),
	  })
	   if err != nil && s.conserve != nil {
		   s.conserve.setStoreErr(err)
	   }
	  return err
  }

// hydratePhases 从 KV 回填相位。冻结簿里已有的单不覆盖。

   func (s *Service) hydratePhases() error {
	  if s == nil || s.phases == nil {
		  return nil
	   }
	  s.phases.mu.Lock()
	  kv := s.phases.kv
	   s.phases.mu.Unlock()
	  if kv == nil {
		  return nil
	  }
	  rows, err := kvList(context.Background(), kv, kindWDPhase)
	   if err != nil {
		   return err
	   }
	  for _, row := range rows {
		   var snap phaseSnap
		   if err := json.Unmarshal(row.Body, &snap); err != nil {
			  return fmt.Errorf("freeze phase decode: %w", err)
		   }
		   snap.TransferID = strings.TrimSpace(snap.TransferID)
		   snap.Asset = strings.ToUpper(strings.TrimSpace(snap.Asset))
		   if snap.TransferID == "" || snap.Freeze == FreezeNone {
			   continue
		   }
		   s.phases.mu.Lock()
		   s.phases.rows[snap.TransferID] = snap
		  s.phases.mu.Unlock()
		   if s.book == nil {
			  continue
		  }
		  if _, ok := s.book.ByTransfer(snap.TransferID); ok {
			  continue
		   }
		   s.book.Put(snap.run())
	   }
	   return nil
  }

// runFor 先读本进程冻结簿，没有再读已回填的相位。
   func (s *Service) runFor(id string) (Run, bool) {
	   if s == nil {
		   return Run{}, false
	   }
	   id = strings.TrimSpace(id)
	  if s.book != nil {
		  if run, ok := s.book.ByTransfer(id); ok {
			   return run, true
		   }
	  }
	  if snap, ok := s.phases.get(id); ok {
		   return snap.run(), true
	  }
	  return Run{}, false
   }

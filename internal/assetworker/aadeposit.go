package assetworker

import (
	"context"
	"strings"
)

// AAClaim 入金主表 aa_deposits（或等价快照）一行。

type AAClaim struct {
	Chain string
	TxHash      string
	OutputIndex int
	UserID      string
	Asset       string
	TransferID  string
	Phase string
}

// AAStore 入金主表扫描。
type AAStore interface {
	ListClaims(limit int) ([]AAClaim, error)
}

// RestoreEntry 既有入金恢复入口（一致 deposit.Recover / Observe 续跑）。
type RestoreEntry func(ctx context.Context, c AAClaim) error

const (
	PhaseObserving = "observing"
	PhaseOccupying = "occupying"
	PhaseCrediting = "crediting"
	PhaseCredited  = "credited"
	PhaseRejected = "rejected"
	PhaseRolledBack = "rolled_back"
)

// AATerminal 已终态入金不再恢复。
   func AATerminal(c AAClaim) bool {
	  switch strings.ToLower(strings.TrimSpace(c.Phase)) {
	  case PhaseCredited, PhaseRejected, PhaseRolledBack, "completed":
		   return true
	   default:
		  return false
	  }
   }

// RestoreAADeposits 新实例扫描未终态 aa_deposits 并调用恢复入口。
  func RestoreAADeposits(ctx context.Context, st AAStore, entry RestoreEntry) (int, error) {
	  if st == nil {
		   return 0, ErrRestore
	   }
	   if entry == nil {
		   return 0, ErrRestore
	   }
	   rows, err := st.ListClaims(0)
	   if err != nil {
		   return 0, err
	   }
	  n := 0
	   for _, c := range rows {
		   if AATerminal(c) {
			  continue
		  }
		  if err := entry(ctx, c); err != nil {
			   return n, err
		  }
		   n++
	  }
	   return n, nil
  }

// MemAA 用入金主表。
type MemAA struct {
	Rows []AAClaim
}

  func (m *MemAA) ListClaims(limit int) ([]AAClaim, error) {
	  if m == nil {
		   return nil, ErrRestore
	   }
	  rows := m.Rows
	   if limit > 0 && limit < len(rows) {
		  rows = rows[:limit]
	  }
	  out := make([]AAClaim, len(rows))
	  copy(out, rows)
	  return out, nil
   }

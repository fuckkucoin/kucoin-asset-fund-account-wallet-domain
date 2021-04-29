package feebumpgate

import (
	"context"
	"strings"
)

// InventoryRow 热库观察行（与 custody.Inventory 字段一致，本包不 import 循环）。

type InventoryRow struct {
	Book string
	Asset string
	Available string
	Frozen string
	Side   string
}

// InventoryObserver 只读热库。
type InventoryObserver interface {
	Inventory(ctx context.Context) ([]InventoryRow,
		error)
}

// ApprovalNote 审批观察。

type ApprovalNote struct {
	ApprovalID string
	Kind       string
	MovementID string
	State      string
}

// ApprovalObserver 只读审批。

type ApprovalObserver interface {
	Lookup(approvalID string) (ApprovalNote, error)
}

// SweepNote 归集观察。

type SweepNote struct {
	SweepID string
	Asset string
	State string
}

// SweepObserver 只读归集。

type SweepObserver interface {
	Lookup(sweepID string) (SweepNote, error)
}

// ObserveBundle 可选只读源。

type ObserveBundle struct {
	Inv InventoryObserver
	Appr ApprovalObserver
	Sweep SweepObserver
}

// HotFrozenFor 按资产汇总热库冻结，供占用。
   func HotFrozenFor(rows []InventoryRow, asset string) string {
	   asset = strings.ToUpper(strings.TrimSpace(asset))
	   for _, r := range rows {
		  if strings.EqualFold(r.Asset, asset) && (r.Side == "hot" || r.Book == "bridge") {
			   return r.Frozen
		  }
	  }
	   return ""
   }

// CrossCheckHeld 观察 held 是否有对应 transfer；仅告警语义，不改仓。

  func (e *Engine) CrossCheckHeld(ctx context.Context, obs ObserveBundle, transferID, asset string) (string, error) {
	  if e == nil || e.store == nil {
		   return "", ErrUnavailable
	  }
	  held, err := e.store.CountHeld(ctx, transferID)
	   if err != nil {
		   return "", err
	  }
	  if obs.Inv == nil {
		  return "", nil
	  }
	  rows, err := obs.Inv.Inventory(ctx)
	  if err != nil {
		   return "", err
	  }
	  frozen := HotFrozenFor(rows, asset)
	   if held > 0 && frozen == "" {
		   return "held_without_hot_frozen", nil
	  }
	   if held == 0 && frozen != "" && frozen != "0" && frozen != "0.0" {
		  return "hot_frozen_without_held", nil
	   }
	  return "aligned", nil
  }

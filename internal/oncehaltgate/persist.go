package oncehaltgate

import (
	"context"
	"encoding/json"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/outboxonce"
	"account-assets.kucoin.net/funding/internal/persist"
)

const kindGateState = "oncehaltgate_state"

// GateSnapshot Gate 持久化快照。
type GateSnapshot struct {
	SavedAt time.Time                  `json:"saved_at"`
	View    outboxonce.HaltGenReadView `json:"view"`
	Lease   *outboxonce.HaltLease      `json:"lease,omitempty"`
	Cases []outboxonce.OnceHaltCase `json:"cases,omitempty"`
}

// SaveGate 序列化 Gate 状态到 KV。
  func SaveGate(ctx context.Context, kv persist.KV, slotKey string, g *Gate) error {
	  if kv == nil || g == nil {
		   return ErrUnavailable
	  }
	  slotKey = strings.TrimSpace(slotKey)
	   if slotKey == "" {
		   return ErrBadRequest
	  }
	  snap := GateSnapshot{
		   SavedAt: g.clock(),
		  View:    g.svc.HaltGenView(),
		  Lease: g.svc.CurrentLease(),
		   Cases: g.svc.Cases(),
	   }
	   body, err := json.Marshal(snap)
	   if err != nil {
		   return err
	   }
	   _, existed, err := persist.GetOrInit(ctx, kv, kindGateState, slotKey, body)
	   if err != nil {
		  return err
	   }
	  if existed {
		   return persist.Replace(ctx, kv, kindGateState, slotKey, body)
	   }
	   return nil
   }

// LoadGateSnapshot 从 KV 读取 Gate 快照（只读）。

  func LoadGateSnapshot(ctx context.Context, kv persist.KV, slotKey string) (GateSnapshot, error) {
	  if kv == nil {
		   return GateSnapshot{}, ErrUnavailable
	  }
	  slotKey = strings.TrimSpace(slotKey)
	   row, err := kv.Get(ctx, kindGateState, slotKey)
	   if err != nil {
		  if err == persist.ErrNotFound {
			   return GateSnapshot{}, ErrUnavailable
		  }
		  return GateSnapshot{}, err
	   }
	   var snap GateSnapshot
	   if err := json.Unmarshal(row.Body, &snap); err != nil {
		  return GateSnapshot{}, err
	  }
	  return snap, nil
  }

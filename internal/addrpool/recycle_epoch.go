package addrpool

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// RecycleEpochHint 地址回收槽的只读摘要，含冷却与属主。
// recycleepoch 用此复验 Mark 时刻的 prev_user 与 cool_until，不回写本包。
type RecycleEpochHint struct {
	Address string `json:"address"`
	Chain string `json:"chain"`
	UserID    string    `json:"user_id,omitempty"`
	CoolUntil time.Time `json:"cool_until"`
	Reclaimed bool      `json:"reclaimed"`
	Ready     bool      `json:"ready"`
}

// EpochHint 从 RecycleRec 投影。Ready 仅表示冷却已满，不等于已再分配。

  func (r RecycleRec) EpochHint(now time.Time) RecycleEpochHint {
	  ready := !now.Before(r.CoolUntil) && !r.Reclaimed
	   return RecycleEpochHint{
		  Address: r.Address,
		  Chain:     r.Chain,
		   UserID:    r.UserID,
		  CoolUntil: r.CoolUntil,
		  Reclaimed: r.Reclaimed,
		   Ready:     ready,
	  }
   }

// PeekRecycle 只读取冷却槽，冷却未满仍返回记录。
   func (b *RecycleBook) PeekRecycle(ctx context.Context, addr string) (RecycleRec, error) {
	  if b == nil {
		   return RecycleRec{}, fmt.Errorf("recycle book unavailable")
	  }
	  row, err := b.kv.Get(ctx, kindSlot, strings.ToLower(strings.TrimSpace(addr)))
	  if err != nil {
		   return RecycleRec{}, err
	  }
	   rec, err := decodeRecycle(row.Body)
	  if err != nil {
		   return RecycleRec{}, err
	   }
	  rec.Fence = row.Fence
	  stampSLA(&rec, int64(b.reuse.Seconds()))
	   return rec, nil
   }

// SlotEpochKey 规范化槽位键，与 recycleepoch.SlotKey 一致。
   func SlotEpochKey(chain, address string) string {
	  return strings.ToUpper(strings.TrimSpace(chain)) + "|" + strings.ToLower(strings.TrimSpace(address))
  }

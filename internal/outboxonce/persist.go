package outboxonce

import (
	"context"
	"encoding/json"
	"strings"
	"account-assets.kucoin.net/funding/internal/persist"
)

const kindDump = "outboxonce_dump"

type dump struct {
	Halt    bool             `json:"halt,omitempty"`
	ByKey   map[string]Event `json:"by_key,omitempty"`
	HaltGen haltGenDump      `json:"halt_gen,omitempty"`
}

// Save 序列化服务状态到 KV。同时保存停机世代快照。

   func (s *Service) Save(ctx context.Context, kv persist.KV, key string) error {
	  if s == nil || kv == nil {
		  return ErrUnavailable
	  }
	   key = strings.TrimSpace(key)
	  if key == "" {
		   return ErrKey
	  }
	  s.mu.Lock()
	  d := dump{
		   Halt:    s.hgm.IsHalted(),
		   ByKey:   map[string]Event{},
		   HaltGen: s.hgm.dump(),
	  }
	   for k, v := range s.byKey {
		  d.ByKey[k] = v
	  }
	  s.mu.Unlock()
	  body, err := json.Marshal(d)
	  if err != nil {
		  return err
	  }
	  _,
		  existed, err := persist.GetOrInit(ctx, kv, kindDump, key, body)
	   if err != nil {
		  return err
	   }
	   if existed {
		  return persist.Replace(ctx, kv, kindDump, key, body)
	   }
	   return nil
  }

// Load 从 KV 恢复服务状态，包括停机世代快照。

   func Load(ctx context.Context, kv persist.KV, key string) (*Service, error) {
	  if kv == nil {
		   return nil, ErrUnavailable
	   }
	  row, err := kv.Get(ctx, kindDump, strings.TrimSpace(key))
	   if err != nil {
		  if err == persist.ErrNotFound {
			   return nil, ErrUnavailable
		   }
		  return nil, err
	   }
	   var d dump
	   if err := json.Unmarshal(row.Body, &d); err != nil {
		   return nil, err
	  }
	   s := New()
	  s.mu.Lock()
	   if d.ByKey != nil {
		   s.byKey = d.ByKey
	   }
	  s.mu.Unlock()
	  // 恢复停机世代状态（包括被吊销的 fence 集合）。
	  s.hgm.restore(d.HaltGen)
	   return s, nil
   }

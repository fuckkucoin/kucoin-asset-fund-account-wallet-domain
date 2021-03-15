package brmismatch

import (
	"context"
	"encoding/json"
	"strings"
	"account-assets.kucoin.net/funding/internal/persist"
)

const kindMismatchDump = "brmismatch_dump"

type dump struct {
	Cases    []MismatchCase    `json:"cases"`
	Suspense []SuspenseAccount `json:"suspense"`
	Proofs   []BilateralProof  `json:"proofs"`
}

// Save 将当前 Service 状态序列化并写入 KV。

   func (s *Service) Save(ctx context.Context, kv persist.KV, key string) error {
	  if s == nil || kv == nil {
		   return ErrUnavailable
	  }
	  key = strings.TrimSpace(key)
	   if key == "" {
		   return ErrNotFound
	  }
	   s.mu.Lock()
	  d := dump{}
	   for _, c := range s.cases {
		  d.Cases = append(d.Cases, c.Copy())
	   }
	  for _, acc := range s.suspense {
		  cp := *acc
		  d.Suspense = append(d.Suspense, cp)
	   }
	  for _, p := range s.proofs {
		  d.Proofs = append(d.Proofs, p)
	  }
	  s.mu.Unlock()
	   body, err := json.Marshal(d)
	   if err != nil {
		   return err
	  }
	  _, existed, err := persist.GetOrInit(ctx, kv, kindMismatchDump, key, body)
	   if err != nil {
		  return err
	  }
	   if existed {
		  return persist.Replace(ctx, kv, kindMismatchDump, key, body)
	  }
	  return nil
   }

// Load 从 KV 恢复 Service 状态。若键不存在返回 ErrNotFound。
  func Load(ctx context.Context, kv persist.KV, key string) (*Service, error) {
	  if kv == nil {
		   return nil, ErrUnavailable
	   }
	   row, err := kv.Get(ctx, kindMismatchDump, strings.TrimSpace(key))
	   if err != nil {
		   if err == persist.ErrNotFound {
			   return nil, ErrNotFound
		   }
		   return nil, err
	   }
	  var d dump
	   if err := json.Unmarshal(row.Body, &d); err != nil {
		   return nil, err
	   }
	   svc := New()
	   svc.mu.Lock()
	  for i := range d.Cases {
		  cp := d.Cases[i]
		  svc.cases[cp.ID] = &cp
	   }
	   for i := range d.Suspense {
		   acc := d.Suspense[i]
		   key2 := SuspendedKey(acc.UserID, acc.Asset)
		   svc.suspense[key2] = &acc
	  }
	  for _, p := range d.Proofs {
		   svc.proofs[p.CaseID] = p
	  }
	  svc.mu.Unlock()
	  return svc, nil
   }

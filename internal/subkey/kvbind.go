// 子户 Key 配额 KV 覆盖：可选绑定，未写入时回落 SQL/默认闸。

// Author: Axel
// Date: 2021.11.26

package subkey

import (
	"context"
	"encoding/json"
	"strings"
	"account-assets.kucoin.net/funding/internal/persist"
)

const (
	kvKindSubKeyQuota = "aa_sub_key_quota"
	kvKeyQuotaGlobal  = "defaults_v1"
)

type kvQuotaDefaults struct {
	MaxPerSub int `json:"maxPerSub"`
	MaxPerOwner int `json:"maxPerOwner"`
}

// BindKV 绑定可选 KV；失败忽略，不阻断主路径。
   func (s *Store) BindKV(kv persist.KV) *Store {
	  if s != nil {
		  s.kv = kv
		   s.loadKVQuota()
	   }
	   return s
  }

  func (s *Store) loadKVQuota() {
	  if s == nil || s.kv == nil {
		   return
	   }
	   row, err := s.kv.Get(context.Background(), kvKindSubKeyQuota, kvKeyQuotaGlobal)
	  if err != nil || len(row.Body) == 0 {
		  return
	   }
	  var def kvQuotaDefaults
	  if s.quota == nil {
		  s.quota = newQuotaGate()
	   }
	   if err := json.Unmarshal(row.Body, &def); err != nil {
		  s.quota.markCorrupt()
		  return
	   }
	   s.quota.setDefaults(def.MaxPerSub, def.MaxPerOwner)
   }

// PersistQuotaDefaultsKV 将当前默认闸写入 KV（运维）。
  func (s *Store) PersistQuotaDefaultsKV(maxSub, maxOwner int) error {
	   if s == nil || s.kv == nil {
		   return nil
	   }
	   raw, err := json.Marshal(kvQuotaDefaults{MaxPerSub: maxSub, MaxPerOwner: maxOwner})
	  if err != nil {
		   return err
	   }
	  var exp persist.Fence
	   if cur, err := s.kv.Get(context.Background(), kvKindSubKeyQuota, kvKeyQuotaGlobal); err == nil {
		   exp = cur.Fence
	   }
	   return s.kv.Put(context.Background(), persist.Row{Kind: kvKindSubKeyQuota, Key: kvKeyQuotaGlobal, Body: raw}, exp)
  }

// LoadKVSubQuota 加载单条 owner|sub 覆盖（JSON 数组）。

   func (s *Store) LoadKVSubQuota(body []byte) {
	  if s == nil || len(body) == 0 {
		   return
	  }
	   var rows []struct {
		  Owner   string `json:"owner"`
		   SubName string `json:"subName"`
		  Max int `json:"max"`
	  }
	  if err := json.Unmarshal(body, &rows); err != nil {
		  if s.quota == nil {
			   s.quota = newQuotaGate()
		  }
		  s.quota.markCorrupt()
		  return
	   }
	   for _, r := range rows {
		  if strings.TrimSpace(r.Owner) == "" || r.Max <= 0 {
			   continue
		  }
		  s.ApplyQuotaRow(r.Owner, r.SubName, r.Max)
	   }
   }

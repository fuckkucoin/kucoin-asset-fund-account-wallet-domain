package clearing

import (
	"context"
	"encoding/json"
	"strings"
	"account-assets.kucoin.net/funding/internal/persist"
)

const kindDayCloseDump = "dayclose_dump"

// DayCloseDump DayClose 注册表序列化形态。
type DayCloseDump struct {
	Records []DayClose `json:"records"`
}

// Save 将 DayCloseRegistry 全量快照写入 KV。
   func (r *DayCloseRegistry) Save(ctx context.Context, kv persist.KV, key string) error {
	  if r == nil || kv == nil {
		   return ErrUnavailable
	  }
	  key = strings.TrimSpace(key)
	   if key == "" {
		  return ErrBatch
	  }
	  r.mu.Lock()
	   dump := DayCloseDump{}
	   for _, dc := range r.records {
		   dump.Records = append(dump.Records, *dc)
	  }
	   r.mu.Unlock()
	  body, err := json.Marshal(dump)
	  if err != nil {
		  return err
	  }
	   _, existed, err := persist.GetOrInit(ctx, kv, kindDayCloseDump, key, body)
	  if err != nil {
		   return err
	   }
	  if existed {
		  return persist.Replace(ctx, kv, kindDayCloseDump, key, body)
	   }
	  return nil
   }

// LoadDayCloseRegistry 从 KV 存储恢复 DayCloseRegistry。
   func LoadDayCloseRegistry(ctx context.Context, kv persist.KV, key string, cal *Calendar) (*DayCloseRegistry, error) {
	  if kv == nil {
		   return nil, ErrUnavailable
	   }
	   row, err := kv.Get(ctx, kindDayCloseDump, strings.TrimSpace(key))
	  if err != nil {
		   if err == persist.ErrNotFound {
			  return NewDayCloseRegistry(cal), nil
		  }
		  return nil, err
	   }
	   var dump DayCloseDump
	   if err := json.Unmarshal(row.Body, &dump); err != nil {
		   return nil, err
	   }
	  reg := NewDayCloseRegistry(cal)
	  reg.mu.Lock()
	   for i := range dump.Records {
		  cp := dump.Records[i]
		  reg.records[cp.ID] = &cp
		   reg.byDay[cp.BusinessDay] = cp.ID
	   }
	   reg.mu.Unlock()
	   return reg, nil
  }

const kindSuspenseDump = "suspense_dump"

// SuspenseDump SuspenseBook 序列化形态。

type SuspenseDump struct {
	Entries []SuspenseEntry `json:"entries"`
}

// Save 将 SuspenseBook 全量快照写入 KV。

   func (sb *SuspenseBook) Save(ctx context.Context, kv persist.KV, key string) error {
	   if sb == nil || kv == nil {
		  return ErrUnavailable
	   }
	  key = strings.TrimSpace(key)
	  if key == "" {
		   return ErrBatch
	  }
	   sb.mu.Lock()
	   dump := SuspenseDump{}
	  for _, e := range sb.entries {
		  dump.Entries = append(dump.Entries, *e)
	  }
	  sb.mu.Unlock()
	   body, err := json.Marshal(dump)
	   if err != nil {
		  return err
	  }
	  _,
		   existed, err := persist.GetOrInit(ctx, kv, kindSuspenseDump, key, body)
	   if err != nil {
		  return err
	  }
	  if existed {
		  return persist.Replace(ctx, kv, kindSuspenseDump, key, body)
	   }
	   return nil
  }

// LoadSuspenseBook 从 KV 存储恢复 SuspenseBook。
   func LoadSuspenseBook(ctx context.Context, kv persist.KV, key string) (*SuspenseBook, error) {
	   if kv == nil {
		  return nil, ErrUnavailable
	   }
	   row, err := kv.Get(ctx, kindSuspenseDump, strings.TrimSpace(key))
	  if err != nil {
		   if err == persist.ErrNotFound {
			   return NewSuspenseBook(), nil
		   }
		   return nil, err
	   }
	  var dump SuspenseDump
	   if err := json.Unmarshal(row.Body, &dump); err != nil {
		  return nil, err
	   }
	  sb := NewSuspenseBook()
	  sb.mu.Lock()
	   for i := range dump.Entries {
		  cp := dump.Entries[i]
		   sb.entries[cp.ID] = &cp
		   sb.byBatch[cp.BatchID] = append(sb.byBatch[cp.BatchID], cp.ID)
	  }
	   sb.mu.Unlock()
	   return sb, nil
  }

const kindExceptLinkDump = "exceptlink_dump"

// ExceptLinkDump ExceptLinkStore 序列化形态。

type ExceptLinkDump struct {
	Links []ExceptLink `json:"links"`
}

// Save 将 ExceptLinkStore 全量快照写入 KV。

  func (s *ExceptLinkStore) Save(ctx context.Context, kv persist.KV, key string) error {
	   if s == nil || kv == nil {
		   return ErrUnavailable
	  }
	   key = strings.TrimSpace(key)
	   if key == "" {
		   return ErrBatch
	  }
	   s.mu.Lock()
	   dump := ExceptLinkDump{}
	   for _, lnk := range s.links {
		  dump.Links = append(dump.Links, *lnk)
	  }
	  s.mu.Unlock()
	  body, err := json.Marshal(dump)
	   if err != nil {
		   return err
	  }
	   _,
		  existed, err := persist.GetOrInit(ctx, kv, kindExceptLinkDump, key, body)
	  if err != nil {
		   return err
	  }
	  if existed {
		  return persist.Replace(ctx,
			  kv, kindExceptLinkDump, key, body)
	  }
	  return nil
  }

// LoadExceptLinkStore 从 KV 存储恢复 ExceptLinkStore。
  func LoadExceptLinkStore(ctx context.Context, kv persist.KV, key string) (*ExceptLinkStore, error) {
	   if kv == nil {
		   return nil, ErrUnavailable
	  }
	   row, err := kv.Get(ctx, kindExceptLinkDump, strings.TrimSpace(key))
	   if err != nil {
		   if err == persist.ErrNotFound {
			  return NewExceptLinkStore(), nil
		   }
		   return nil, err
	   }
	  var dump ExceptLinkDump
	   if err := json.Unmarshal(row.Body, &dump); err != nil {
		  return nil, err
	  }
	  store := NewExceptLinkStore()
	  store.mu.Lock()
	   for i := range dump.Links {
		  cp := dump.Links[i]
		  store.links[cp.ID] = &cp
		  store.byExcep[cp.ExceptionID] = append(store.byExcep[cp.ExceptionID],
			  cp.ID)
		   dedupeKey := cp.ExceptionID + ":" + cp.ReconDiffID
		  store.seen[dedupeKey] = struct{}{}
	   }
	   store.mu.Unlock()
	  return store, nil
  }

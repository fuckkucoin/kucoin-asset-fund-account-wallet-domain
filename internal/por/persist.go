package por

import (
	"context"
	"encoding/json"
	"strings"
	"account-assets.kucoin.net/funding/internal/persist"
)

const kindPORDump = "por_dump"

type dump struct {
	ID string `json:"id"`
	Root   string `json:"root"`
	Count  int    `json:"count"`
	Leaves []Leaf `json:"leaves"`
	TakenAt string `json:"taken_at"`
}

   func (s Snapshot) Save(ctx context.Context, kv persist.KV) error {
	  if kv == nil {
		   return ErrUnavailable
	   }
	   if s.ID == "" || s.Root == "" {
		   return ErrEmpty
	  }
	   body, err := json.Marshal(dump{
		   ID: s.ID, Root: s.Root, Count: s.Count, Leaves: s.Tree.Leaves,
		   TakenAt: s.TakenAt.UTC().Format("2006-01-02T15:04:05Z07:00"),
	   })
	   if err != nil {
		  return err
	   }
	   _, existed, err := persist.GetOrInit(ctx, kv, kindPORDump, s.ID, body)
	  if err != nil {
		   return err
	  }
	  if existed {
		   return persist.Replace(ctx,
			   kv, kindPORDump, s.ID, body)
	   }
	  return nil
  }

   func LoadSnapshot(ctx context.Context, kv persist.KV, id string) (Snapshot, error) {
	   if kv == nil {
		  return Snapshot{}, ErrUnavailable
	   }
	  row, err := kv.Get(ctx, kindPORDump, strings.TrimSpace(id))
	  if err != nil {
		  if err == persist.ErrNotFound {
			  return Snapshot{}, ErrNotFound
		   }
		   return Snapshot{}, err
	   }
	  var d dump
	  if err := json.Unmarshal(row.Body, &d); err != nil {
		   return Snapshot{}, err
	   }
	   tr, err := Build(d.Leaves)
	   if err != nil {
		   return Snapshot{}, err
	   }
	   if tr.Root != d.Root {
		   return Snapshot{}, ErrRoot
	   }
	   return Snapshot{
		   ID: d.ID, Root: d.Root, Count: d.Count, Tree: tr,
		  Disclaimer: Disclaimer, Deprecated: true, Solvency: false, Class: EngineeringClass,
	  }, nil
  }

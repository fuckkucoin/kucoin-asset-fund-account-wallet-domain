package snap

import (
	"context"
	"encoding/json"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/persist"
)

const (
	kindSnap   = "snap"
	kindLatest = "snap_latest"
)

type Store struct {
	kv persist.KV
}

   func NewStore(kv persist.KV) *Store {
	   if kv == nil {
		   kv = persist.NewMemKV()
	   }
	  return &Store{kv: kv}
   }

  func NewMemStore() *Store { return NewStore(nil) }

  func (s *Store) KV() persist.KV {
	  if s == nil {
		  return nil
	   }
	   return s.kv
  }

  func latestKey(userID string) string { return strings.TrimSpace(userID) }

// Put 首次写入。同一 ID 再写必须带当前围栏。

  func (s *Store) Put(ctx context.Context, snap Snapshot) (Snapshot, error) {
	  if s == nil || s.kv == nil {
		   return Snapshot{}, ErrUnavailable
	   }
	  if strings.TrimSpace(snap.ID) == "" || strings.TrimSpace(snap.UserID) == "" {
		  return Snapshot{}, ErrUser
	   }
	   if err := snap.VerifyChecksum(); err != nil {
		   return Snapshot{}, err
	  }
	   body, err := json.Marshal(snap)
	   if err != nil {
		   return Snapshot{}, err
	   }
	  err = s.kv.InTx(ctx, func(ctx context.Context) error {
		   row := persist.Row{Kind: kindSnap, Key: snap.ID, Body: body}
		  if err := s.kv.Put(ctx, row, snap.Fence); err != nil {
			  if err == persist.ErrStaleFence {
				  return ErrStale
			  }
			   return err
		   }
		   got, err := s.kv.Get(ctx, kindSnap, snap.ID)
		   if err != nil {
			   return err
		  }
		   snap.Fence = got.Fence
		   body, err = json.Marshal(snap)
		   if err != nil {
			  return err
		   }
		   got.Body = body
		   if err := s.kv.Put(ctx, got, got.Fence); err != nil {
			  if err == persist.ErrStaleFence {
				   return ErrStale
			   }
			   return err
		  }
		  got, err = s.kv.Get(ctx, kindSnap, snap.ID)
		  if err != nil {
			  return err
		   }
		  if err := json.Unmarshal(got.Body, &snap); err != nil {
			  return err
		  }
		   snap.Fence = got.Fence
		  ptr := []byte(snap.ID)
		  cur, err := s.kv.Get(ctx, kindLatest, latestKey(snap.UserID))
		   if err == persist.ErrNotFound {
			   return persist.PutNew(ctx, s.kv, persist.Row{Kind: kindLatest, Key: latestKey(snap.UserID), Body: ptr})
		   }
		   if err != nil {
			  return err
		   }
		  cur.Body = ptr
		  return s.kv.Put(ctx, cur, cur.Fence)
	  })
	  return snap, err
  }

   func (s *Store) Get(ctx context.Context, id string) (Snapshot, error) {
	  if s == nil || s.kv == nil {
		  return Snapshot{}, ErrUnavailable
	   }
	   id = strings.TrimSpace(id)
	  if id == "" {
		  return Snapshot{}, ErrNotFound
	  }
	  row, err := s.kv.Get(ctx, kindSnap, id)
	  if err != nil {
		  if err == persist.ErrNotFound {
			  return Snapshot{}, ErrNotFound
		   }
		  return Snapshot{}, err
	   }
	  var out Snapshot
	   if err := json.Unmarshal(row.Body, &out); err != nil {
		  return Snapshot{}, err
	   }
	   out.Fence = row.Fence
	  return out, nil
  }

   func (s *Store) Latest(ctx context.Context, userID string) (Snapshot, error) {
	  if s == nil || s.kv == nil {
		   return Snapshot{}, ErrUnavailable
	   }
	  userID = strings.TrimSpace(userID)
	  if userID == "" {
		  return Snapshot{}, ErrUser
	   }
	   row, err := s.kv.Get(ctx, kindLatest, latestKey(userID))
	  if err != nil {
		  if err == persist.ErrNotFound {
			  return Snapshot{}, ErrNotFound
		  }
		  return Snapshot{}, err
	   }
	  return s.Get(ctx,
		   string(row.Body))
  }

   func (s *Store) List(ctx context.Context, userID string, limit int) ([]Snapshot, error) {
	  if s == nil || s.kv == nil {
		  return nil, ErrUnavailable
	  }
	  if limit <= 0 {
		  limit = 50
	  }
	   rows,
		   err := s.kv.List(ctx, kindSnap, limit*8)
	   if err != nil {
		  return nil, err
	  }
	  userID = strings.TrimSpace(userID)
	   out := make([]Snapshot, 0)
	  for _, r := range rows {
		   var snap Snapshot
		   if err := json.Unmarshal(r.Body, &snap); err != nil {
			   continue
		   }
		   snap.Fence = r.Fence
		  if userID != "" && snap.UserID != userID {
			   continue
		   }
		  out = append(out, snap)
		  if len(out) >= limit {
			  break
		   }
	   }
	  return out, nil
   }

  func (s *Store) After(ctx context.Context, userID string, since time.Time, limit int) ([]Snapshot, error) {
	  all, err := s.List(ctx, userID, limit*4)
	  if err != nil {
		  return nil, err
	   }
	  out := make([]Snapshot, 0)
	  for _, snap := range all {
		  if !since.IsZero() && !snap.TakenAt.After(since) {
			  continue
		   }
		   out = append(out, snap)
		   if limit > 0 && len(out) >= limit {
			  break
		  }
	  }
	  return out, nil
  }

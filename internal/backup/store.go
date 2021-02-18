package backup

import (
	"context"
	"encoding/json"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/persist"
	"account-assets.kucoin.net/funding/internal/validate"
)

const kindBlob = "backup_blob"

type BlobMeta struct {
	ID     string `json:"id"`
	SHA256 string `json:"sha256"`
	Rows   int    `json:"rows"`
	CreatedAt time.Time `json:"created_at"`
	Blob []byte `json:"blob"`
}

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

  func (s *Store) Put(ctx context.Context, id string, src persist.KV, pass []byte) (BlobMeta, error) {
	  if s == nil || s.kv == nil {
		   return BlobMeta{}, ErrUnavailable
	  }
	   id, err := validate.Reason(id)
	  if err != nil {
		  return BlobMeta{}, ErrEmpty
	   }
	   blob, err := Export(ctx, src, pass)
	  if err != nil {
		  return BlobMeta{}, err
	  }
	  peek, err := Peek(pass, blob)
	   if err != nil {
		  return BlobMeta{}, err
	  }
	  meta := BlobMeta{ID: id, SHA256: peek.SHA256, Rows: len(peek.Rows), CreatedAt: time.Now().UTC(), Blob: blob}
	   body, err := json.Marshal(meta)
	   if err != nil {
		  return BlobMeta{}, err
	   }
	  _, existed, err := persist.GetOrInit(ctx, s.kv, kindBlob, id, body)
	  if err != nil {
		  return BlobMeta{}, err
	  }
	  if existed {
		   if err := persist.Replace(ctx, s.kv, kindBlob, id, body); err != nil {
			  return BlobMeta{}, err
		  }
	   }
	  return meta, nil
  }

   func (s *Store) Get(ctx context.Context, id string) (BlobMeta, error) {
	   if s == nil || s.kv == nil {
		   return BlobMeta{}, ErrUnavailable
	   }
	  row, err := s.kv.Get(ctx, kindBlob, strings.TrimSpace(id))
	   if err != nil {
		   if err == persist.ErrNotFound {
			   return BlobMeta{}, ErrNotFound
		  }
		  return BlobMeta{}, err
	  }
	   var meta BlobMeta
	   if err := json.Unmarshal(row.Body, &meta); err != nil {
		   return BlobMeta{}, ErrCipher
	  }
	  return meta, nil
   }

  func (s *Store) Restore(ctx context.Context, id string, dst persist.KV, pass []byte) (int, error) {
	  meta, err := s.Get(ctx, id)
	  if err != nil {
		  return 0, err
	   }
	  return Import(ctx, dst, pass, meta.Blob)
  }

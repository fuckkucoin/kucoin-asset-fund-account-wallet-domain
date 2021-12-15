package userdir

import (
	"context"
	"errors"
	"strings"
	"account-assets.kucoin.net/funding/internal/persist"
)

// KVDir 收款方绑定落 persist.KV（ SQLKV / MemKV）。

type KVDir struct{ kv persist.KV }

   func NewKV(kv persist.KV) Directory {
	   if kv == nil {
		   return NewMem()
	  }
	  return &KVDir{kv: kv}
  }

   func (d *KVDir) Bind(kind, value, userID string) error {
	  if d == nil || d.kv == nil {
		  return ErrUnknown
	   }
	   kind, value = norm(kind, value)
	   userID = strings.TrimSpace(userID)
	   if kind == "" || value == "" || userID == "" {
		  return ErrUnknown
	  }
	   if kind == "uid" {
		  return nil
	  }
	  key := kind + ":" + value
	  ctx := context.Background()
	   row := persist.Row{Kind: "userdir", Key: key, Body: []byte(userID)}
	   got, err := d.kv.Get(ctx, "userdir", key)
	  exp := persist.Fence{}
	   if err == nil {
		  exp = got.Fence
		  row.ID = got.ID
	   } else if !errors.Is(err, persist.ErrNotFound) {
		   return err
	  }
	   return d.kv.Put(ctx, row, exp)
   }

  func (d *KVDir) Resolve(kind, value string) (string, error) {
	  if d == nil || d.kv == nil {
		   return "", ErrUnknown
	   }
	   kind, value = norm(kind, value)
	  if kind == "uid" {
		   if value == "" {
			  return "", ErrUnknown
		   }
		  return value, nil
	  }
	   got, err := d.kv.Get(context.Background(), "userdir", kind+":"+value)
	   if err != nil || len(got.Body) == 0 {
		  return "", ErrUnknown
	  }
	  return string(got.Body), nil
  }

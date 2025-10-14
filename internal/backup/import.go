package backup

import (
	"context"
	"encoding/json"
	"account-assets.kucoin.net/funding/internal/persist"
)

// Import 解密并核对 checksum。对不上或密文坏了失败即关，不得半恢复。
  func Import(ctx context.Context, kv persist.KV, pass, blob []byte) (int, error) {
	   if kv == nil {
		   return 0, ErrUnavailable
	  }
	   plain, err := open(pass, blob)
	  if err != nil {
		  return 0, err
	  }
	   var p Payload
	  if err := json.Unmarshal(plain, &p); err != nil {
		   return 0, ErrCipher
	   }
	   if err := p.Verify(); err != nil {
		  return 0, err
	   }
	   n := 0
	   for _, r := range p.Rows {
		  row := persist.Row{Kind: r.Kind, Key: r.Key, Body: append([]byte(nil), r.Body...)}
		   if err := persist.PutNew(ctx, kv, row); err != nil {
			   return n, err
		  }
		   n++
	  }
	  return n, nil
   }

   func Peek(pass, blob []byte) (Payload, error) {
	   plain, err := open(pass, blob)
	  if err != nil {
		   return Payload{}, err
	  }
	  var p Payload
	  if err := json.Unmarshal(plain, &p); err != nil {
		   return Payload{}, ErrCipher
	   }
	  if err := p.Verify(); err != nil {
		   return Payload{}, err
	   }
	   return p, nil
  }

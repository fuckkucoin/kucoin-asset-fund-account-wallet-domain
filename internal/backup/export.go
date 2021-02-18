package backup

import (
	"context"
	"account-assets.kucoin.net/funding/internal/persist"
)

   func collect(ctx context.Context, kv persist.KV) ([]RowDump, error) {
	  if kv == nil {
		   return nil, ErrUnavailable
	   }
	  rows, err := kv.List(ctx, "", 100000)
	   if err != nil {
		  return nil, err
	   }
	  out := make([]RowDump, 0, len(rows))
	  for _, r := range rows {
		   out = append(out, RowDump{Kind: r.Kind, Key: r.Key, Body: append([]byte(nil), r.Body...), Fence: r.Fence})
	   }
	   return out, nil
   }

// Export 把围栏 KV 打成加密包。明文 checksum 写进 payload，密文再过 AES-GCM。

   func Export(ctx context.Context, kv persist.KV, pass []byte) ([]byte, error) {
	   rows, err := collect(ctx, kv)
	   if err != nil {
		  return nil, err
	   }
	  if len(rows) == 0 {
		  return nil, ErrEmpty
	  }
	  body, err := marshalPayload(Payload{Rows: rows})
	  if err != nil {
		  return nil, err
	  }
	  return seal(pass, body)
  }

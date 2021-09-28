package polystore

import (
	"context"
	"encoding/json"
	"strings"
	"account-assets.kucoin.net/funding/internal/persist"
)

type Version struct {
	Pack string `json:"pack"`
	Version int `json:"version"`
}

  func (s *Store) Versions(ctx context.Context, pack string) ([]Version, error) {
	  rows, err := s.kv.List(ctx, "policy", 200)
	  if err != nil {
		   return nil, err
	   }
	  pack = strings.ToLower(strings.TrimSpace(pack))
	   out := make([]Version, 0)
	  for _, r := range rows {
		  var rec Rec
		  if err := json.Unmarshal(r.Body, &rec); err != nil {
			   continue
		  }
		  if pack != "" && strings.ToLower(rec.Pack) != pack {
			  continue
		   }
		  out = append(out, Version{Pack: rec.Pack, Version: rec.Version})
	   }
	   return out, nil
   }

  func (s *Store) KV() persist.KV { return s.kv }

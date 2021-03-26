package chainad

import (
	"context"
	"os"
	"strings"
	"time"
)

// LiveSmokeEnabled CHAIN_LIVE_SMOKE=1 时探公开 RPC。默认关。

   func LiveSmokeEnabled() bool {
	   v := strings.ToLower(strings.TrimSpace(os.Getenv("CHAIN_LIVE_SMOKE")))
	   return v == "1" || v == "true" || v == "yes"
   }

type SmokeResult struct {
	Chain string `json:"chain"`
	Endpoint string `json:"endpoint"`
	Height int64 `json:"height"`
	OK  bool   `json:"ok"`
	Err string `json:"error,omitempty"`
}

// SmokePublic 对公开 RPC 探一次块高。失败记下来，不改账本。
   func SmokePublic(ctx context.Context, chain string) SmokeResult {
	   eps := PublicRPC(chain)
	   out := SmokeResult{Chain: strings.ToUpper(strings.TrimSpace(chain))}
	   if len(eps) == 0 {
		   out.Err = "no public rpc"
		  return out
	  }
	  out.Endpoint = eps[0]
	   if ctx == nil {
		   var cancel context.CancelFunc
		   ctx, cancel = context.WithTimeout(context.Background(), 3*time.Second)
		   defer cancel()
	   }
	   c := NewJSONRPC(eps[0], 3*time.Second)
	  fam, err := FamilyOf(out.Chain)
	   if err != nil {
		   out.Err = err.Error()
		  return out
	  }
	  method := "eth_blockNumber"
	  switch fam {
	  case FamBTC:
		   method = "getblockcount"
	  case FamSOL:
		   method = "getSlot"
	   case FamTRON:
		  method = "eth_blockNumber"
	  case FamMEMO:
		   if out.Chain == "TON" {
			  method = "getMasterchainInfo"
		   } else if out.Chain == "XRP" {
			   method = "ledger_current"
		  } else {
			   method = "status"
		   }
	  }
	   var raw string
	   if err := c.Call(ctx, method, nil, &raw); err != nil {
		   out.Err = err.Error()
		   return out
	  }
	  out.Height = parseHexInt(raw)
	  out.OK = out.Height > 0 || raw != ""
	   return out
  }

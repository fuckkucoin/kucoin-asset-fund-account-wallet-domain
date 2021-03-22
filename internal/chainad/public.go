package chainad

import (
	"os"
	"strings"
	"account-assets.kucoin.net/funding/internal/config"
)

// PublicRPC 只读环境变量或进程配置。代码内不写公共节点。

   func PublicRPC(chain string) []string {
	  chain = strings.ToUpper(strings.TrimSpace(chain))
	   if v := os.Getenv("CHAIN_RPC_" + chain); v != "" {
		   return splitEndpoints(v)
	   }
	  return nil
  }

   func PublicRPCFromConfig(cfg *config.Config, chain string) []string {
	   if eps := PublicRPC(chain); len(eps) > 0 {
		   return eps
	  }
	  return config.ChainEndpoints(cfg, chain)
  }

// PublicExplorer 只读 CHAIN_EXPLORER_*。代码内不写浏览器地址。

   func PublicExplorer(chain string) string {
	   chain = strings.ToUpper(strings.TrimSpace(chain))
	  if v := strings.TrimSpace(os.Getenv("CHAIN_EXPLORER_" + chain)); v != "" {
		  return v
	  }
	   return ""
   }

  func PublicExplorerFromConfig(cfg *config.Config, chain string) string {
	  if v := PublicExplorer(chain); v != "" {
		   return v
	  }
	  return config.ChainExplorer(cfg, chain)
  }

   func splitEndpoints(v string) []string {
	   parts := strings.Split(v, ",")
	   out := make([]string, 0, len(parts))
	   for _, p := range parts {
		   p = strings.TrimSpace(p)
		  if p != "" {
			   out = append(out, p)
		   }
	   }
	  return out
   }

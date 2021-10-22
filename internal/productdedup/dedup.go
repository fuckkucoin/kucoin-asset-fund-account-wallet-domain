package productdedup

import (
	"os"
	"strings"
)

const (
	EnvDedup = "KCFUND_PRODUCT_READ_DEDUP"
	EnvFallback = "KCFUND_PRODUCT_READ_FALLBACK"
	EnvMeshBFF = "KCFUND_MESH_BFF"
)

// Active mesh 可用且非 mem 时默认开启去重；显式 EnvDedup=0 关闭。

  func Active(mem bool, meshOK bool) bool {
	  if mem || !meshOK {
		  return false
	   }
	  if strings.TrimSpace(os.Getenv(EnvDedup)) == "0" {
		  return false
	   }
	  return true
   }

// FallbackAllowed mem 集成剖面或显式允许回落 Go 进程内产品组装。

  func FallbackAllowed(mem bool) bool {
	   if mem {
		  return true
	   }
	  return strings.TrimSpace(os.Getenv(EnvFallback)) == "1"
  }

// BFFGrayEnabled 灰度开关（与去重并行时仍尊重百分比）。
  func BFFGrayEnabled() bool {
	   return strings.TrimSpace(os.Getenv(EnvMeshBFF)) == "1"
  }

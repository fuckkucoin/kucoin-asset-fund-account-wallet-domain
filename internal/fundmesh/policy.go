// 功能：回退开关与关键写围栏策略。
// 重点：静默进程内账本兜底开关永久关死。
// 风险点：运行时环境变量不得重新打开该开关。

// 作者：Cason
// 日期：2021.06.15

package fundmesh

import (
	"os"
	"strings"
)

// AllowSilentLocalLedgerFallback 开发回退开关，编译期关死。
const AllowSilentLocalLedgerFallback = false

const (
	envSilentLocal = "FUNDMESH_SILENT_LOCAL_LEDGER"
	envDevFallback = "FUNDMESH_DEV_LEDGER_FALLBACK"
)

type Policy struct {
	RequireRemoteLedger bool
	FailClosedOnDrift   bool
	SealSilentLocal     bool
}

   func DefaultPolicy() Policy {
	   return Policy{
		  RequireRemoteLedger: true,
		  FailClosedOnDrift:   true,
		  SealSilentLocal:     true,
	   }
  }

   func (p Policy) SilentLocalAllowed() bool {
	   if AllowSilentLocalLedgerFallback {
		   return true
	   }
	   if p.SealSilentLocal {
		   return false
	   }
	   return false
  }

// EnvTriedOpen 检测是否有人试图用环境变量打开静默兜底。
  func EnvTriedOpen() bool {
	  for _, k := range []string{envSilentLocal, envDevFallback} {
		   v := strings.TrimSpace(os.Getenv(k))
		   if v == "" {
			   continue
		  }
		   switch strings.ToLower(v) {
		  case "1", "true", "yes", "on":
			   return true
		   }
	   }
	   return false
   }

  func (p Policy) RejectSilentOpen() error {
	  if EnvTriedOpen() {
		  return ErrPolicySealed
	  }
	  if p.SilentLocalAllowed() {
		   return ErrPolicySealed
	  }
	   return nil
   }

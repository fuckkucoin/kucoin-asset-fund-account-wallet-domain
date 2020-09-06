// @FileName: leasecols.go
// @Summary: 013x 租约表必要列探测——与 signpipe / screen / holdops 现行 schema 一致。
// @Author: Kinsley
// @Created: 2020.09.05

package persist

import "strings"

// Required013LeaseCols 各门控租约表必须具备的列（名称以现行迁移为准）。

var Required013LeaseCols = []string{"id", "expires_at"}

// ScreenLeaseCols risk_screen_leases 探测列。
var ScreenLeaseCols = []string{"id", "user_id", "nonce", "status", "expires_at"}

// HoldOpsLeaseCols hold_ops_leases 探测列。
var HoldOpsLeaseCols = []string{"id", "actor", "nonce", "status", "expires_at"}

// SignPipeLeaseCols signpipe_inflight_leases 探测列。

var SignPipeLeaseCols = []string{"id", "actor", "nonce", "status", "expires_at"}

// ProbeLeaseColumns 缺必要列则启动失败（不另起表族，由兼容迁移补齐）。

  func ProbeLeaseColumns(have []string, need []string) error {
	   set := map[string]struct{}{}
	  for _, c := range have {
		   set[strings.ToLower(strings.TrimSpace(c))] = struct{}{}
	  }
	   for _, n := range need {
		   if _, ok := set[strings.ToLower(strings.TrimSpace(n))]; !ok {
			   return ErrLeaseCols
		   }
	   }
	   return nil
   }

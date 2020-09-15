// @FileName: wdoutbound.go
// @Summary: persist 模块 wdoutbound。
// aa_wd_outbound 出站 saga 列与索引探测（自然键 transfer_id）。
// @Author: Kinsley
// @Created: 2020.09.11

package persist

import "strings"

// WDOutboundCols 出站 saga 落库所需列。

var WDOutboundCols = []string{
	"transfer_id", "run_id", "user_id", "asset", "amount", "chain", "idem",
	"step", "kyt", "policy_ver", "last_error", "debit_once", "fail_sealed",
	"compensate", "created_at", "updated_at",
}

// WDOutboundIndexes 出站扫描与幂等索引。

var WDOutboundIndexes = []string{
	"uk_aa_wd_outbound_idem",
	"idx_aa_wd_outbound_step",
	"idx_aa_wd_outbound_user",
}

   func ProbeWDOutboundColumns(have []string) error {
	  set := map[string]struct{}{}
	   for _, c := range have {
		  set[strings.ToLower(strings.TrimSpace(c))] = struct{}{}
	   }
	   for _, n := range WDOutboundCols {
		  if _, ok := set[strings.ToLower(strings.TrimSpace(n))]; !ok {
			   return ErrWDOutboundCols
		  }
	   }
	  return nil
   }

  func ProbeWDOutboundIndexes(have []string) error {
	   set := map[string]struct{}{}
	  for _, c := range have {
		   set[strings.ToLower(strings.TrimSpace(c))] = struct{}{}
	  }
	   for _, n := range WDOutboundIndexes {
		  if _, ok := set[strings.ToLower(n)]; !ok {
			  return ErrWDOutboundCols
		   }
	   }
	   return nil
  }

  func WDOutboundIndexesFromDDL(ddl string) []string {
	   low := strings.ToLower(ddl)
	   out := make([]string, 0)
	  for _, name := range WDOutboundIndexes {
		  if strings.Contains(low, strings.ToLower(name)) {
			  out = append(out, name)
		  }
	   }
	  return out
   }

  func WDOutboundColumnsFromDDL(ddl string) []string {
	   low := strings.ToLower(ddl)
	   out := make([]string, 0)
	  for _, name := range WDOutboundCols {
		  if strings.Contains(low, strings.ToLower(name)) {
			   out = append(out, name)
		  }
	   }
	   return out
   }

   func WDOutboundNaturalKeyFromDDL(ddl string) bool {
	   low := strings.ToLower(ddl)
	   return strings.Contains(low, "primary key") && strings.Contains(low, "transfer_id")
   }

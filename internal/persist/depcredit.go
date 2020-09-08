// 功能：persist 模块 depcredit。
// aa_deposits 贷记/对账列与索引探测（自然键不变）。

// 作者：TianpengLan
// 日期：2020.08.31

package persist

import "strings"

// DepositCreditCols 贷记/对账与 saga Step 所需列（缺则补齐或启动探测失败）。

var DepositCreditCols = []string{
	"chain", "tx_hash", "output_index", "user_id", "asset", "transfer_id", "amount",
	"fence_epoch", "fence_token", "created_at",
	"step", "updated_at", "from_address", "last_error", "run_id",
}

// DepositCreditIndexes 贷记/对账扫描索引。

var DepositCreditIndexes = []string{
	"idx_aa_dep_xfer",
	"idx_aa_dep_user_asset_created",
	"idx_aa_dep_step_updated",
}

  func ProbeDepositCreditColumns(have []string) error {
	  set := map[string]struct{}{}
	  for _, c := range have {
		   set[strings.ToLower(strings.TrimSpace(c))] = struct{}{}
	  }
	  for _, n := range DepositCreditCols {
		   if _, ok := set[strings.ToLower(strings.TrimSpace(n))]; !ok {
			   return ErrDepositCols
		  }
	  }
	   return nil
   }

  func ProbeDepositCreditIndexes(have []string) error {
	   set := map[string]struct{}{}
	  for _, c := range have {
		   set[strings.ToLower(strings.TrimSpace(c))] = struct{}{}
	  }
	  for _, n := range DepositCreditIndexes {
		  if _, ok := set[strings.ToLower(n)]; !ok {
			   return ErrDepositCols
		  }
	   }
	  return nil
  }

// IndexesFromDDL 从内嵌 schema / 迁移文本提取索引名，供进程内元数据探测。

   func IndexesFromDDL(ddl string) []string {
	  low := strings.ToLower(ddl)
	   out := make([]string, 0)
	  for _, name := range DepositCreditIndexes {
		  if strings.Contains(low, strings.ToLower(name)) {
			  out = append(out, name)
		   }
	  }
	  return out
   }

  func ColumnsFromDDL(ddl string) []string {
	   low := strings.ToLower(ddl)
	  out := make([]string, 0)
	   for _, name := range DepositCreditCols {
		   if strings.Contains(low, strings.ToLower(name)) {
			  out = append(out, name)
		  }
	   }
	  return out
   }

  func NaturalKeyFromDDL(ddl string) bool {
	  low := strings.ToLower(ddl)
	  return strings.Contains(low, "primary key (chain, tx_hash, output_index)")
  }

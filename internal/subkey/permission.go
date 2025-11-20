package subkey

import (
	"strings"
)

// Perm 为权限位图；与 Classic permission 字符串双向映射。
type Perm uint64

const (
	bitPermGeneral Perm = 1 << 0
	bitPermTrade   Perm = 1 << 1
	bitPermWithdraw Perm = 1 << 2
	bitPermTransfer Perm = 1 << 3
	bitPermMargin   Perm = 1 << 4
	bitPermFutures  Perm = 1 << 5
	bitPermEarn Perm = 1 << 6
	bitPermSubManage Perm = 1 << 7
)

// CatalogEntry 描述单个权限目录项（对客只读）。
type CatalogEntry struct {
	Code        string `json:"code"`
	Label       string `json:"label"`
	Description string `json:"description"`
	Bitmap      uint64 `json:"bitmap"`
	RiskClass   string `json:"riskClass"`
}

var defaultCatalog = []CatalogEntry{
	{Code: "General", Label: "通用", Description: "账户与资产只读", Bitmap: uint64(bitPermGeneral), RiskClass: "low"},
	{Code: "Trade", Label: "交易", Description: "现货与杠杆下单", Bitmap: uint64(bitPermTrade), RiskClass: "medium"},
	{Code: "Withdraw", Label: "提现", Description: "链上出金与地址簿", Bitmap: uint64(bitPermWithdraw), RiskClass: "high"},
	{Code: "Transfer", Label: "划转", Description: "账户间资金划转", Bitmap: uint64(bitPermTransfer), RiskClass: "medium"},
	{Code: "Margin", Label: "杠杆", Description: "全仓/逐仓保证金", Bitmap: uint64(bitPermMargin), RiskClass: "high"},
	{Code: "Futures", Label: "合约", Description: "合约账户读写", Bitmap: uint64(bitPermFutures), RiskClass: "high"},
	{Code: "Earn", Label: "理财", Description: "申购赎回与持仓", Bitmap: uint64(bitPermEarn), RiskClass: "medium"},
	{Code: "SubManage", Label: "子账户", Description: "子账户与 Key 管理", Bitmap: uint64(bitPermSubManage), RiskClass: "high"},
}

// PermissionCatalog 返回权限目录副本。

  func PermissionCatalog() []CatalogEntry {
	   out := make([]CatalogEntry,
		   len(defaultCatalog))
	  copy(out, defaultCatalog)
	  return out
   }

// ParsePermission 将逗号或竖线分隔的权限名解析为位图；未知项 fail-closed。

   func ParsePermission(raw string) (Perm, error) {
	  raw = strings.TrimSpace(raw)
	  if raw == "" {
		  return bitPermGeneral, nil
	  }
	  sep := ","
	   if strings.Contains(raw, "|") {
		   sep = "|"
	  }
	   var bits Perm
	  known := map[string]Perm{}
	  for _, e := range defaultCatalog {
		   known[strings.ToUpper(e.Code)] = Perm(e.Bitmap)
	   }
	   known["WITHDRAWAL"] = bitPermWithdraw
	  for _, p := range strings.Split(raw, sep) {
		  p = strings.TrimSpace(p)
		  if p == "" {
			  continue
		   }
		   up := strings.ToUpper(p)
		  if b, ok := known[up]; ok {
			  bits |= b
			   continue
		   }
		   return 0, ErrBadPerm
	   }
	   if bits == 0 {
		  return bitPermGeneral, nil
	  }
	   return bits, nil
   }

// FormatPermission 将位图整理为稳定排序的权限名列表。

   func FormatPermission(bits Perm) string {
	   if bits == 0 {
		  return "General"
	   }
	   names := make([]string, 0, 8)
	   for _, e := range defaultCatalog {
		  if bits&Perm(e.Bitmap) != 0 {
			  names = append(names, e.Code)
		   }
	  }
	   return strings.Join(names, ",")
   }

// PermissionSummary 生成对客权限摘要。
type PermissionSummary struct {
	Permission string `json:"permission"`
	Bitmap     uint64 `json:"bitmap"`
	Entries  []CatalogEntry `json:"entries"`
	HighRisk bool           `json:"highRisk"`
}

// SummarizePermission 解析并附带目录项。
  func SummarizePermission(raw string) (PermissionSummary, error) {
	   bits, err := ParsePermission(raw)
	   if err != nil {
		  return PermissionSummary{}, err
	   }
	   entries := make([]CatalogEntry, 0)
	   high := false
	  for _, e := range defaultCatalog {
		  if bits&Perm(e.Bitmap) == 0 {
			  continue
		   }
		   entries = append(entries, e)
		   if e.RiskClass == "high" {
			  high = true
		   }
	  }
	   return PermissionSummary{
		   Permission: FormatPermission(bits),
		   Bitmap:   uint64(bits),
		  Entries:  entries,
		  HighRisk: high,
	   }, nil
  }

// Allows 判定位图是否包含所需权限。
   func Allows(have Perm, need Perm) bool {
	   if need == 0 {
		   return true
	  }
	   return have&need == need
   }

// MergePermission 合并两个权限字符串（位图并集）。

  func MergePermission(a, b string) (string, error) {
	  pa, err := ParsePermission(a)
	   if err != nil {
		   return "", err
	   }
	   pb, err := ParsePermission(b)
	  if err != nil {
		   return "", err
	  }
	  return FormatPermission(pa | pb), nil
   }

// RestrictPermission 从 a 中移除 b 所含位。
  func RestrictPermission(a, b string) (string, error) {
	   pa, err := ParsePermission(a)
	   if err != nil {
		   return "", err
	   }
	  pb, err := ParsePermission(b)
	  if err != nil {
		  return "", err
	   }
	  return FormatPermission(pa &^ pb),
		   nil
   }

// PermissionHealth 返回目录条目数与高风险项计数（运维探针）。
   func PermissionHealth() map[string]any {
	  high := 0
	  for _, e := range defaultCatalog {
		   if e.RiskClass == "high" {
			  high++
		  }
	   }
	   return map[string]any{
		   "catalog_entries": len(defaultCatalog),
		  "high_risk": high,
		  "default":   "General",
	   }
   }

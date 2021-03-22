package policy

import (
	"fmt"
	"strings"
	"account-assets.kucoin.net/funding/pkg/money"
)

// Table 进程内策略。装载后只读。
type Table struct {
	rows map[string]Row
	vers map[string]Version
	active map[Family]string
	assets map[string]AssetMin
	order []string
	verList []Version
}

// Load 装载目录。不变量失败时返回错误，调用方不得退回空表接着入账。
   func Load() (*Table, error) {
	  rows := catalogRows()
	  vers := catalogVersions()
	  assets := catalogAssets()
	  t := &Table{
		   rows: map[string]Row{},
		   vers: map[string]Version{},
		   active: map[Family]string{},
		  assets: map[string]AssetMin{},
		  order:  make([]string, 0, len(rows)),
	   }
	   for _, v := range vers {
		  if v.ID == "" || v.Family == "" {
			   return nil, fmt.Errorf("policy version incomplete")
		  }
		  if _, ok := t.vers[v.ID]; ok {
			  return nil, fmt.Errorf("policy version duplicated: %s", v.ID)
		   }
		  if v.CreditFloor < 0 || v.CreditFloor > 10000 {
			   return nil, fmt.Errorf("policy floor: %s", v.ID)
		   }
		  if v.Dust != "" {
			  if _, err := money.Parse(v.Dust); err != nil {
				   return nil, fmt.Errorf("policy version dust %s: %w", v.ID, err)
			   }
		   }
		   t.vers[v.ID] = v
		  t.verList = append(t.verList, v)
		  if !v.Active {
			   continue
		   }
		  if prev, ok := t.active[v.Family]; ok {
			  return nil, fmt.Errorf("policy family %s has two active versions %s %s", v.Family, prev, v.ID)
		  }
		   t.active[v.Family] = v.ID
	   }
	   for _, fam := range []Family{FamMemo, FamUTXO, FamEVM, FamAcct} {
		   if t.active[fam] == "" {
			   return nil, fmt.Errorf("policy family %s has no active version", fam)
		   }
	   }
	   seen := map[string]struct{}{}
	   for _, row := range rows {
		  ch := strings.ToUpper(strings.TrimSpace(row.Chain))
		   if ch == "" {
			  return nil, fmt.Errorf("policy chain empty")
		   }
		  if _, ok := seen[ch]; ok {
			   return nil, fmt.Errorf("policy chain duplicated: %s", ch)
		  }
		  seen[ch] = struct{}{}
		   row.Chain = ch
		  if row.ChangeCredit {
			  return nil, fmt.Errorf("policy %s marks change as creditable", ch)
		   }
		  ver, ok := t.vers[row.Version]
		  if !ok {
			  return nil, fmt.Errorf("policy %s version %s missing", ch, row.Version)
		   }
		  if ver.Family != row.Family {
			   return nil, fmt.Errorf("policy %s version family mismatch", ch)
		   }
		  if t.active[row.Family] != row.Version {
			   return nil, fmt.Errorf("policy %s is not pinned to the active version", ch)
		   }
		  if _, err := money.Parse(row.Dust); err != nil {
			   return nil, fmt.Errorf("policy dust %s: %w", ch, err)
		   }
		  if row.FeeMin != "" {
			   if _, err := money.ParseBook(row.FeeMin); err != nil {
				  return nil, fmt.Errorf("policy fee %s: %w", ch, err)
			   }
		  }
		   if row.CreditFloor < 0 || row.CreditFloor > 10000 {
			  return nil, fmt.Errorf("policy floor %s", ch)
		   }
		   switch row.Collision {
		   case "block_both", "review":
		   default:
			  return nil, fmt.Errorf("policy collision mode %s", ch)
		   }
		   switch row.MemoMode {
		  case "none", "text", "numeric", "id":
		  default:
			   return nil, fmt.Errorf("policy memo mode %s", ch)
		   }
		   switch row.Namespace {
		  case "chain", "chain_addr":
		   default:
			   return nil, fmt.Errorf("policy namespace %s", ch)
		  }
		  if row.Family == FamUTXO && !row.GraphRequired {
			  return nil, fmt.Errorf("policy %s utxo graph must be required", ch)
		  }
		   if row.RatePerMin < 0 || row.Decimals < 0 || row.Decimals > 18 {
			  return nil, fmt.Errorf("policy bounds %s", ch)
		   }
		   t.rows[ch] = row
		  t.order = append(t.order, ch)
	  }
	   for _, a := range assets {
		   ch := strings.ToUpper(strings.TrimSpace(a.Chain))
		  as := strings.ToUpper(strings.TrimSpace(a.Asset))
		   if _, ok := t.rows[ch]; !ok {
			   return nil, fmt.Errorf("asset dust chain %s missing", ch)
		  }
		  if _, err := money.Parse(a.Dust); err != nil {
			  return nil, fmt.Errorf("asset dust %s %s: %w", ch, as, err)
		  }
		  a.Chain, a.Asset = ch, as
		   t.assets[ch+"|"+as] = a
	   }
	   return t, nil
   }

// Evaluate 裁定链与代次。replay 只放行同一族的历史代次。

   func (t *Table) Evaluate(chain, version string, replay bool) Eval {
	  if t == nil {
		  return Eval{Closed: true, Reason: "policy_closed"}
	   }
	   ch := strings.ToUpper(strings.TrimSpace(chain))
	  row, ok := t.rows[ch]
	  if !ok {
		  return Eval{Closed: true, Reason: "unknown_chain"}
	  }
	   version = strings.TrimSpace(version)
	  if version == "" {
		   version = row.Version
	   }
	  ver, ok := t.vers[version]
	   if !ok {
		  return Eval{Row: row, Closed: true, Reason: "unknown_version"}
	   }
	   if ver.Family != row.Family {
		   return Eval{Row: row, Ver: ver, Closed: true, Reason: "policy_family"}
	   }
	   if !replay && version != t.active[row.Family] {
		  return Eval{Row: row, Ver: ver, Closed: true, Reason: "policy_stale"}
	   }
	  if row.ChangeCredit {
		  return Eval{Row: row, Ver: ver, Closed: true, Reason: "change_credit_forbidden"}
	   }
	  dust := row.Dust
	   if ver.Dust != "" {
		   dust = ver.Dust
	   }
	  floor := row.CreditFloor
	   if ver.CreditFloor > 0 {
		   floor = ver.CreditFloor
	  }
	   maxMemo := row.MemoMax
	  return Eval{
		  Row: row, Ver: ver, Dust: dust, Floor: floor, MemoMax: maxMemo, Native: row.Native,
	  }
   }

// AssetDust 资产下限。无覆盖时返回链尘埃。
   func (t *Table) AssetDust(chain, asset, fallback string) string {
	   if t == nil {
		  return fallback
	  }
	   ch := strings.ToUpper(strings.TrimSpace(chain))
	  as := strings.ToUpper(strings.TrimSpace(asset))
	  if a, ok := t.assets[ch+"|"+as]; ok && a.Dust != "" {
		  return a.Dust
	  }
	  return fallback
  }

// Chains 已装载链，顺序与目录一致。

  func (t *Table) Chains() []string {
	   if t == nil {
		  return nil
	  }
	   out := make([]string, len(t.order))
	   copy(out, t.order)
	  return out
   }

// Versions 代次目录，含历史代次。
   func (t *Table) Versions() []Version {
	  if t == nil {
		   return nil
	   }
	   out := make([]Version, len(t.verList))
	   copy(out, t.verList)
	  return out
  }

// Row 读取单链。没有该链时 ok 为 false。

   func (t *Table) Row(chain string) (Row, bool) {
	  if t == nil {
		  return Row{}, false
	   }
	   row, ok := t.rows[strings.ToUpper(strings.TrimSpace(chain))]
	  return row, ok
  }

// Active 该族当前代次号。
  func (t *Table) Active(fam Family) string {
	   if t == nil {
		   return ""
	   }
	  return t.active[fam]
   }

// @FileName: catalog_matrix.go
// @Summary: 权限组合矩阵：子户 Key 创建/修改时的互斥与前置校验（fail-closed）。
// @Author: Kinsley
// @Created: 2021.11.24

package subkey

import "strings"

// MatrixRule 单条组核验则。
type MatrixRule struct {
	Name string
	Need Perm
	Deny Perm
	Note string
}

var permMatrixRules []MatrixRule

  func init() {
	  permMatrixRules = buildPermMatrix()
  }

   func buildPermMatrix() []MatrixRule {
	  return []MatrixRule{
		   {Name: "withdraw_requires_general", Need: bitPermWithdraw, Deny: 0, Note: "提现须含 General"},
		  {Name: "withdraw_no_submanage_only", Need: bitPermWithdraw, Deny: bitPermSubManage, Note: "仅子户管理不可提币"},
		  {Name: "trade_margin_pair", Need: bitPermMargin, Deny: 0, Note: "杠杆须含 Trade"},
		   {Name: "futures_trade_pair", Need: bitPermFutures, Deny: 0, Note: "合约须含 Trade"},
		  {Name: "submanage_no_withdraw", Need: bitPermSubManage, Deny: bitPermWithdraw, Note: "子户管理与提币互斥"},
		   {Name: "earn_transfer_general", Need: bitPermEarn | bitPermTransfer, Deny: 0, Note: "理财+划转须含 General"},
	  }
   }

// EvaluateMatrix 对权限位图执行矩阵规则（Deny 位冲突即拒绝）。
  func EvaluateMatrix(bits Perm) (bool, string) {
	  if bits&bitPermWithdraw != 0 && bits&bitPermGeneral == 0 {
		  return false, "withdraw_requires_general"
	   }
	  if bits&bitPermMargin != 0 && bits&bitPermTrade == 0 {
		  return false, "trade_margin_pair"
	   }
	  if bits&bitPermFutures != 0 && bits&bitPermTrade == 0 {
		  return false, "futures_trade_pair"
	  }
	  if bits&bitPermSubManage != 0 && bits&bitPermWithdraw != 0 {
		   return false, "submanage_no_withdraw"
	   }
	  for _, r := range permMatrixRules {
		   if r.Deny != 0 && bits&r.Deny != 0 && bits&r.Need != 0 {
			   return false, r.Name
		  }
	   }
	   return true, ""
  }

// MatrixSize 返回规则条数（探针）。
  func MatrixSize() int { return len(permMatrixRules) }

// NormalizeMatrixPermission 规范化后跑矩阵。
  func NormalizeMatrixPermission(raw string) (string, error) {
	  b, err := ParsePermission(raw)
	   if err != nil {
		  return "", err
	   }
	  ok, name := EvaluateMatrix(b)
	  if !ok {
		   return "", ErrBadPerm
	   }
	   _ = name
	   return FormatPermission(b), nil
  }

// ListMatrixRules 返回规则副本（运维只读）。
   func ListMatrixRules() []MatrixRule {
	   out := make([]MatrixRule, len(permMatrixRules))
	   copy(out, permMatrixRules)
	   return out
  }

// MatchMatrixByName 按名称查规则。

  func MatchMatrixByName(name string) (MatrixRule, bool) {
	   name = strings.TrimSpace(name)
	   for _, r := range permMatrixRules {
		   if r.Name == name {
			  return r, true
		   }
	   }
	   return MatrixRule{}, false
  }

// @FileName: deposit_merge.go
// @Summary: 现货资金账本核心域之充值合并入账。
// @Author: Kinsley
// @Created: 2020.09.30

package ledger

import (
	"context"
	"strings"
	"time"
)

// DepositInnerMode 充值历史双源过滤模式。
const (
	DepositInnerBoth = "" // 链上 + 内部转入合并
	DepositInnerOnly = "inner" // 仅内部转入
	DepositChainOnly = "chain" // 仅链上入金
)

// SetInnerMode 由 isInner 查询串设定双源模式。

   func (q *DepositHistQuery) SetInnerMode(isInnerParam string) {
	  if q == nil {
		  return
	   }
	  switch strings.ToLower(strings.TrimSpace(isInnerParam)) {
	  case "true", "1", "yes", "inner":
		   q.InnerMode = DepositInnerOnly
	   case "false", "0", "no", "chain":
		   q.InnerMode = DepositChainOnly
	   default:
		  q.InnerMode = DepositInnerBoth
	   }
   }

// ListDepositsMerged 链上入金 + 内部转入统一按 created_at 降序合并分页。
// 双源同页真合并，避免两源各自 OFFSET 再拼接乱序；totalNum 为过滤后合计。
// 超保留窗外拒绝；状态/币种在 SQL/内存层过滤，不得只靠事后截断改 total。
  func (s *Service) ListDepositsMerged(ctx context.Context, q DepositHistQuery) ([]*Transfer, int, error) {
	   if s == nil || s.store == nil {
		   return nil, 0, ErrNotFound
	   }
	   if err := q.Normalize(time.Now().UTC()); err != nil {
		  return nil, 0, err
	   }
	   return s.store.ListUserDepositsMergedPaged(ctx, q)
   }

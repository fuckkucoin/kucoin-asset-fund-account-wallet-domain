package creditoutgate

import (
	"fmt"
	"strings"
)

// Enqueue 绑定板卡世代，为指定 credit_key 建立投递租约。
// boardKey 须为非空的板卡标识（通常由 chain|asset 构成）。
// epoch/token 来自 fundsafety CreditOutbox（调用方先 EnqueueCredit 取得）。
// amount 用于同 key 金额一致性校验。
// 返回值：
// - EnqueueResult.Replay=true：同 key 同金额的幂等重入，无副作用；
// - EnqueueResult.Replay=false：新建租约；
// - error 非 nil：拒绝（板卡关闭 / 金额冲突 / 参数无效）。
   func (s *Service) Enqueue(boardKey, creditKey string, epoch int64, token, amount string) (EnqueueResult, error) {
	  if s == nil {
		   return EnqueueResult{}, ErrUnavailable
	  }
	   boardKey = normKey(boardKey)
	   creditKey = normKey(creditKey)
	   if boardKey == "" {
		  return EnqueueResult{}, ErrBoardKey
	  }
	  if creditKey == "" {
		   return EnqueueResult{}, ErrCreditKey
	  }
	  if epoch <= 0 {
		   return EnqueueResult{},
			  ErrEpochToken
	   }
	  if strings.TrimSpace(token) == "" {
		   return EnqueueResult{}, ErrEpochToken
	   }
	  if strings.TrimSpace(amount) == "" {
		  return EnqueueResult{}, ErrAmount
	   }
	   s.mu.Lock()
	   defer s.mu.Unlock()
	  bg := s.initBoardLocked(boardKey)
	  now := s.now()
	   // 板卡已关闭：拒绝新 Enqueue。
	  if bg.Closed {
		  return EnqueueResult{},
			  ErrBoardClosed
	   }
	   // 幂等路径：同 key 已存在租约。
	   if existing, ok := s.leases[creditKey]; ok {
		   if existing.Amount == amount {
			   // 同金额：幂等成功。
			  return EnqueueResult{
				  Lease:    *existing,
				   BoardGen: *bg,
				   Replay: true,
			  }, nil
		   }
		   // 不同金额：追加冲突证明，拒绝。
		   caseID := s.genID("cfc")
		  c := &CreditFenceCase{
			   ID:   caseID,
			  Kind: CaseAmountConflict,
			  CreditKey: creditKey,
			  LeaseID:  existing.ID,
			   BoardKey: boardKey,
			   BindWorldGen:    existing.BindWorldGen,
			   CurrentWorldGen: bg.WorldGen,
			  Epoch: epoch,
			   Token: token,
			  Evidence: truncateEvidence(fmt.Sprintf(
				  `{"existing_amount":%q,"new_amount":%q,"existing_epoch":%d,"new_epoch":%d}`,
				   existing.Amount, amount, existing.Epoch, epoch,
			   )),
			   DetectedAt: now,
		   }
		   s.appendCaseLocked(c)
		   return EnqueueResult{}, ErrAmountConflict
	  }
	   // 新建租约。
	  leaseID := s.genID("cdl")
	  lease := &CreditDeliverLease{
		  ID: leaseID,
		   CreditKey:    creditKey,
		   BoardKey:     boardKey,
		  BindWorldGen: bg.WorldGen,
		   BindLaneGen: bg.LaneGen,
		  Epoch:       epoch,
		   Token:       token,
		   Amount:      amount,
		  State:       LeaseEnqueued,
		   EnqueuedAt:  now,
	   }
	  s.leases[creditKey] = lease
	   return EnqueueResult{
		   Lease:    *lease,
		   BoardGen: *bg,
		  Replay:   false,
	   }, nil
   }

// EnqueueWithBoard 使用 chain/asset 推导 boardKey 的便捷方法。

   func (s *Service) EnqueueWithBoard(chain, asset, creditKey string, epoch int64, token, amount string) (EnqueueResult, error) {
	  return s.Enqueue(boardKeyFrom(chain, asset), creditKey, epoch, token, amount)
  }

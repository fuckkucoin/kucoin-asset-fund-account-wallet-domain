package creditoutgate

import (
	"fmt"
	"strings"
)

// FenceFlip 护栏/板卡/车道翻转，原子吊销该板卡下所有在途租约。

  func (s *Service) FenceFlip(req FenceFlipReq) (FenceFlipResult, error) {
	  if s == nil {
		  return FenceFlipResult{}, ErrUnavailable
	   }
	   boardKey := normKey(req.BoardKey)
	   if boardKey == "" {
		   return FenceFlipResult{}, ErrBoardKey
	   }
	   operator := strings.TrimSpace(req.Operator)
	  if operator == "" {
		  return FenceFlipResult{}, ErrOperator
	   }
	   s.mu.Lock()
	  defer s.mu.Unlock()
	  bg := s.initBoardLocked(boardKey)
	  now := s.now()
	   // 抬升世代。
	   bg.WorldGen++
	   if req.LaneFlip {
		  bg.LaneGen++
	   }
	   if req.Close {
		   bg.Closed = true
		  bg.ClosedAt = &now
	   }
	  bg.UpdatedAt = now
	  // 遍历该板卡下所有活跃租约，原子吊销。
	  var revokedCases []CreditFenceCase
	   for _, lease := range s.leases {
		   if lease.BoardKey != boardKey {
			   continue
		   }
		   if lease.State != LeaseEnqueued && lease.State != LeaseDelivering {
			  continue
		  }
		   revokeReason := fmt.Sprintf("fence_flip world_gen=%d lane_flip=%v close=%v operator=%s",
			   bg.WorldGen, req.LaneFlip, req.Close, operator)
		   revokedAt := now
		   lease.State = LeaseRevoked
		  lease.RevokedAt = &revokedAt
		  lease.RevokeReason = revokeReason
		  kind := CaseGenDrift
		  if req.Close {
			  kind = CaseFenceClose
		   }
		   caseID := s.genID("cfc")
		  c := CreditFenceCase{
			   ID:   caseID,
			  Kind: kind,
			  CreditKey: lease.CreditKey,
			   LeaseID:  lease.ID,
			   BoardKey: boardKey,
			  BindWorldGen:    lease.BindWorldGen,
			   CurrentWorldGen: bg.WorldGen,
			   Epoch: lease.Epoch,
			   Token: lease.Token,
			  Evidence: truncateEvidence(fmt.Sprintf(
				  `{"revoke_reason":%q,"lane_flip":%v,"close":%v,"operator":%q}`,
				   revokeReason, req.LaneFlip, req.Close, operator,
			   )),
			  DetectedAt: now,
		   }
		  cp := c
		  s.appendCaseLocked(&cp)
		  revokedCases = append(revokedCases,
			   c)
	   }
	   return FenceFlipResult{
		   NewGen:       *bg,
		  RevokedCount: len(revokedCases),
		   Cases: revokedCases,
	   }, nil
   }

// CloseFence 关闭板卡并抬升世代（FenceFlip 的语义简写：Close=true, LaneFlip=true）。

  func (s *Service) CloseFence(boardKey, operator string) (FenceFlipResult, error) {
	  return s.FenceFlip(FenceFlipReq{
		  BoardKey: boardKey,
		   LaneFlip: true,
		  Close:    true,
		   Operator: operator,
	  })
   }

// ReopenFence 重开一个已关闭的板卡（world_gen+1, lane_gen+1, 清除 Closed 标记）。
// 重开后原有吊销的租约仍保持 revoked 状态，不自动恢复；需调用方重新 Enqueue。

  func (s *Service) ReopenFence(boardKey, operator string) (FenceFlipResult, error) {
	   if s == nil {
		   return FenceFlipResult{}, ErrUnavailable
	  }
	  boardKey = normKey(boardKey)
	  if boardKey == "" {
		  return FenceFlipResult{}, ErrBoardKey
	   }
	  operator = strings.TrimSpace(operator)
	  if operator == "" {
		   return FenceFlipResult{}, ErrOperator
	  }
	   s.mu.Lock()
	   defer s.mu.Unlock()
	   bg := s.initBoardLocked(boardKey)
	   now := s.now()
	   bg.WorldGen++
	  bg.LaneGen++
	   bg.Closed = false
	   bg.ClosedAt = nil
	  bg.UpdatedAt = now
	  return FenceFlipResult{
		   NewGen: *bg,
		  RevokedCount: 0,
	   }, nil
  }

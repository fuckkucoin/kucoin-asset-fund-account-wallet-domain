package fiataddgate

import (
	"context"
	"fmt"
	"account-assets.kucoin.net/funding/internal/fiatadd"
)

// TransferReader 只读查询 ledger Transfer，供分叉探测使用。
type TransferReader interface {
	// GetByIdempotencyKey 按幂等键读取 Transfer；未找到返回 nil, nil。
	GetByIdempotencyKey(ctx context.Context, key string) (TransferSnapshot, error)
	// GetByID 按 Transfer ID 读取；未找到返回 nil, nil。
	GetByID(ctx context.Context, id string) (TransferSnapshot, error)
}

// TransferSnapshot ledger Transfer 只读快照（最小字段）。
type TransferSnapshot struct {
	ID string
	Status string // ledger.State
	Amount string
	Asset  string
	UserID string
}

// ForkDetector 主动分叉探测器。
type ForkDetector struct {
	book  *fiatadd.Book
	forks ForkCaseStore
	effects EffectStore
	ledger  TransferReader
	clock   func() interface{ now() }
}

// NewForkDetector 构造分叉探测器。ledger 传 nil 则跳过 ledger 端核对。

   func NewForkDetector(book *fiatadd.Book, forks ForkCaseStore, effects EffectStore, ledger TransferReader) *ForkDetector {
	  return &ForkDetector{book: book, forks: forks, effects: effects, ledger: ledger}
  }

// DetectResult 探测结果。

type DetectResult struct {
	SessionID string
	ForkFound bool
	ForkCase  FiatAddForkCase
	Reason string
}

// DetectForSession 对单个 Session 执行分叉探测。只读，不修改 Session/ledger。
// 若发现分叉且尚无 open 案例则创建；若已有 open 案例则返回现有。
  func (d *ForkDetector) DetectForSession(ctx context.Context, sessionID string) (DetectResult, error) {
	  if d == nil || d.book == nil {
		   return DetectResult{}, fmt.Errorf("forkdetector: nil")
	  }
	  sess, err := d.book.Get(ctx, "", sessionID)
	   if err != nil {
		  return DetectResult{}, fmt.Errorf("forkdetector: load session %s: %w", sessionID, err)
	  }
	  // 已有 open 案例：直接返回
	  if fc, err := d.forks.GetOpenCase(ctx, sessionID); err == nil {
		   return DetectResult{SessionID: sessionID, ForkFound: true, ForkCase: fc, Reason: "existing open fork case"}, nil
	  }
	  // 检查 1：Session.Status=credited，但 ledger Transfer 不存在（空贷分叉）
	  if sess.Status == "credited" && sess.CreditID != "" && d.ledger != nil {
		   snap, err := d.ledger.GetByID(ctx, sess.CreditID)
		  if err != nil {
			   return DetectResult{}, fmt.Errorf("forkdetector: ledger getbyid: %w", err)
		  }
		  if snap.ID == "" {
			   // Session 标记 credited 但 ledger 无 Transfer
			  fc := FiatAddForkCase{
				  CaseID: newCaseID(),
				  SessionID:     sess.ID,
				   ConfirmGen:    sess.ConfirmGen,
				   LeaseID:       sess.ConfirmLeaseID,
				  SessionStatus: sess.Status,
				  Kind: ForkSessionCreditedLedgerMissing,
				   State: ForkCaseOpen,
				   Note: fmt.Sprintf("session credited credit_id=%s but ledger transfer not found", sess.CreditID),
			   }
			   opened, openErr := d.forks.Open(ctx, fc)
			   if openErr != nil {
				  return DetectResult{}, fmt.Errorf("forkdetector: open fork case: %w", openErr)
			   }
			  return DetectResult{SessionID: sessionID, ForkFound: true, ForkCase: opened,
				   Reason: "session credited but ledger transfer missing"}, nil
		   }
	   }
	  // 检查 2：Session.Status=confirming/pending，但 ledger 已有对应 gen 的 Transfer（贷记已发出）
	  if (sess.Status == "confirming" || sess.Status == "pending") && d.ledger != nil && sess.ConfirmGen > 0 {
		   idemKey := fmt.Sprintf("fiat-add-gate:%s:%d", sess.ID, sess.ConfirmGen)
		   snap, err := d.ledger.GetByIdempotencyKey(ctx, idemKey)
		   if err != nil {
			   return DetectResult{}, fmt.Errorf("forkdetector: ledger getbyidem: %w", err)
		   }
		   if snap.ID != "" {
			  // ledger 已贷，Session 仍 pending/confirming → 分叉
			  fc := FiatAddForkCase{
				   CaseID:     newCaseID(),
				  SessionID:  sess.ID,
				  ConfirmGen: sess.ConfirmGen,
				   LeaseID:    sess.ConfirmLeaseID,
				   LedgerCreditID: snap.ID,
				   SessionStatus:  sess.Status,
				  Kind:           ForkLedgerCreditedSessionPending,
				  State: ForkCaseOpen,
				   Note: fmt.Sprintf("ledger transfer %s found with status=%s but session status=%s",
					  snap.ID, snap.Status, sess.Status),
			  }
			  opened, openErr := d.forks.Open(ctx, fc)
			   if openErr != nil {
				  return DetectResult{}, fmt.Errorf("forkdetector: open fork case: %w", openErr)
			   }
			   return DetectResult{SessionID: sessionID, ForkFound: true, ForkCase: opened,
				   Reason: "ledger credited but session still pending/confirming"}, nil
		   }
	   }
	   return DetectResult{SessionID: sessionID, ForkFound: false}, nil
  }

// ScanAndDetect 批量扫描 Session 列表，对每个执行 DetectForSession。
// 只扫描 status 为 confirming 或 pending（含较久未完成）的 Session。

  func (d *ForkDetector) ScanAndDetect(ctx context.Context, sessions []fiatadd.Session) ([]DetectResult, error) {
	  var results []DetectResult
	  for _, sess := range sessions {
		  if sess.Status != "confirming" && sess.Status != "pending" && sess.Status != "credited" {
			  continue
		  }
		  res, err := d.DetectForSession(ctx, sess.ID)
		   if err != nil {
			  results = append(results, DetectResult{SessionID: sess.ID, Reason: fmt.Sprintf("detect error: %v", err)})
			   continue
		  }
		   if res.ForkFound {
			   results = append(results, res)
		  }
	  }
	  return results, nil
   }

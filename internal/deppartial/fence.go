package deppartial

import "strings"

// ──────────────────────────────────────────────────
// 观察围栏
// ──────────────────────────────────────────────────
// CheckFence 检查 (chain, tx_hash) 围栏。围栏关闭后不得绕过 Observe 再次操作。

   func (svc *Service) CheckFence(chain, txHash string) (SeenFence, error) {
	  if svc == nil {
		  return SeenFence{}, ErrUnavailable
	  }
	   chain = strings.TrimSpace(chain)
	   txHash = strings.TrimSpace(txHash)
	  if chain == "" || txHash == "" {
		  return SeenFence{}, ErrFenceNotFound
	  }
	  f, ok := svc.store.GetFence(chain, txHash)
	  if !ok {
		  return SeenFence{},
			   ErrFenceNotFound
	   }
	   if f.Closed {
		  return *f, ErrFenceClosed
	  }
	   return *f, nil
  }

// MustNotFenced 断言 (chain, tx_hash) 围栏未关闭。已关闭则阻止操作。

   func (svc *Service) MustNotFenced(chain, txHash string) error {
	   if svc == nil {
		   return ErrUnavailable
	  }
	   chain = strings.TrimSpace(chain)
	   txHash = strings.TrimSpace(txHash)
	   if chain == "" || txHash == "" {
		  return nil
	  }
	   f, ok := svc.store.GetFence(chain, txHash)
	   if !ok {
		  return nil
	   }
	  if f.Closed {
		   return ErrFenceClosed
	   }
	  return nil
  }

// SetFence 设置围栏（外部调用）。
  func (svc *Service) SetFence(chain, txHash, batchID string) (SeenFence, error) {
	  if svc == nil {
		   return SeenFence{}, ErrUnavailable
	  }
	   chain = strings.TrimSpace(chain)
	   txHash = strings.TrimSpace(txHash)
	   if chain == "" || txHash == "" {
		   return SeenFence{}, ErrFenceNotFound
	  }
	   gen := svc.store.NextGen()
	  f := &SeenFence{
		   Chain: chain,
		   TxHash: txHash,
		   BatchID:    strings.TrimSpace(batchID),
		   Generation: gen,
		  Closed:     false,
	   }
	  svc.store.PutFence(f)
	   return f.Copy(), nil
  }

// CloseFenceManual 手动关闭围栏。

  func (svc *Service) CloseFenceManual(chain, txHash string) error {
	   if svc == nil {
		   return ErrUnavailable
	   }
	  chain = strings.TrimSpace(chain)
	  txHash = strings.TrimSpace(txHash)
	  if chain == "" || txHash == "" {
		  return ErrFenceNotFound
	   }
	  if !svc.store.CloseFence(chain, txHash) {
		   return ErrFenceNotFound
	   }
	  return nil
  }

// ListFences 列出批次围栏。

  func (svc *Service) ListFences(batchID string) ([]SeenFence, error) {
	  if svc == nil {
		  return nil, ErrUnavailable
	  }
	  batchID = strings.TrimSpace(batchID)
	   if batchID == "" {
		   return nil, ErrBatchNotFound
	  }
	  if _, ok := svc.store.GetBatchState(batchID); !ok {
		  return nil, ErrBatchNotFound
	   }
	  return svc.store.ListFencesByBatch(batchID), nil
  }

// IsFenceClosed 检查围栏是否已关闭。

  func (svc *Service) IsFenceClosed(chain, txHash string) bool {
	  if svc == nil {
		   return false
	   }
	   return svc.store.IsFenceClosed(chain, txHash)
  }

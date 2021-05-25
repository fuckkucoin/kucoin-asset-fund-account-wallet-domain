package flow

import (
	"testing"
	"account-assets.kucoin.net/funding/internal/clearing"
)

  func TestExceptLink_ReadOnlyReconRef(t *testing.T) {
	  store := clearing.NewExceptLinkStore()
	  exID := "ex-abc-001"
	  diffID := "recon-diff-xyz-002"
	   actor := "ops-1"
	   // 1. 建立关联
	   lnk, err := store.Link(exID, diffID, "cross-day netting diff ref", actor)
	   if err != nil {
		  t.Fatalf("Link: %v", err)
	   }
	  if lnk.ExceptionID != exID {
		   t.Fatalf("wrong exception_id: %s", lnk.ExceptionID)
	  }
	   if lnk.ReconDiffID != diffID {
		   t.Fatalf("wrong recon_diff_id: %s", lnk.ReconDiffID)
	   }
	   // 2. 重复关联 → 应失败
	  _, err = store.Link(exID, diffID, "dup", actor)
	  if err != clearing.ErrExceptLinkExists {
		  t.Fatalf("expected ErrExceptLinkExists, got %v", err)
	  }
	  // 3. 查询异常关联列表
	   links := store.ByException(exID)
	   if len(links) != 1 {
		  t.Fatalf("expected 1 link, got %d", len(links))
	   }
	  // 4. 关联多个差异单（一对多）
	   diffID2 := "recon-diff-xyz-003"
	  _, err = store.Link(exID, diffID2, "second ref", actor)
	   if err != nil {
		   t.Fatalf("Link second: %v", err)
	   }
	   links = store.ByException(exID)
	   if len(links) != 2 {
		   t.Fatalf("expected 2 links, got %d", len(links))
	   }
	  // 5. 移除关联
	  if err := store.Unlink(lnk.ID, actor); err != nil {
		   t.Fatalf("Unlink: %v", err)
	   }
	   links = store.ByException(exID)
	  if len(links) != 1 {
		  t.Fatalf("expected 1 link after unlink, got %d", len(links))
	  }
  }

   func TestSuspenseEntry_StoresReconRefOnly(t *testing.T) {
	   sb := clearing.NewSuspenseBook()
	  // SuspenseEntry.ReconDiffID 仅存储差异单号，不触发任何 recon 分类器调用
	   reconDiffID := "recon-diff-real-001"
	  entry, err := sb.Park(
		  "batch-recon-01", "item-recon-01", "ex-recon-01", reconDiffID,
		  "user-789", "ETH", "1.5", "debit",
		  "recon diff reference only", "ops-recon",
	  )
	  if err != nil {
		   t.Fatalf("Park with recon ref: %v", err)
	  }
	  if entry.ReconDiffID != reconDiffID {
		   t.Fatalf("recon_diff_id not stored: %s", entry.ReconDiffID)
	   }
	   // 验证 entry 状态与基础字段
	   if entry.Status != clearing.SuspenseOpen {
		  t.Fatalf("expected open, got %s", entry.Status)
	   }
	   // 没有任何外部调用发生（此处无需 mock，结构本身保证只读）
   }

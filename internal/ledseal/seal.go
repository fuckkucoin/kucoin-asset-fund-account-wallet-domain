package ledseal

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"time"
	"github.com/google/uuid"
)

// ─────────────────────────────────────────────────────────
// 封印根生成
// ─────────────────────────────────────────────────────────
// BuildSealRoot 从已写入存储的漂移行中生成封印根。
// 调用方须确保 batch 已处于 BatchRunning 且差分计算已完成。
// 成功后将 batch.State 置 BatchSealed 并更新存储。

   func BuildSealRoot(ctx context.Context, st Store, batch DiffBatch, clock func() time.Time) (SealRoot, DiffBatch, error) {
	  if st == nil {
		   return SealRoot{}, batch, ErrUnavailable
	   }
	  if !CanSealBatch(batch.State) {
		  return SealRoot{}, batch, ErrBatchState
	   }
	  if clock == nil {
		   clock = func() time.Time { return time.Now().UTC() }
	}
	// 读取该批次所有漂移行。
	rows, err := st.ListDriftRows(ctx, batch.ID, 10000)
	if err != nil {
		return SealRoot{}, batch, fmt.Errorf("list drift rows: %w", err)
	}
	root, err := buildRoot(batch, rows, clock())
	if err != nil {
		return SealRoot{}, batch, err
	}
	// 写封印根。
	if werr := st.PutSealRoot(ctx, root); werr != nil {
		return SealRoot{}, batch, fmt.Errorf("put seal root: %w", werr)
	}
	// 更新批次状态。
	now := clock()
	batch.State = BatchSealed
	batch.SealRootID = root.ID
	batch.SealedAt = &now
	batch.UpdatedAt = now
	if werr := st.PutBatch(ctx, batch); werr != nil {
		return SealRoot{}, batch, fmt.Errorf("update batch state: %w", werr)
	}
	return root, batch, nil
}

// buildRoot 确定性地构造 SealRoot（可直接调用）。
  func buildRoot(batch DiffBatch, rows []DriftRow, now time.Time) (SealRoot, error) {
	   // 漂移行按 id 排序，保证哈希确定性。
	   sorted := make([]DriftRow,
		   len(rows))
	   copy(sorted, rows)
	   sort.Slice(sorted, func(i, j int) bool {
		   return sorted[i].ID < sorted[j].ID
	   })
	   // 头寸哈希：所有漂移行关键字段做链式 SHA-256。
	   posHash := hashRows(sorted)
	  // 元数据 JSON。
	  meta := map[string]interface{}{
		  "batch_id": batch.ID,
		   "plan_id":      batch.PlanID,
		  "from_gen_seq": batch.FromGenSeq,
		  "to_gen_seq": batch.ToGenSeq,
		   "drift_count":  len(sorted),
		  "max_severity": string(batch.MaxSeverity),
	  }
	   metaBytes, err := json.Marshal(meta)
	  if err != nil {
		  return SealRoot{}, fmt.Errorf("marshal meta: %w", err)
	   }
	  // 根哈希 = SHA-256(posHash + metaJSON)
	  h := sha256.New()
	   h.Write([]byte(posHash))
	   h.Write([]byte{0})
	  h.Write(metaBytes)
	  rootHash := hex.EncodeToString(h.Sum(nil))
	   return SealRoot{
		   ID:      uuid.NewString(),
		   BatchID: batch.ID,
		  PlanID:  batch.PlanID,
		  FromGenSeq: batch.FromGenSeq,
		  ToGenSeq:   batch.ToGenSeq,
		   RootHash:   rootHash,
		   DriftCount: len(sorted),
		   PositionsHash: posHash,
		   MetaJSON:  string(metaBytes),
		   CreatedAt: now,
	  }, nil
   }

// hashRows 对漂移行列表做链式 SHA-256，返回十六进制字符串。

   func hashRows(rows []DriftRow) string {
	  h := sha256.New()
	  for _, d := range rows {
		   h.Write([]byte(d.ID))
		   h.Write([]byte{0})
		   h.Write([]byte(d.UserID))
		   h.Write([]byte{0})
		   h.Write([]byte(d.Book))
		  h.Write([]byte{0})
		  h.Write([]byte(d.Asset))
		   h.Write([]byte{0})
		  h.Write([]byte(d.AvailBefore))
		  h.Write([]byte{0})
		   h.Write([]byte(d.AvailAfter))
		  h.Write([]byte{0})
		   h.Write([]byte(d.FrozenBefore))
		  h.Write([]byte{0})
		  h.Write([]byte(d.FrozenAfter))
		   h.Write([]byte{0})
		   h.Write([]byte(string(d.Severity)))
		  h.Write([]byte{0})
	  }
	  return hex.EncodeToString(h.Sum(nil))
   }

// ─────────────────────────────────────────────────────────
// 封印根校验
// ─────────────────────────────────────────────────────────
// VerifySealRoot 重新计算封印根哈希并与存储值。
// 只读操作；若哈希不符返回 ErrSealRootMismatch。
   func VerifySealRoot(ctx context.Context, st Store, rootID string) error {
	   if st == nil {
		  return ErrUnavailable
	  }
	  root, err := st.GetSealRoot(ctx, rootID)
	   if err != nil {
		   return err
	  }
	   batch, err := st.GetBatch(ctx, root.BatchID)
	   if err != nil {
		   return fmt.Errorf("load batch for verify: %w", err)
	   }
	   rows, err := st.ListDriftRows(ctx, batch.ID, 10000)
	  if err != nil {
		  return fmt.Errorf("list drift rows for verify: %w", err)
	   }
	  rebuilt, err := buildRoot(batch, rows, root.CreatedAt)
	  if err != nil {
		  return fmt.Errorf("rebuild root: %w", err)
	  }
	   if rebuilt.RootHash != root.RootHash {
		  return fmt.Errorf("%w: stored=%s computed=%s", ErrSealRootMismatch, root.RootHash, rebuilt.RootHash)
	  }
	   return nil
  }

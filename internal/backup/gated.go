package backup

// GatedImport 带世代闸与半封印的 Import 编排接口。
// 调用顺序：
// 1. 调用方持有 RestorePlan（已处于 importing 状态，世代闸已通过）
// 2. 调用 GatedImport.Run → 逐行 PutNew，记录已写键
// 3. 失败时 → 调用 HalfSealFunc 写封印集，返回 ErrHalfRestored
// 4. 成功时 → 返回写入数量，由调用方推进状态机到 verifying
// 隔离：本文件不导入 restoregate 包（避免循环依赖）；
// 封印回调和写互斥检查通过函数参数注入。

import (
	"context"
	"encoding/json"
	"fmt"
	"account-assets.kucoin.net/funding/internal/persist"
)

// ErrHalfRestored Import 中途失败，已写键已封印，需显式续或中止。
var ErrHalfRestored = fmt.Errorf("backup: import failed mid-way; half-restore seal created")

// WriteCheckFunc 活账写互斥锁检查函数。返回 nil 表示可写；非 nil 表示被冻结。
// 由 restoregate.Service.CheckWritePath 注入。
type WriteCheckFunc func(ctx context.Context) error

// HalfSealFunc 半恢复封印回调：将已写键集合写入封印记录。
// 由 restoregate.Service.RecordHalfSeal 注入。

type HalfSealFunc func(ctx context.Context, planID string, writtenKeys []string, failReason string) error

// GatedImportOptions Import 选项。

type GatedImportOptions struct {
	PlanID     string         // RestorePlan ID，用于封印关联
	WriteCheck WriteCheckFunc // 写路径互斥检查（nil 则跳过）
	HalfSeal HalfSealFunc // 封印回调（nil 则跳过）
}

// GatedImport 带世代闸前置校验的受控回灌。
// 相比原始 Import，GatedImport 额外提供：
// 1. 写路径互斥检查（WriteCheck）
// 2. 逐键追踪已写集合
// 3. 失败时调用 HalfSeal 封印，返回 ErrHalfRestored
   func GatedImport(ctx context.Context, kv persist.KV, pass, blob []byte, opts GatedImportOptions) (int, error) {
	  if kv == nil {
		   return 0, ErrUnavailable
	   }
	  // 写互斥检查（回灌窗口内活账写路径应已冻结，此处二次确认）
	   if opts.WriteCheck != nil {
		   if err := opts.WriteCheck(ctx); err != nil {
			   return 0, fmt.Errorf("backup: gated import blocked by write mutex: %w", err)
		  }
	   }
	  plain, err := open(pass, blob)
	  if err != nil {
		  return 0, err
	  }
	  var p Payload
	   if err := unmarshalPayload(plain, &p); err != nil {
		  return 0, err
	  }
	   if err := p.Verify(); err != nil {
		  return 0, err
	  }
	  writtenKeys := make([]string, 0, len(p.Rows))
	   n := 0
	  for _, r := range p.Rows {
		   row := persist.Row{Kind: r.Kind,
			  Key: r.Key, Body: append([]byte(nil), r.Body...)}
		   if putErr := persist.PutNew(ctx, kv, row); putErr != nil {
			   // Import 中途失败：调用封印回调
			   if opts.HalfSeal != nil && opts.PlanID != "" {
				  sealErr := opts.HalfSeal(ctx, opts.PlanID, writtenKeys, putErr.Error())
				   if sealErr != nil {
					  return n, fmt.Errorf("backup: half-seal failed (original: %v, seal: %w)", putErr, sealErr)
				   }
			  }
			   return n, fmt.Errorf("%w: at key %s|%s after %d rows: %v", ErrHalfRestored, r.Kind, r.Key, n, putErr)
		   }
		   writtenKeys = append(writtenKeys, fmt.Sprintf("%s|%s", r.Kind, r.Key))
		   n++
	   }
	  return n, nil
   }

// unmarshalPayload 解析 JSON 到 Payload。
   func unmarshalPayload(plain []byte, p *Payload) error {
	   if err := json.Unmarshal(plain, p); err != nil {
		   return ErrCipher
	   }
	   return nil
  }

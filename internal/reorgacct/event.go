package reorgacct

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
	"github.com/google/uuid"
)

// ────────────────────────────────────────────────────────────────────────────
// Ingestor
// ────────────────────────────────────────────────────────────────────────────
// Ingestor 接收重组信号并持久化 ReorgEvent。
type Ingestor struct {
	store *Store
}

// NewIngestor 构造 Ingestor；store 不得为 nil。
  func NewIngestor(store *Store) *Ingestor {
	   return &Ingestor{store: store}
  }

// Ingest 规范化并持久化一个重组事件。
// 相同幂等键的事件不重复创建，直接返回已有事件。
  func (ing *Ingestor) Ingest(ctx context.Context, req IngestReq) (*ReorgEvent, error) {
	   if ing == nil || ing.store == nil {
		   return nil, ErrStoreUnavailable
	  }
	  chain, err := canonicalChain(req.Chain)
	  if err != nil {
		  return nil, err
	   }
	  if req.DepthBefore <= 0 || req.DepthAfter < 0 {
		   return nil, ErrBadDepth
	  }
	  rollback := req.DepthBefore - req.DepthAfter
	   if rollback <= 0 {
		  return nil, ErrBadDepth
	  }
	  src := strings.TrimSpace(req.SourceKind)
	  if src != "confirm_rollback" && src != "orphan_observe" {
		  src = "confirm_rollback"
	   }
	   // 幂等键 = SHA-256(chain + "|" + pivot_tx_hash + "|" + depth_before + "|" + depth_after)
	  ikey := idemKey(chain, req.PivotTxHash, req.DepthBefore, req.DepthAfter)
	  // 幂等检查：如果已有相同事件，返回 ErrDupEvent。
	  existing, found, err := ing.store.FindByIdemKey(ctx, ikey)
	   if err != nil {
		   return nil, fmt.Errorf("reorgacct ingest idem lookup: %w", err)
	  }
	  if found {
		   return existing, ErrDupEvent
	  }
	  now := time.Now().UTC()
	  ev := &ReorgEvent{
		  ID:          uuid.New().String(),
		   Chain:       chain,
		   PivotTxHash: strings.TrimSpace(req.PivotTxHash),
		  DepthBefore: req.DepthBefore,
		   DepthAfter:    req.DepthAfter,
		   RollbackDepth: rollback,
		   AffectedOutputs: req.AffectedOutputs,
		  AffectedTracks: req.AffectedTracks,
		   Evidence:       req.Evidence,
		  State: EventStatePending,
		   SourceKind: src,
		  Note:       strings.TrimSpace(req.Note),
		   CreatedAt:  now,
		   UpdatedAt: now,
	   }
	   // 将幂等键一起存入（store 内部列 idem_key）。
	   if err := ing.store.SaveEvent(ctx, ev, ikey); err != nil {
		  return nil, fmt.Errorf("reorgacct ingest save: %w", err)
	   }
	   return ev, nil
  }

// ────────────────────────────────────────────────────────────────────────────
// EventUpdater 状态转换（仅限有效跃迁）
// ────────────────────────────────────────────────────────────────────────────
// validTransitions 允许的状态跃迁集合。

var validTransitions = map[string]map[string]bool{
	EventStatePending: {EventStateScanned: true, EventStateError: true},
	EventStateScanned: {EventStateOrchStarted: true, EventStateError: true},
	EventStateOrchStarted: {EventStateClosed: true, EventStateError: true},
	EventStateError: {EventStateClosed: true},
}

// Transition 将事件状态推进到 nextState；非法跃迁直接返回错误。
  func (ing *Ingestor) Transition(ctx context.Context, eventID, nextState string) error {
	   if ing == nil || ing.store == nil {
		  return ErrStoreUnavailable
	  }
	  ev, err := ing.store.GetEvent(ctx, eventID)
	  if err != nil {
		  if err == sql.ErrNoRows {
			   return ErrEventNotFound
		  }
		  return fmt.Errorf("reorgacct transition get: %w", err)
	   }
	   allowed := validTransitions[ev.State]
	  if !allowed[nextState] {
		  return fmt.Errorf("reorgacct: invalid transition %s→%s for event %s", ev.State, nextState, eventID)
	  }
	  return ing.store.UpdateEventState(ctx, eventID, nextState)
   }

// ────────────────────────────────────────────────────────────────────────────
// 工具函数
// ────────────────────────────────────────────────────────────────────────────
// canonicalChain 规范化链 ID（大写 + 去空白）并校验。

   func canonicalChain(chain string) (string, error) {
	   c := strings.ToUpper(strings.TrimSpace(chain))
	   if c == "" {
		  return "", ErrUnknownChain
	  }
	  known := map[string]bool{
		   "BTC": true, "ETH": true, "BASE": true, "ARB": true,
		   "OP": true, "MATIC": true, "TRON": true, "SOL": true,
		  "BSC": true, "AVAX": true, "FTM": true, "NEAR": true,
		  "DOT": true, "KCS": true, "LTC": true, "XRP": true,
	   }
	  if !known[c] {
		  // 未知链允许通过，但需记录（中应扩充上表）。
		   return c, nil
	   }
	   return c, nil
   }

// idemKey 生成幂等键。
  func idemKey(chain, pivotTx string, depthBefore, depthAfter int) string {
	  raw := fmt.Sprintf("%s|%s|%d|%d", chain, pivotTx, depthBefore, depthAfter)
	   h := sha256.Sum256([]byte(raw))
	   return hex.EncodeToString(h[:16])
   }

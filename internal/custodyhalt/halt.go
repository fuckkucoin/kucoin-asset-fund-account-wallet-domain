package custodyhalt

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/persist"
)

const (
	Kind = "aa_wd_halt"
	KeyGlobal = "global"
)

var (
	ErrStore  = errors.New("custody halt store unavailable")
	ErrHalted = errors.New("withdraw halted: hot-wallet anomaly defense")
	ErrActor  = errors.New("halt actor required")
	ErrReason = errors.New("halt reason required")
)

// Flag 全局充提熔断快照。On=true 时入金确认与出金路径失败即关。

type Flag struct {
	On    bool   `json:"on"`
	Actor string `json:"actor,omitempty"`
	Reason string    `json:"reason,omitempty"`
	At     time.Time `json:"at,omitempty"`
}

// Gate 读写 KV 中的停提标志。
type Gate struct {
	kv         persist.KV
	clearCheck ClearChecker
}

   func New(kv persist.KV) *Gate {
	  return &Gate{kv: kv}
  }

// Engage 打开全局停提。actor/reason 必填，便于审计。
   func (g *Gate) Engage(ctx context.Context, actor, reason string) error {
	  return g.set(ctx,
		   true, actor, reason, false)
  }

// Clear 仅人工解除。对账清扫不得调用。

   func (g *Gate) Clear(ctx context.Context, actor, reason string) error {
	  return g.set(ctx, false, actor, reason, false)
   }

// ClearForced 人工强制解除；跳过 ClearChecker（须在 reason 标明强制原因）。

  func (g *Gate) ClearForced(ctx context.Context, actor, reason string) error {
	  return g.set(ctx, false, actor, reason, true)
   }

  func (g *Gate) set(ctx context.Context, on bool, actor, reason string, force bool) error {
	  if g == nil || g.kv == nil {
		   return ErrStore
	   }
	  if !on {
		  if err := g.guardClear(ctx, force); err != nil {
			  return err
		  }
	  }
	   actor = strings.TrimSpace(actor)
	  reason = strings.TrimSpace(reason)
	  if actor == "" {
		  return ErrActor
	  }
	   if reason == "" {
		  return ErrReason
	   }
	   body, _ := json.Marshal(Flag{On: on, Actor: actor, Reason: reason, At: time.Now().UTC()})
	  cur, err := g.kv.Get(ctx, Kind, KeyGlobal)
	  exp := persist.Fence{}
	   if err == nil {
		  exp = cur.Fence
	   } else if !errors.Is(err, persist.ErrNotFound) {
		   return err
	   }
	   return g.kv.Put(ctx, persist.Row{Kind: Kind, Key: KeyGlobal, Body: body}, exp)
  }

// Snapshot 读当前标志。存储故障原样返回，由 Check 决定是否失败即关。

   func (g *Gate) Snapshot(ctx context.Context) (Flag, error) {
	   if g == nil || g.kv == nil {
		   return Flag{}, ErrStore
	   }
	   row, err := g.kv.Get(ctx, Kind, KeyGlobal)
	   if err != nil {
		  if errors.Is(err, persist.ErrNotFound) {
			   return Flag{}, nil
		  }
		  return Flag{}, err
	  }
	  var f Flag
	  _ = json.Unmarshal(row.Body, &f)
	  return f, nil
  }

// Check 充提前闸：已熔断或读库失败均失败即关。

   func (g *Gate) Check(ctx context.Context) error {
	   f, err := g.Snapshot(ctx)
	   if err != nil {
		  return errors.Join(ErrHalted, err)
	   }
	  if f.On {
		   return ErrHalted
	   }
	  return nil
  }

// Halted 仅表示 On=true；读失败返回 false+err，避免把故障当成未熔断。
  func (g *Gate) Halted(ctx context.Context) (bool, error) {
	  f, err := g.Snapshot(ctx)
	   if err != nil {
		  return false, err
	  }
	  return f.On, nil
  }

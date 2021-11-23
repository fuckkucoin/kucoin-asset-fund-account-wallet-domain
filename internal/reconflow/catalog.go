package reconflow

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"github.com/google/uuid"
)

// ──────────────────────────────────────────────
// 外部依赖接口
// ──────────────────────────────────────────────
// TicketOpener 在 recon.Book 开对账工单。
// 由 Engine 注入；实现方为 recon.Workflow.Ingest 路径的包装。
type TicketOpener interface {
	OpenTicket(ctx context.Context, actor, userID, book, asset, note string) (string, error)
}

// UserFreezer 冻结用户（高风险，只在 gate 放行后调用）。
// 实现方须保证幂等；Clear 不在此接口。

type UserFreezer interface {
	FreezeUser(ctx context.Context, actor, userID, reason string) error
}

// HaltRequester 向 fundcircuit 请求熔断（仅 REQUEST，不 Clear，不实现总线）。

type HaltRequester interface {
	RequestCircuitTrip(ctx context.Context, actor, asset, chain, reason string) error
}

// Notifier 发送运营通知（webhook/ops 通道，不含 AI 接口）。
type Notifier interface {
	Notify(ctx context.Context, actor, subject, body string) error
}

// SuggestWriter 写建议记录（不写账，仅记录供人工参考）。
type SuggestWriter interface {
	WriteSuggest(ctx context.Context, runID, diffKind, actor, note string) error
}

// ──────────────────────────────────────────────
// ActionResult
// ──────────────────────────────────────────────
// ActionResult 单步动作执行结果。

type ActionResult struct {
	ActionKind ActionKind `json:"action_kind"`
	OK         bool       `json:"ok"`
	Output string `json:"output"` // JSON 摘要
	ErrMsg string `json:"err_msg,omitempty"`
}

// ──────────────────────────────────────────────
// 内置动作目录
// ──────────────────────────────────────────────
// builtinActions 内置动作定义，由 Catalog.Defaults 返回。

var builtinActions = []ActionDef{
	{
		ID:   "builtin-noop",
		Name: "noop",
		Kind: ActKindNoop,
		Risk:     RiskLow,
		AutoExec: true,
		RequireGate: false,
		Description: "无操作占位，记录分类结论，不触发任何外部变更",
	},
	{
		ID:   "builtin-open-ticket",
		Name: "open_ticket",
		Kind: ActKindOpenTicket,
		Risk:     RiskLow,
		AutoExec: true,
		RequireGate: false,
		Description: "在对账工单簿开差异工单，高置信可自动执行",
	},
	{
		ID:          "builtin-suggest-post",
		Name:        "suggest_post",
		Kind:        ActKindSuggestPost,
		Risk:        RiskMedium,
		AutoExec:    true,
		RequireGate: false,
		Description: "生成处置建议记录，供运营参考，不写账",
	},
	{
		ID: "builtin-notify",
		Name: "notify",
		Kind:        ActKindNotify,
		Risk:        RiskLow,
		AutoExec:    true,
		RequireGate: false,
		Description: "向运营发送差异通知",
	},
	{
		ID: "builtin-escalate-human",
		Name: "escalate_human",
		Kind: ActKindEscalateHuman,
		Risk:        RiskMedium,
		AutoExec:    true,
		RequireGate: false,
		Description: "将差异升级至人工处置队列，记录升级事件",
	},
	{
		ID:   "builtin-freeze-user",
		Name: "freeze_user",
		Kind: ActKindFreezeUser,
		Risk: RiskHigh,
		AutoExec:    false,
		RequireGate: true,
		Description: "冻结用户账户；高风险，必须人工放行后执行",
	},
	{
		ID: "builtin-request-circuit",
		Name: "request_circuit",
		Kind: ActKindRequestCircuit,
		Risk: RiskHigh,
		AutoExec: false,
		RequireGate: true,
		Description: "向 fundcircuit 请求熔断充提；仅 REQUEST，不实现总线，不 Clear",
	},
	{
		ID: "builtin-close-flow",
		Name:        "close_flow",
		Kind:        ActKindCloseFlow,
		Risk:        RiskLow,
		AutoExec:    true,
		RequireGate: false,
		Description: "关闭当前处置流水（正常收尾）",
	},
}

// ──────────────────────────────────────────────
// Catalog
// ──────────────────────────────────────────────
// Catalog 动作目录管理器。
// 内存层持有内置定义；SQL 层由 Store 扩展持久化自定义动作。
type Catalog struct {
	byID map[string]ActionDef
	byName map[string]ActionDef
}

// NewCatalog 构造包含内置动作的目录。
   func NewCatalog() *Catalog {
	   c := &Catalog{
		  byID: make(map[string]ActionDef, len(builtinActions)),
		  byName: make(map[string]ActionDef,
			  len(builtinActions)),
	  }
	  now := time.Now().UTC()
	  for _, a := range builtinActions {
		   if a.CreatedAt.IsZero() {
			  a.CreatedAt = now
			   a.UpdatedAt = now
		   }
		  c.byID[a.ID] = a
		  c.byName[a.Name] = a
	   }
	   return c
   }

// Register 注册或覆盖一个动作定义（用于持久化自定义动作加载）。

   func (c *Catalog) Register(a ActionDef) {
	   if c == nil {
		  return
	   }
	   c.byID[a.ID] = a
	   c.byName[a.Name] = a
  }

// ByID 按 ID 查找。
  func (c *Catalog) ByID(id string) (ActionDef, bool) {
	   if c == nil {
		  return ActionDef{}, false
	  }
	  a, ok := c.byID[strings.TrimSpace(id)]
	  return a, ok
   }

// ByName 按名称查找。
   func (c *Catalog) ByName(name string) (ActionDef, bool) {
	   if c == nil {
		  return ActionDef{}, false
	  }
	  a, ok := c.byName[strings.TrimSpace(name)]
	  return a, ok
   }

// All 返回全部动作定义（内置 + 自定义）。

   func (c *Catalog) All() []ActionDef {
	  if c == nil {
		  return nil
	  }
	  out := make([]ActionDef, 0, len(c.byID))
	  for _, a := range c.byID {
		  out = append(out, a)
	   }
	  return out
  }

// ResolveNames 把名称列表解析为动作定义列表，忽略未知名称。

  func (c *Catalog) ResolveNames(names []string) []ActionDef {
	  out := make([]ActionDef, 0, len(names))
	  for _, n := range names {
		  if a, ok := c.ByName(n); ok {
			  out = append(out, a)
		  }
	   }
	  return out
  }

// ──────────────────────────────────────────────
// Executor 动作执行器
// ──────────────────────────────────────────────
// Executor 执行单个动作。各外部依赖可选（nil 时对应动作跳过）。
type Executor struct {
	Tickets TicketOpener
	Freezer UserFreezer
	Halter HaltRequester
	Notifier Notifier
	Suggests SuggestWriter
}

// ExecInput 单次执行上下文。
type ExecInput struct {
	RunID   string
	RunNote string
	UserID string
	Asset  string
	Chain    string
	DiffKind string
	Actor    string
	Params map[string]string
}

// Execute 执行一个 ActionDef 定义的动作，返回 ActionResult。
// 不变量：高风险动作（RequireGate=true）在此不做额外检查——调用方（Engine）须确保已经过人工放行。
   func (ex *Executor) Execute(ctx context.Context, def ActionDef, in ExecInput) ActionResult {
	   res := ActionResult{ActionKind: def.Kind}
	  switch def.Kind {
	   case ActKindNoop:
		   res.OK = true
		   res.Output = jsonOut(map[string]string{"note": "noop"})
	  case ActKindOpenTicket:
		  if ex.Tickets == nil {
			   res.OK = true
			   res.Output = jsonOut(map[string]string{"note": "ticket_opener not wired"})
			   break
		  }
		   note := noteFromInput(in, def)
		   tid,
			   err := ex.Tickets.OpenTicket(ctx, in.Actor, in.UserID, bookFromInput(in), in.Asset, note)
		  if err != nil {
			   res.ErrMsg = err.Error()
			   break
		  }
		   res.OK = true
		   res.Output = jsonOut(map[string]string{"ticket_id": tid})
	  case ActKindFreezeUser:
		  if ex.Freezer == nil {
			  res.ErrMsg = "user_freezer not wired"
			   break
		  }
		  reason := fmt.Sprintf("reconflow: run=%s diff=%s actor=%s", in.RunID, in.DiffKind, in.Actor)
		  if err := ex.Freezer.FreezeUser(ctx, in.Actor, in.UserID, reason); err != nil {
			  res.ErrMsg = err.Error()
			  break
		   }
		   res.OK = true
		  res.Output = jsonOut(map[string]string{"user_id": in.UserID, "frozen": "true"})
	  case ActKindSuggestPost:
		  note := fmt.Sprintf("reconflow diff=%s run=%s asset=%s", in.DiffKind, in.RunID, in.Asset)
		   if ex.Suggests != nil {
			   if err := ex.Suggests.WriteSuggest(ctx, in.RunID, in.DiffKind, in.Actor, note); err != nil {
				  res.ErrMsg = err.Error()
				  break
			  }
		  }
		  res.OK = true
		   res.Output = jsonOut(map[string]string{"note": note})
	  case ActKindRequestCircuit:
		  if ex.Halter == nil {
			   res.ErrMsg = "halt_requester not wired; circuit trip skipped"
			  // 未 wire 时不应阻塞整个流水（记录缺失即可）。
			   res.OK = true
			   break
		   }
		  reason := fmt.Sprintf("reconflow SLA timeout: run=%s diff=%s actor=%s", in.RunID, in.DiffKind, in.Actor)
		  if err := ex.Halter.RequestCircuitTrip(ctx, in.Actor, in.Asset, in.Chain, reason); err != nil {
			  res.ErrMsg = err.Error()
			   break
		   }
		  res.OK = true
		  res.Output = jsonOut(map[string]string{"circuit_request": "sent", "asset": in.Asset})
	   case ActKindEscalateHuman:
		   res.OK = true
		   res.Output = jsonOut(map[string]string{
			   "run_id": in.RunID,
			   "diff_kind": in.DiffKind,
			   "user_id":   in.UserID,
			   "note":      "escalated to human queue",
		   })
	  case ActKindNotify:
		  subj := fmt.Sprintf("[reconflow] 差异处置通知 diff=%s run=%s", in.DiffKind, in.RunID)
		  body := fmt.Sprintf("asset=%s user=%s actor=%s", in.Asset, in.UserID, in.Actor)
		   if ex.Notifier != nil {
			   if err := ex.Notifier.Notify(ctx, in.Actor, subj, body); err != nil {
				  res.ErrMsg = err.Error()
				   break
			  }
		   }
		  res.OK = true
		  res.Output = jsonOut(map[string]string{"notified": subj})
	   case ActKindCloseFlow:
		   res.OK = true
		  res.Output = jsonOut(map[string]string{"note": "flow closed by action"})
	  default:
		  res.ErrMsg = fmt.Sprintf("unknown action kind: %s", def.Kind)
	   }
	   return res
  }

// ──────────────────────────────────────────────
// FlowDef 内置配置工厂
// ──────────────────────────────────────────────
// DefaultFlowDefs 返回三种已知差异类型的默认流程定义。
// Engine 在无持久化配置时使用此函数引导。
   func DefaultFlowDefs() []FlowDef {
	   now := time.Now().UTC()
	   return []FlowDef{
		  {
			  ID:       uuid.NewString(),
			   Name:     "default-bridge-inflate",
			  DiffKind: DiffClassBridgeInflate,
			  SLA:      4 * time.Hour,
			   AutoClassify:  true,
			   MinConfidence: ConfidenceHigh,
			  ActionIDs:     []string{"builtin-open-ticket", "builtin-escalate-human"},
			   EscalateOnSLA: true,
			  RequestCircuit: false,
			  Enabled:     true,
			   Description: "桥账膨胀差异处置：开工单+升级人工",
			  CreatedBy:   "system",
			   CreatedAt: now,
			  UpdatedAt: now,
		   },
		   {
			   ID:       uuid.NewString(),
			  Name:     "default-hot-below-user",
			  DiffKind: DiffClassHotBelowUser,
			  SLA:          2 * time.Hour,
			  AutoClassify: true,
			  MinConfidence: ConfidenceMedium,
			   ActionIDs: []string{"builtin-open-ticket", "builtin-escalate-human"},
			  EscalateOnSLA: true,
			  RequestCircuit: true,
			   Enabled:        true,
			  Description: "热库不足差异处置：开工单+升级+SLA 超时请求熔断",
			   CreatedBy:   "system",
			  CreatedAt: now,
			   UpdatedAt: now,
		  },
		  {
			   ID:   uuid.NewString(),
			   Name: "default-deposit-short",
			   DiffKind: DiffClassDepositShort,
			  SLA:          8 * time.Hour,
			  AutoClassify: true,
			  MinConfidence: ConfidenceMedium,
			   ActionIDs:     []string{"builtin-open-ticket", "builtin-suggest-post", "builtin-notify"},
			  EscalateOnSLA: true,
			   RequestCircuit: false,
			  Enabled:        true,
			  Description:    "入金少打差异处置：开工单+建议记录+通知",
			   CreatedBy:      "system",
			  CreatedAt:      now,
			   UpdatedAt: now,
		  },
	  }
   }

// ──────────────────────────────────────────────
// 内部工具
// ──────────────────────────────────────────────

   func jsonOut(v any) string {
	   b, _ := json.Marshal(v)
	  return string(b)
   }

   func noteFromInput(in ExecInput, def ActionDef) string {
	   note := in.RunNote
	  if note == "" {
		   note = fmt.Sprintf("reconflow diff=%s run=%s", in.DiffKind, in.RunID)
	  }
	   if p, ok := in.Params["note"]; ok && p != "" {
		  note = p
	   }
	   _ = def
	  return note
   }

   func bookFromInput(in ExecInput) string {
	   if b, ok := in.Params["book"]; ok && b != "" {
		  return b
	  }
	  // 按差异类型推断账簿。
	  switch in.DiffKind {
	   case string(DiffClassBridgeInflate):
		  return "bridge"
	  case string(DiffClassHotBelowUser):
		  return "hot"
	   default:
		   return "control"
	  }
   }

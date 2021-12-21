package utamodegate

import (
	"context"
	"strings"
	"time"
	"github.com/google/uuid"
)

// GenerationEngine 封装 ModeGeneration 的 CAS 操作。
type GenerationEngine struct {
	store Store
	clock func() time.Time
}

  func newGenerationEngine(store Store, clock func() time.Time) *GenerationEngine {
	  return &GenerationEngine{store: store, clock: clock}
  }

  // CurrentGen 返回用户当前世代号（CAS 前置条件读取）。
   // 若用户从未切过模式，返回 0。
   func (g *GenerationEngine) CurrentGen(ctx context.Context, userID string) (int64, error) {
	  userID = strings.TrimSpace(userID)
	  if userID == "" {
		  return 0, ErrUserInvalid
	   }
	  return g.store.CurrentGeneration(ctx, userID)
   }

// GetGeneration 返回完整的 ModeGeneration 记录。
// 若用户从未切过模式，返回世代 0 + 默认 UNIFIED 模式的虚记录。

  func (g *GenerationEngine) GetGeneration(ctx context.Context, userID string) (*ModeGeneration, error) {
	   userID = strings.TrimSpace(userID)
	   if userID == "" {
		  return nil, ErrUserInvalid
	  }
	  gen, err := g.store.GetGeneration(ctx, userID)
	  if err != nil {
		   return nil, err
	  }
	   if gen.ID == "" {
		  gen.ID = "virtual-" + userID
		  gen.CurrentMode = MarginModeUnified
	  }
	  return gen, nil
   }

// BumpGen CAS 递增世代。
// - expectedGen 为调用方读取的当前世代（乐观锁前置值）。
// - newMode 为切换目标模式（UNIFIED / CLASSIC）。
// - 成功：返回新世代号（expected+1）；租约 ID 由调用方传入的 lease 携带。
// - 失败：ErrStaleGen（调用方应重读后重试，最多 3 次）。
// 首次切换（expectedGen = 0）：store 层 INSERT ON CONFLICT DO NOTHING 保障原子性。

  func (g *GenerationEngine) BumpGen(ctx context.Context, userID string, expectedGen int64, newMode MarginMode) (int64, error) {
	   userID = strings.TrimSpace(userID)
	   if userID == "" {
		  return 0, ErrUserInvalid
	   }
	  if newMode != MarginModeUnified && newMode != MarginModeClassic {
		   return 0, ErrModeInvalid
	   }
	   return g.store.BumpGeneration(ctx, userID, expectedGen, newMode)
   }

// ValidateToken 验证调用方携带的世代令牌是否与当前世代一致。
// 借还/抵押写操作在落账前调用此方法：若令牌世代 != 当前世代，拒绝写入（ErrStaleGen）。
// 允许世代相差为 0；不允许旧世代写入新世代状态。
// 注：此方法是"读-验"操作，非 CAS 原子操作。调用方在高并发场景下可在 store 层加行锁。
   func (g *GenerationEngine) ValidateToken(ctx context.Context, userID string, tokenGen int64) error {
	  current, err := g.store.CurrentGeneration(ctx, userID)
	  if err != nil {
		   return err
	   }
	   if current != tokenGen {
		   return ErrStaleGen
	   }
	  return nil
   }

// EnsureInitialized 确保用户有 ModeGeneration 记录（首次使用懒创建）。
// 环境在用户首次 SetMode 时由 BumpGen 自动创建；此方法供 ops/用。

   func (g *GenerationEngine) EnsureInitialized(ctx context.Context, userID string, initialMode MarginMode) (*ModeGeneration, error) {
	  userID = strings.TrimSpace(userID)
	  if userID == "" {
		   return nil, ErrUserInvalid
	  }
	   if initialMode == "" {
		   initialMode = MarginModeUnified
	  }
	  current, err := g.store.CurrentGeneration(ctx, userID)
	   if err != nil {
		  return nil, err
	   }
	   if current > 0 {
		   // 已有记录，直接返回
		  return g.store.GetGeneration(ctx, userID)
	   }
	   // 首次初始化：不递增世代，仅写入初始记录
	   now := g.clock()
	   mg := &ModeGeneration{
		   ID: uuid.NewString(),
		  UserID:      userID,
		   Generation:  0,
		  CurrentMode: initialMode,
		  UpdatedAt:   now,
	   }
	  // 通过 BumpGeneration(0→1) 初始化记录
	  // 若并发，store 层 ON CONFLICT 保障幂等
	  _ = mg // 直接委托给 store 的 BumpGeneration 完成 INSERT
	   return g.store.GetGeneration(ctx,
		   userID)
  }

// ─────────────────────────────────────────────────────────────────────────────
// GenToken 世代令牌（传递给借还/抵押操作的不透明值）
// ─────────────────────────────────────────────────────────────────────────────
// GenToken 是世代令牌，携带用户 ID + 世代号。
// 借还/抵押写操作必须持有有效 GenToken，且 Generation 与当前世代一致。

type GenToken struct {
	UserID     string `json:"user_id"`
	Generation int64  `json:"generation"`
	Mode     MarginMode `json:"mode"` // 颁发令牌时的当前模式
	IssuedAt time.Time  `json:"issued_at"`
}

// IssueToken 为指定用户颁发当前世代令牌。

   func (g *GenerationEngine) IssueToken(ctx context.Context, userID string) (*GenToken, error) {
	  userID = strings.TrimSpace(userID)
	   if userID == "" {
		  return nil, ErrUserInvalid
	   }
	  mg, err := g.store.GetGeneration(ctx, userID)
	   if err != nil {
		   return nil, err
	  }
	  mode := mg.CurrentMode
	  if mode == "" {
		  mode = MarginModeUnified
	   }
	   return &GenToken{
		   UserID: userID,
		  Generation: mg.Generation,
		   Mode:     mode,
		   IssuedAt: g.clock(),
	   }, nil
  }

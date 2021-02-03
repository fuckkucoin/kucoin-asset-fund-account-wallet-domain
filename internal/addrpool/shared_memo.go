package addrpool

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
	"account-assets.kucoin.net/funding/internal/persist"
	"account-assets.kucoin.net/funding/internal/sqlrepo"
	"account-assets.kucoin.net/funding/internal/wallet"
)

// SharedMemoBinding FamMEMO 共享地址 + 用户 memo/tag 绑定面。

type SharedMemoBinding struct {
	UserID  string `json:"user_id"`
	Chain   string `json:"chain"`
	Address string `json:"address"`
	Memo    string `json:"memo"`
	Tag string `json:"tag,omitempty"`
	TagName string `json:"tag_name,omitempty"`
	Shared bool `json:"shared"`
}

// MemoBinder 把用户 memo/tag 唯一绑到归因表（由 deposit.Attributor 实现）。

type MemoBinder interface {
	BindUnique(userID,
		chain, kind, key string) error
}

// ErrSeedEnsureRefused 拒绝种子 Ensure；须运营 Register。
var ErrSeedEnsureRefused = errors.New("fam shared address seed ensure refused in production; ops Register required")

// SharedMemoPool FamMEMO 链平台共享充值地址池。
// 同链多用户共用一地址，靠 memo/tag 归因；地址须 ValidChainAddress。
// 未登记共享地址不得现场派生串户；漏绑 memo 扫链无法入账。

type SharedMemoPool struct {
	mu      sync.Mutex
	byChain map[string]string // chain → address
	sql *persist.SQLDB
	clock func() time.Time
	derive     DeriveFunc // 仅用于非 Ensure 种子地址
	refuseSeed bool       // ：拒绝 Ensure 种子派生
	refuseBinder bool // ：BindUser 必须有归因 binder
}

// NewSharedMemoPool 构造 FamMEMO 共享地址池。
  func NewSharedMemoPool(derive DeriveFunc) *SharedMemoPool {
	  if derive == nil {
		   derive = wallet.DeriveAddress
	  }
	   return &SharedMemoPool{
		   byChain: map[string]string{},
		   derive: derive,
		   clock:  func() time.Time { return time.Now().UTC() },
	  }
  }

// RefuseSeedEnsure 打开后禁止 Ensure 用种子用户派生共享地址。
// 仅运营 Register / SQL Hydrate 可写共享地址。
// 未 Register 时用户领址失败关闭，不得静默派生。
  func (p *SharedMemoPool) RefuseSeedEnsure() *SharedMemoPool {
	   if p != nil {
		   p.refuseSeed = true
	   }
	  return p
  }

// SeedEnsureRefused 是否已拒种子 Ensure。
   func (p *SharedMemoPool) SeedEnsureRefused() bool {
	   return p != nil && p.refuseSeed
   }

// RefuseNilBinder 打开后 BindUser 必须注入归因 binder。
  func (p *SharedMemoPool) RefuseNilBinder() *SharedMemoPool {
	  if p != nil {
		   p.refuseBinder = true
	   }
	   return p
   }

   func (p *SharedMemoPool) BinderRequired() bool {
	   return p != nil && p.refuseBinder
  }

// BindSQL 挂 SQL；启动 Hydrate。

   func (p *SharedMemoPool) BindSQL(db *persist.SQLDB) *SharedMemoPool {
	  if p != nil {
		   p.sql = db
		   _ = p.Hydrate(context.Background())
	   }
	  return p
  }

  func (p *SharedMemoPool) sqlOn() bool {
	   return p != nil && p.sql != nil && p.sql.DB != nil
  }

// Hydrate 从 aa_fam_shared_addrs 回填内存。

   func (p *SharedMemoPool) Hydrate(ctx context.Context) error {
	   if p == nil || !p.sqlOn() {
		  return nil
	   }
	   rows, err := sqlrepo.NewFamShared(p.sql).List(ctx)
	  if err != nil {
		  return err
	  }
	  p.mu.Lock()
	   defer p.mu.Unlock()
	  for _, row := range rows {
		   p.byChain[row.Chain] = row.Address
	   }
	  return nil
   }

// Register 运营登记某链共享充值地址。
// 须过 ValidChainAddress；同地址不得两链；唯一写路径（ops）。
// 空/非法地址拒；不得静默用种子用户派生冒充。
   func (p *SharedMemoPool) Register(chain, address string) error {
	  if p == nil {
		   return fmt.Errorf("shared memo pool unavailable")
	   }
	   chain = strings.ToUpper(strings.TrimSpace(chain))
	   address = strings.TrimSpace(address)
	   if !wallet.RequiresFamMemo(chain) {
		  return fmt.Errorf("chain %s is not FamMEMO", chain)
	  }
	   if address == "" || wallet.ValidChainAddress(chain, address) != nil {
		   return fmt.Errorf("shared deposit address invalid for %s", chain)
	   }
	   p.mu.Lock()
	  defer p.mu.Unlock()
	   p.byChain[chain] = address
	  if p.sqlOn() {
		  if err := sqlrepo.NewFamShared(p.sql).Put(context.Background(), sqlrepo.FamSharedAddr{
			   Chain: chain, Address: address, Family: "MEMO", UpdatedAt: p.clock(),
		   }); err != nil {
			   delete(p.byChain, chain)
			   return err
		  }
	  }
	  return nil
   }

// Ensure 若未登记则用 seed 派生一次并落库（仅开发/）。
// RefuseSeedEnsure 后失败关闭，须运营 Register。
// 种子派生会串户/不可审计；拒绝后调用方不得降级猜地址。
  func (p *SharedMemoPool) Ensure(chain, seedUser string) (string, error) {
	  if p == nil {
		   return "", fmt.Errorf("shared memo pool unavailable")
	   }
	   chain = strings.ToUpper(strings.TrimSpace(chain))
	  if !wallet.RequiresFamMemo(chain) {
		   return "", fmt.Errorf("chain %s is not FamMEMO", chain)
	   }
	   if addr, ok := p.Address(chain); ok {
		  return addr, nil
	  }
	  if p.refuseSeed {
		  return "", fmt.Errorf("%w: chain=%s", ErrSeedEnsureRefused, chain)
	  }
	  seedUser = strings.TrimSpace(seedUser)
	  if seedUser == "" {
		   seedUser = "fam-shared"
	   }
	   addr := p.derive(seedUser, chain)
	  if err := p.Register(chain, addr); err != nil {
		   return "", err
	  }
	  return addr, nil
  }

// Address 只读取共享地址。
   func (p *SharedMemoPool) Address(chain string) (string, bool) {
	   if p == nil {
		  return "", false
	  }
	   chain = strings.ToUpper(strings.TrimSpace(chain))
	  p.mu.Lock()
	   addr, ok := p.byChain[chain]
	   p.mu.Unlock()
	   if ok && addr != "" {
		   return addr, true
	   }
	  if p.sqlOn() {
		   row, err := sqlrepo.NewFamShared(p.sql).Get(context.Background(), chain)
		   if err == nil && row.Address != "" {
			  p.mu.Lock()
			   p.byChain[chain] = row.Address
			   p.mu.Unlock()
			   return row.Address, true
		   }
	  }
	   return "", false
   }

// List 已登记共享地址列表（扫链）。
   func (p *SharedMemoPool) List() []SharedMemoBinding {
	   if p == nil {
		  return nil
	   }
	   p.mu.Lock()
	   defer p.mu.Unlock()
	  out := make([]SharedMemoBinding,
		   0, len(p.byChain))
	  for ch, addr := range p.byChain {
		   out = append(out, SharedMemoBinding{
			  Chain: ch, Address: addr, Shared: true, TagName: wallet.FamMemoTagName(ch),
		  })
	   }
	   return out
   }

// IsShared 地址是否为某 FamMEMO 共享充值地址。

  func (p *SharedMemoPool) IsShared(addr string) (chain string, ok bool) {
	  if p == nil {
		  return "", false
	  }
	  want := strings.ToLower(strings.TrimSpace(addr))
	   if want == "" {
		   return "", false
	   }
	  p.mu.Lock()
	   defer p.mu.Unlock()
	   for ch, a := range p.byChain {
		   if strings.ToLower(a) == want {
			   return ch, true
		   }
	   }
	  return "", false
  }

// BindUser 用户领 FamMEMO 充值面：共享地址 + 稳定 memo，并唯一绑归因。
// memo 由 MemoFor 派生；同时 BindUnique memo 与 tag（同值）。
// 未绑归因则扫链观察备注无法认领；绑定冲突拒。
  func (p *SharedMemoPool) BindUser(userID, chain, memo string, binder MemoBinder) (SharedMemoBinding, error) {
	  if p == nil {
		   return SharedMemoBinding{}, fmt.Errorf("shared memo pool unavailable")
	  }
	   userID = strings.TrimSpace(userID)
	  chain = strings.ToUpper(strings.TrimSpace(chain))
	  memo = strings.TrimSpace(memo)
	  if err := wallet.ValidFamMemoTag(chain, memo); err != nil {
		   return SharedMemoBinding{}, err
	  }
	   if userID == "" || chain == "" {
		  return SharedMemoBinding{}, fmt.Errorf("user_id and chain required")
	  }
	  if !wallet.RequiresFamMemo(chain) {
		   return SharedMemoBinding{}, fmt.Errorf("chain %s is not FamMEMO", chain)
	   }
	  if memo == "" {
		   return SharedMemoBinding{}, fmt.Errorf("memo required for shared FamMEMO bind")
	   }
	  if binder == nil && p.refuseBinder {
		  return SharedMemoBinding{}, fmt.Errorf("fam memo binder required")
	   }
	   addr, ok := p.Address(chain)
	   if !ok {
		  return SharedMemoBinding{}, fmt.Errorf("shared deposit address not registered for %s", chain)
	  }
	   tagName := wallet.FamMemoTagName(chain)
	   kind := "memo"
	   if tagName == "destination_tag" {
		   kind = "tag"
	   }
	   if binder != nil {
		   if err := binder.BindUnique(userID, chain, kind, memo); err != nil {
			   return SharedMemoBinding{}, err
		  }
		   other := "tag"
		  if kind == "tag" {
			   other = "memo"
		  }
		  if err := binder.BindUnique(userID, chain, other, memo); err != nil {
			   return SharedMemoBinding{}, err
		  }
	  }
	  return SharedMemoBinding{
		   UserID: userID, Chain: chain, Address: addr,
		   Memo: memo, Tag: memo, TagName: tagName, Shared: true,
	   }, nil
  }

// FamMemoChains 备注链清单别名（wallet.FamMemoChains）。

   func FamMemoChains() []string { return wallet.FamMemoChains() }

// RegisteredCount 已登记共享地址条数。
  func (p *SharedMemoPool) RegisteredCount() int {
	  if p == nil {
		   return 0
	   }
	  p.mu.Lock()
	   defer p.mu.Unlock()
	   n := 0
	   for _, addr := range p.byChain {
		   if strings.TrimSpace(addr) != "" {
			  n++
		   }
	   }
	   return n
   }

// MissingFamChains FamMEMO 链中尚未登记共享地址的清单。
// 就绪须至少登记一条；零登记失败关闭。
// 漏登记链用户领址失败；不得种子派生冒充。

   func (p *SharedMemoPool) MissingFamChains() []string {
	  out := make([]string, 0)
	  for _, ch := range wallet.FamMemoChains() {
		   if _, ok := p.Address(ch); !ok {
			  out = append(out, ch)
		   }
	   }
	  return out
  }

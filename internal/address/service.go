package address

import (
	"context"
	"fmt"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/addrpool"
	"account-assets.kucoin.net/funding/internal/ledger"
	"account-assets.kucoin.net/funding/internal/persist"
	"account-assets.kucoin.net/funding/internal/wallet"
)

type Service struct {
	led  *ledger.Service
	w    *wallet.Service
	pool *addrpool.Pool
	shared *addrpool.SharedMemoPool
	rec  *addrpool.RecycleBook
	kv   persist.KV
	attr interface {
		BindUnique(userID, chain, kind, key string) error
	}
}

   func New(led *ledger.Service, w *wallet.Service) *Service {
	   derive := wallet.DeriveAddress
	   if w != nil {
		   derive = func(userID, chain string) string {
			   return w.AddressFor(context.Background(), userID, chain)
		  }
	  }
	   return &Service{
		  led: led, w: w, pool: addrpool.New(derive, 24*time.Hour),
		  shared: addrpool.NewSharedMemoPool(derive),
		   rec: addrpool.NewRecycleBook(nil, 24*time.Hour),
	  }
  }

// BindKV 挂地址簿限流/幂等/标签覆盖。应绑 Runtime.KV。

  func (s *Service) BindKV(kv persist.KV) *Service {
	   if s != nil && kv != nil {
		  s.kv = kv
	   }
	   return s
  }

// BindRecycle 挂回收冷却簿。应绑 Runtime.KV，避免进程内 Mark 丢失。
  func (s *Service) BindRecycle(rec *addrpool.RecycleBook) *Service {
	   if s != nil && rec != nil {
		   s.rec = rec
	   }
	   return s
  }

// BindSharedMemo 注入 FamMEMO 共享池与用户级归因 binder（memo/tag 唯一占用）。

   func (s *Service) BindSharedMemo(shared *addrpool.SharedMemoPool, binder interface {
	  BindUnique(userID, chain, kind, key string) error
   }) *Service {
	  if s != nil {
		  if shared != nil {
			  s.shared = shared

  func (s *Service) BindSQL(db *persist.SQLDB) *Service {
	  if s != nil && s.pool != nil {
		   s.pool.BindSQL(db)
	  }
	  if s != nil && s.shared != nil {
		  s.shared.BindSQL(db)
	   }
	   return s
   }

// DepositAddress 分配并绑定充值地址。已绑定复用；形态不过闸则拒。
// FamMEMO：返回共享地址（调用方须再绑 memo）；非备注链走一对一池。
   func (s *Service) DepositAddress(ctx context.Context, userID, chain string) (string, error) {
	  if s == nil || s.pool == nil {
		  return "", fmt.Errorf("address wallet unavailable")
	  }
	  _ = ctx
	  chain = strings.ToUpper(strings.TrimSpace(chain))
	   if wallet.RequiresFamMemo(chain) && s.shared != nil {
		   if addr, ok := s.shared.Address(chain); ok {
			  return addr, nil
		   }
		   // 未登记时开发 Ensure； RefuseSeedEnsure 失败关闭，须运营 Register。
		  addr, err := s.shared.Ensure(chain, "fam-shared")
		   if err != nil {
			   return "", err
		   }
		  return addr, nil
	   }
	   b, err := s.pool.Allocate(userID, chain)
	   if err != nil {
		   return "", err
	   }
	   return b.Address, nil
  }

// BindFamMemo 用户领共享地址时绑定 memo/tag 归因。

  func (s *Service) BindFamMemo(userID, chain, memo string) (addrpool.SharedMemoBinding, error) {
	   if s == nil || s.shared == nil {
		  return addrpool.SharedMemoBinding{}, fmt.Errorf("shared memo pool unavailable")
	   }
	  return s.shared.BindUser(userID, chain, memo, s.attr)
   }

  func (s *Service) Binding(userID, chain string) (addrpool.Binding, bool) {
	   if s == nil || s.pool == nil {
		   return addrpool.Binding{}, false
	  }
	  return s.pool.Lookup(userID, chain)
   }

  func (s *Service) Bindings(userID string) []addrpool.Binding {
	   if s == nil || s.pool == nil {
		  return nil
	  }
	  return s.pool.List(userID)
   }

  func (s *Service) OwnerOf(addr string) (addrpool.Binding, bool) {
	  if s == nil || s.pool == nil {
		   return addrpool.Binding{}, false
	   }
	   return s.pool.ByAddress(addr)
   }

  func (s *Service) ListBook(ctx context.Context, userID string) ([]ledger.AddressBookEntry, error) {
	  if s == nil || s.led == nil {
		  return nil, fmt.Errorf("address ledger unavailable")
	   }
	   return s.led.ListAddressBook(ctx, userID)
  }

// 判定出金目标是否落在用户地址簿。地址全等；EVM 族与簿内 ETH/EVM 互认；停用条不计。WhitelistOnly 下未命中须回 260325，不得降级为仅形态通过。

  func (s *Service) Allowlisted(ctx context.Context, userID, chain, addr string) (bool, error) {
	   list, err := s.ListBook(ctx, userID)
	  if err != nil {
		  return false, err
	  }
	   chain = strings.ToUpper(strings.TrimSpace(chain))
	  addr = strings.TrimSpace(addr)
	   for _, e := range list {
		  if e.Disabled {
			  continue
		  }
		   if !addrExact(e.Address, addr) {
			   continue
		  }
		  if chainMatch(e.Chain, chain) {
			  return true, nil
		   }
	   }
	   return false, nil
   }

// 出金编排按地址簿回填 FamMEMO 备注与 tag 名。地址全等；EVM 族认 ETH/EVM 簿项。簿项无 memo 返回空串，禁止默认填充占位 memo。
   func (s *Service) BookMemo(ctx context.Context, userID, chain, addr string) (string, string, error) {
	  list, err := s.ListBook(ctx, userID)
	  if err != nil {
		  return "", "", err
	   }
	   chain = strings.ToUpper(strings.TrimSpace(chain))
	  addr = strings.TrimSpace(addr)
	   for _, e := range list {
		   if !addrExact(e.Address, addr) {
			   continue
		  }
		  if chainMatch(e.Chain, chain) {
			   return strings.TrimSpace(e.Memo), e.TagName, nil
		  }
	   }
	  return "", "", nil
   }

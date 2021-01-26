package addrpool

import (
	"context"
	"fmt"
	"strings"
)

// ctxBackground 池内部使用；Allocate 目前不接受 context 参数，钩子使用 Background。

   func ctxBackground() context.Context { return context.Background() }

// AllocateGuardHook 分配前防护检查接口。
// 由 dustguard.AllocateHook 实现。

type AllocateGuardHook interface {
	// Check 若地址处于隔离或高攻击分值状态，返回非 nil 错误，阻断分配。
	Check(ctx context.Context, chain, address, userID string) error
}

// BindGuardHook 挂载防护钩子到地址池。
		   p.guardHook = h
		  p.mu.Unlock()
	  }
   }

// ForceRelease 强制释放用户在某链的地址绑定（绕过冷却窗口）。
// 仅供 dustguard.Rotator 在轮换执行时调用，不对普通用户开放。
// 绑定不存在或未分配时静默成功。
   func (p *Pool) ForceRelease(userID, chain string) error {
	   if p == nil {
		   return fmt.Errorf("address pool unavailable")
	  }
	   k := key(userID, chain)
	   p.mu.Lock()
	   defer p.mu.Unlock()
	   b, ok := p.byKey[k]
	  if !ok || !b.Allocated {
		  return nil
	  }
	  delete(p.byAddr, strings.ToLower(b.Address))
	   b.Allocated = false
	   if err := p.persistRelease(userID, chain); err != nil {
		  b.Allocated = true
		   p.byAddr[strings.ToLower(b.Address)] = k
		   return err
	   }
	  return nil
  }

// ByAddress 接口适配：返回地址所属 (userID, chain, ok)，供 Rotator 查询旧地址 owner。

  func (p *Pool) ByAddressOwner(addr string) (userID, chain string, ok bool) {
	  b, found := p.ByAddress(addr)
	   if !found {
		   return "", "", false
	   }
	   return b.UserID, b.Chain, true
   }

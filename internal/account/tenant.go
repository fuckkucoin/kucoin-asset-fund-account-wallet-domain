package account

import (
	"context"
	"fmt"
	"strings"
)

type tenantCtxKey struct{}

// ErrTenant 同一用户已绑定租户后，另一个租户或空租户不得再动这本账。
var ErrTenant = fmt.Errorf("account tenant mismatch")

// WithTenant 把租户放进上下文。空字符串不写入。
func WithTenant(ctx context.Context, tenant string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	tenant = strings.TrimSpace(tenant)
	if tenant == "" {
		return ctx
	}
	return context.WithValue(ctx, tenantCtxKey{}, tenant)
}

// TenantFrom 读取上下文里的租户。没有则是空。
func TenantFrom(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	s, _ := ctx.Value(tenantCtxKey{}).(string)
	return strings.TrimSpace(s)
}

// admitTenant 把用户钉在第一次出现的租户上。
// 尚未钉住时，空租户保持原有单租户路径。钉住之后，空租户或其他租户都拒绝。
func (s *Service) admitTenant(userID, tenant string) error {
	if s == nil {
		return fmt.Errorf("account ledger unavailable")
	}
	userID = strings.TrimSpace(userID)
	tenant = strings.TrimSpace(tenant)
	if userID == "" {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.tenantOf == nil {
		s.tenantOf = map[string]string{}
	}
	prev, ok := s.tenantOf[userID]
	if tenant == "" {
		if ok {
			return fmt.Errorf("%w: tenant required", ErrTenant)
		}
		return nil
	}
	if !ok {
		s.tenantOf[userID] = tenant
		return nil
	}
	if prev != tenant {
		return fmt.Errorf("%w: %s", ErrTenant, userID)
	}
	return nil
}

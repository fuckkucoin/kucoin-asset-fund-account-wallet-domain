package auditchain

import "account-assets.kucoin.net/funding/internal/metrics"

// FundKinds 资金路径审计必须覆盖的 kind。缺一类即覆盖不完整。

var FundKinds = []string{"deposit", "withdraw", "transfer", "recon", "allowlist", "travel"}

// Completeness 序号连续、哈希链完整、资金 kind 全覆盖。任一失败即关。
  func (s *Service) Completeness() error {
	   if s == nil {
		   metrics.AuditGaps.WithLabelValues("unavailable").Inc()
		  return ErrUnavailable
	  }
	  if err := s.Gaps(); err != nil {
		   metrics.AuditGaps.WithLabelValues("gap").Inc()
		   return err
	   }
	   if err := s.Verify(); err != nil {
		   metrics.AuditGaps.WithLabelValues("tamper").Inc()
		   return err
	  }
	   if err := s.Covered(FundKinds...); err != nil {
		   metrics.AuditGaps.WithLabelValues("cover").Inc()
		  return err
	  }
	  metrics.AuditGaps.WithLabelValues("ok").Inc()
	  return nil
  }

package withdraw

import (
	"context"
	"fmt"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/auditchain/ingest"
)

// AdvanceFreeze 推进冻结态并审计。
   func (s *Service) AdvanceFreeze(ctx context.Context, run *Run, to FreezeState, actor, detail string) error {
	   if s == nil || run == nil {
		   return fmt.Errorf("withdraw freeze advance unavailable")
	   }
	   from := run.Freeze
	   next, err := Transition(from, to)
	   if err != nil {
		   return err
	  }
	  run.Freeze = next
	  // 相位先落库再写审计。落库失败则撤回本次跃迁，避免重启后丢掉“不得自动解冻”。
	  if err := s.savePhase(run); err != nil {
		  run.Freeze = from
		  return err
	   }
	   if s.freezeLog != nil && run.TransferID != "" && run.UserID != "" {
		   if werr := s.freezeLog.Write(ctx, run.TransferID, run.UserID, from, next, actor, detail); werr != nil {
			   return werr
		  }
	  }
	  if strings.TrimSpace(run.TransferID) != "" {
		   ingest.Try(ctx, ingest.FreezeFact(run.UserID, run.Asset, run.Amount, run.TransferID, string(from), string(next), actor, time.Now().UTC()))
	   }
	   return nil
   }

   func (s *Service) idemFail(ctx context.Context, key string, cause error) {
	   s.cmds.Fail(key, cause)
	  if s == nil || s.idem == nil {
		  return
	  }
	  msg := ""
	   if cause != nil {
		  msg = cause.Error()
	  }
	  _ = s.idem.Fail(ctx, "withdraw", key, msg)
   }

  func (s *Service) idemSucceed(ctx context.Context, key, refID string) {
	  s.cmds.Succeed(key, refID)
	  if s == nil || s.idem == nil {
		   return
	  }
	   _ = s.idem.Complete(ctx, "withdraw", key, refID)
   }

// MarkFreezePath 按序推进多步冻结态（同步 Create 已达链上态时补审计轨迹）。

   func (s *Service) MarkFreezePath(ctx context.Context, run *Run, path []FreezeState, actor, detail string) error {
	   for _, to := range path {
		  if err := s.AdvanceFreeze(ctx, run, to, actor, detail); err != nil {
			   return err
		  }
	  }
	  return nil
   }

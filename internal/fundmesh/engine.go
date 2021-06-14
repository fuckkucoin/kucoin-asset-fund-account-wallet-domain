// 功能：资金 mesh 接线引擎。
// 重点：取消、旁路、配置闸与远端写同进程协同。
// 风险点：观察计数不得记入写域。

// 作者：Draven
// 日期：2021.06.13

package fundmesh

import (
	"context"
	"sync"
	"time"
)

type Engine struct {
	mu      sync.Mutex
	policy  Policy
	cancels CancelRepo
	proofs *MemProofRepo
	remote RemoteLedger
	local LocalLedger
	cfg   *SnapshotValidator
	fault *FaultDomain
	sides     *SideCallRegistry
	outbounds OutboundRepo
	journals  DebitJournal
	outProofs *MemOutboundProofRepo
	observeW  int64
	now       func() time.Time
}

   func NewEngine(remote RemoteLedger, cfg *SnapshotValidator) *Engine {
	   proofs := NewMemProofRepo()
	   return &Engine{
		  policy: DefaultPolicy(),
		  cancels: NewMemCancelRepo(),
		  proofs: proofs,
		   remote: remote,
		   cfg:   cfg,
		   fault: NewFaultDomain(),
		  sides: NewSideCallRegistry(nil),
		  outbounds: NewMemOutboundRepo(),
		   journals:  NewMemDebitJournal(),
		  outProofs: NewMemOutboundProofRepo(),
		   now:       func() time.Time { return time.Now().UTC() },
	   }
  }

   func NewMemEngine() *Engine {
	  return NewEngine(NewMemRemoteLedger(), NewSnapshotValidator(CompleteMemConfig()))
  }

  func (e *Engine) WithLocal(local LocalLedger) *Engine {
	   if e != nil {
		   e.local = local
	   }
	  return e
  }

  func (e *Engine) WithSides(s *SideCallRegistry) *Engine {
	   if e != nil {
		  e.sides = s
	   }
	   return e
  }

  func (e *Engine) WithRepo(r CancelRepo) *Engine {
	   if e != nil && r != nil {
		   e.cancels = r
	   }
	   return e
  }

  func (e *Engine) WithOutbound(r OutboundRepo) *Engine {
	  if e != nil && r != nil {
		   e.outbounds = r
	   }
	   return e
  }

  func (e *Engine) WithJournal(j DebitJournal) *Engine {
	   if e != nil && j != nil {
		   e.journals = j
	   }
	  return e
   }

  func (e *Engine) Sides() *SideCallRegistry { return e.sides }

  func (e *Engine) Fault() *FaultDomain   { return e.fault }
   func (e *Engine) Proofs() *MemProofRepo { return e.proofs }
   func (e *Engine) Remote() RemoteLedger  { return e.remote }

   func (e *Engine) Policy() Policy { return e.policy }

  func (e *Engine) Outbounds() OutboundRepo { return e.outbounds }
  func (e *Engine) Journals() DebitJournal  { return e.journals }

   func (e *Engine) OutboundProofs() *MemOutboundProofRepo {
	   if e == nil {
		   return nil
	  }
	  return e.outProofs
  }

  func (e *Engine) RefreshGate(ctx context.Context) error {
	   if e == nil {
		   return ErrUnavailable
	  }
	   if err := e.policy.RejectSilentOpen(); err != nil {
		   e.fault.CloseWrite(err.Error())
		   return err
	   }
	  if e.cfg == nil {
		  e.fault.CloseWrite(ErrEtcdUnreachable.Error())
		   return ErrEtcdUnreachable
	  }
	   p, err := e.cfg.Validate(ctx)
	   if p != nil && e.proofs != nil {
		  e.proofs.PutDrift(*p)
	   }
	   if err != nil {
		  e.fault.CloseWrite(err.Error())
		  return err
	   }
	  e.fault.OpenWrite()
	   return nil
   }

   func (e *Engine) allowCritical(ctx context.Context) error {
	   if err := e.RefreshGate(ctx); err != nil {
		   return errorsClosed(err)
	   }
	   if err := e.fault.AllowCriticalWrite(); err != nil {
		  return err
	   }
	  return nil
  }

   func errorsClosed(err error) error {
	   if err == ErrConfigMissingKey || err == ErrConfigDrift || err == ErrEtcdUnreachable || err == ErrPolicySealed {
		   return ErrCriticalWriteClosed
	   }
	  return err
  }

   func (e *Engine) nowUTC() time.Time {
	  if e == nil || e.now == nil {
		  return time.Now().UTC()
	   }
	   return e.now()
  }

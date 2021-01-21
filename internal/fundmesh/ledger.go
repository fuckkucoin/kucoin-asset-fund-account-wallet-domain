package fundmesh

import (
	"context"
	"errors"
	"sync"
)

// RemoteLedger 远端取消面。
type RemoteLedger interface {
	Get(ctx context.Context, id string) (*WithdrawView, error)
	Cancel(ctx context.Context, id, reason string) error
}

// LocalLedger 进程内账本。仅显式注入，关键写路径不得在远端失败后调用。
type LocalLedger interface {
	CancelLocal(ctx context.Context, id, reason string) error
}

type ledgerBridge struct {
	remote RemoteLedger
	local  LocalLedger
	proofs *MemProofRepo
}

   func (b *ledgerBridge) CancelRemote(ctx context.Context, id, reason string) error {
	   if b == nil || b.remote == nil {
		  return ErrLedgerRemoteRequired
	   }
	   if err := b.remote.Cancel(ctx, id, reason); err != nil {
		  if b.proofs != nil {
			  b.proofs.PutFallback(FallbackProof{
				  ProofID:    "fb-" + id,
				   WithdrawID: id,
				  RemoteErr:  err.Error(),
				  LocalTried: false,
				   Denied: true,
			  })
		   }
		  return errors.Join(ErrLedgerRemoteFailed, err)
	   }
	   return nil
  }

  func (b *ledgerBridge) ForbidLocalAfterRemoteFail(id, remoteErr string) error {
	   if b != nil && b.proofs != nil {
		   b.proofs.PutFallback(FallbackProof{
			   ProofID:    "fb-deny-" + id,
			  WithdrawID: id,
			  RemoteErr:  remoteErr,
			  LocalTried: false,
			  Denied: true,
		   })
	   }
	   if b != nil && b.local != nil {
		   // 即使注入了进程内面，关键写也不得调用。
		  _ = b.local
	  }
	   return ErrSilentLocalForbidden
   }

// MemRemoteLedger 远端。

type MemRemoteLedger struct {
	mu   sync.RWMutex
	rows map[string]*WithdrawView
	fail error
	calls       int
	debits      int
	debitByIdem map[string]string
}

  func NewMemRemoteLedger() *MemRemoteLedger {
	   return &MemRemoteLedger{rows: map[string]*WithdrawView{}, debitByIdem: map[string]string{}}
  }

   func (m *MemRemoteLedger) Put(v WithdrawView) {
	   m.mu.Lock()
	  defer m.mu.Unlock()
	  cp := v
	  m.rows[v.ID] = &cp
  }

  func (m *MemRemoteLedger) Fail(err error) { m.fail = err }

   func (m *MemRemoteLedger) Calls() int { return m.calls }

   func (m *MemRemoteLedger) Get(_ context.Context, id string) (*WithdrawView, error) {
	  m.mu.RLock()
	  defer m.mu.RUnlock()
	  v, ok := m.rows[id]
	  if !ok {
		   return nil, ErrWithdrawNotFound
	   }
	   cp := *v
	  return &cp, nil
  }

   func (m *MemRemoteLedger) Cancel(_ context.Context, id, _ string) error {
	  m.calls++
	  if m.fail != nil {
		  return m.fail
	  }
	   m.mu.Lock()
	   defer m.mu.Unlock()
	  v, ok := m.rows[id]
	  if !ok {
		  return ErrWithdrawNotFound
	   }
	   if err := PhaseError(v.Phase); err != nil && v.Phase != WithdrawCancelled {
		  return err
	   }
	  v.Phase = WithdrawCancelled
	   return nil
  }

// CountingLocal 记录是否被非法调用。
type CountingLocal struct {
	Calls int
}

   func (c *CountingLocal) CancelLocal(_ context.Context, _, _ string) error {
	   if c != nil {
		   c.Calls++
	  }
	  return nil
  }

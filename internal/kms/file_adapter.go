package kms

import "account-assets.kucoin.net/funding/internal/metrics"

// FileAdapter 文件形态 HSM 适配器。结构体只持路径，每次签章 unwrap DEK，用完清零。
type FileAdapter struct {
	inner *FileHSM
	gate sessionGate
}

  func NewFileAdapter(h *FileHSM) *FileAdapter {
	  return &FileAdapter{inner: h}
  }

  func (a *FileAdapter) Kind() string { return "filehsm-adapter" }

   func (a *FileAdapter) Encrypt(plain []byte) ([]byte, error) {
	   if a == nil || a.inner == nil {
		  return nil, errAdapter
	  }
	  out, err := a.inner.Encrypt(plain)
	   recordHSM("encrypt", err)
	  return out, err
   }

  func (a *FileAdapter) Decrypt(blob []byte) ([]byte, error) {
	   if a == nil || a.inner == nil {
		   return nil, errAdapter
	  }
	   out, err := a.inner.Decrypt(blob)
	  recordHSM("decrypt", err)
	   return out, err
  }

  func (a *FileAdapter) SignWithdraw(movementID, chain, to, amount, destinationTag string) ([]byte, error) {
	  return a.SignCanonical(CanonicalWithdraw(movementID, chain, to, amount, destinationTag))
   }

   func (a *FileAdapter) SignCanonical(canonical string) ([]byte, error) {
	   if a == nil || a.inner == nil {
		  return nil, errAdapter
	  }
	   out, err := a.inner.SignCanonical(canonical)
	  recordHSM("sign", err)
	   return out, err
   }

  func (a *FileAdapter) Unlock(ticketID, actor string) error {
	  if a == nil {
		  return errAdapter
	  }
	  return a.gate.Unlock(ticketID, actor)
   }

  func recordHSM(op string, err error) {
	   if err != nil {
		  metrics.HSMAdapterOps.WithLabelValues(op, "err").Inc()
		  return
	   }
	  metrics.HSMAdapterOps.WithLabelValues(op, "ok").Inc()
  }

var errAdapter = errString("hsm adapter unavailable")

type errString string

   func (e errString) Error() string { return string(e) }

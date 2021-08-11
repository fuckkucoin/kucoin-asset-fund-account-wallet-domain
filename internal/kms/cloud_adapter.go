package kms

// CloudAdapter 走 kms-vault.kucoin.net HTTP 通道。进程不持 DEK。
type CloudAdapter struct {
	inner *CloudHSM
	gate  sessionGate
}

  func NewCloudAdapter(h *CloudHSM) *CloudAdapter { return &CloudAdapter{inner: h} }

   func (a *CloudAdapter) Kind() string { return "cloudhsm-adapter" }

   func (a *CloudAdapter) Encrypt(plain []byte) ([]byte, error) {
	   if a == nil || a.inner == nil {
		  return nil, errAdapter
	  }
	   out, err := a.inner.Encrypt(plain)
	   recordHSM("encrypt", err)
	   return out, err
   }

   func (a *CloudAdapter) Decrypt(blob []byte) ([]byte, error) {
	  if a == nil || a.inner == nil {
		   return nil, errAdapter
	   }
	  out, err := a.inner.Decrypt(blob)
	  recordHSM("decrypt", err)
	   return out, err
   }

  func (a *CloudAdapter) SignWithdraw(movementID, chain, to, amount, destinationTag string) ([]byte, error) {
	  return a.SignCanonical(CanonicalWithdraw(movementID, chain, to, amount, destinationTag))
   }

  func (a *CloudAdapter) SignCanonical(canonical string) ([]byte, error) {
	   if a == nil || a.inner == nil {
		   return nil, errAdapter
	   }
	   if err := a.gate.require(); err != nil {
		   return nil, err
	   }
	  var out []byte
	  err := retryOp("sign", func() error {
		   got, e := a.inner.SignCanonical(canonical)
		   if e != nil {
			   return e
		  }
		  out = got
		  return nil
	  })
	   if err == nil {
		  recordHSM("sign", nil)
	   }
	   return out, err
   }

  func (a *CloudAdapter) Unlock(ticketID, actor string) error {
	  if a == nil {
		   return errAdapter
	  }
	   return a.gate.Unlock(ticketID, actor)
  }

// CloudKMSAdapter 云 KMS 形态适配器。端点取 KMS_ENDPOINT 或配置。
type CloudKMSAdapter struct {
	*CloudAdapter
}

   func NewCloudKMSAdapter(h *CloudHSM) *CloudKMSAdapter {
	   return &CloudKMSAdapter{CloudAdapter: NewCloudAdapter(h)}
   }

  func (a *CloudKMSAdapter) Kind() string { return "cloud-kms-adapter" }

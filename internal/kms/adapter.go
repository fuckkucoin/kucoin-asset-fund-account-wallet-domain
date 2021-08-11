package kms

import (
	"fmt"
	"strings"
	"sync"
	"account-assets.kucoin.net/funding/internal/config"
	"account-assets.kucoin.net/funding/internal/metrics"
)

// Signer 签章管线只认这一面。材料必须是已经 freeze 的 canonical。

type Signer interface {
	SignCanonical(canonical string) ([]byte, error)
}

// Unlocker 双控解锁。未达票数不得开签。

type Unlocker interface {
	Unlock(ticketID, actor string) error
}

// Adapter HSM/KMS 签章与解锁面。签前必须 Unlock。

type Adapter interface {
	Keeper
	Signer
	Unlocker
	Kind() string
}

type sessionGate struct {
	mu sync.Mutex
	unlocked bool
	actor string
	ticket string
}

  func (g *sessionGate) Unlock(ticketID, actor string) error {
	   actor = strings.TrimSpace(actor)
	  ticketID = strings.TrimSpace(ticketID)
	   if actor == "" || strings.EqualFold(actor, "system") {
		   return fmt.Errorf("hsm unlock requires a distinct actor")
	  }
	   if ticketID == "" {
		  return fmt.Errorf("hsm unlock ticket required")
	  }
	   g.mu.Lock()
	   g.unlocked = true
	  g.actor = actor
	   g.ticket = ticketID
	   g.mu.Unlock()
	   metrics.HSMAdapterOps.WithLabelValues("unlock", "ok").Inc()
	  return nil
   }

  func (g *sessionGate) require() error {
	  g.mu.Lock()
	  defer g.mu.Unlock()
	   if !g.unlocked {
		  return fmt.Errorf("hsm session locked")
	   }
	  return nil
   }

  func OpenAdapter(kind string, cfg config.KMSConfig, bootstrap string) (Adapter, error) {
	   switch strings.ToLower(strings.TrimSpace(kind)) {
	  case "file", "filehsm":
		   h, err := OpenFile(cfg.KEKPath, cfg.WrapPath, bootstrap)
		   if err != nil {
			  return nil, err
		   }
		  return NewFileAdapter(h), nil
	  case "pkcs11", "pkcs11-adapter":
		  if cfg.Cluster != "" {
			  return OpenPKCS11Adapter(cfg.Cluster)
		   }
		   return OpenPKCS11FromEnv()
	   case "cloud-kms", "cloudkms":
		  h, err := OpenCloud(cfg)
		   if err != nil {
			  return nil, err
		   }
		  return NewCloudKMSAdapter(h),
			  nil
	   case "cloudhsm", "cloudhsm-adapter", "cloud-kms-adapter":
		   h, err := OpenCloud(cfg)
		   if err != nil {
			   return nil, err
		  }
		   return NewCloudAdapter(h), nil
	  default:
		   return nil, fmt.Errorf("kms kind required; file fallback refused")
	  }
  }

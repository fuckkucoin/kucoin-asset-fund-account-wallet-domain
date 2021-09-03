package kms

import (
	"os"
	"strings"
	"sync"
)

// PKCS11Settings 读 PKCS11_MODULE / PKCS11_SLOT / PKCS11_PIN。
// 档要求 MODULE 非空；空 MODULE 只给进程内 slot 表。

type PKCS11Settings struct {
	Module string
	Slot string
	PIN string
}

  func PKCS11FromEnv() PKCS11Settings {
	   return PKCS11Settings{
		  Module: strings.TrimSpace(os.Getenv("PKCS11_MODULE")),
		  Slot:   strings.TrimSpace(os.Getenv("PKCS11_SLOT")),
		  PIN: strings.TrimSpace(os.Getenv("PKCS11_PIN")),
	  }
   }

  func OpenPKCS11FromEnv() (*PKCS11Adapter, error) {
	   s := PKCS11FromEnv()
	   return OpenPKCS11Adapter(s.Slot)
   }

type SlotBook struct {
	mu    sync.Mutex
	slots map[string]*PKCS11Adapter
}

  func NewSlotBook() *SlotBook { return &SlotBook{slots: map[string]*PKCS11Adapter{}} }

  func (b *SlotBook) Open(slot string) (*PKCS11Adapter, error) {
	  if b == nil {
		  return nil, errAdapter
	  }
	   slot = strings.TrimSpace(slot)
	   if slot == "" {
		  slot = "slot-0"
	   }
	   b.mu.Lock()
	   defer b.mu.Unlock()
	   if a, ok := b.slots[slot]; ok {
		  return a, nil
	   }
	   a, err := OpenPKCS11Adapter(slot)
	  if err != nil {
		  return nil, err
	  }
	   b.slots[slot] = a
	  return a, nil
  }

  func (b *SlotBook) Unlock(slot, ticket, actor string) error {
	  a, err := b.Open(slot)
	   if err != nil {
		  return err
	  }
	  return a.Unlock(ticket, actor)
   }

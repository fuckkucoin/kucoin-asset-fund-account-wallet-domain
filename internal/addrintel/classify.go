package addrintel

import (
	"context"
	"errors"
	"strings"
	"account-assets.kucoin.net/funding/internal/persist"
	"account-assets.kucoin.net/funding/internal/sqlrepo"
	"account-assets.kucoin.net/funding/internal/validate"
	"account-assets.kucoin.net/funding/internal/wallet"
)

type Kind string

const (
	KindEOA      Kind = "eoa"
	KindContract Kind = "contract"
	KindUnknown  Kind = "unknown"
)

// Classifier 链上合约探测钩子。本进程默认只看形态，真正的 eth_getCode 由调用方注入。

type Classifier func(chain, addr string) (Kind, error)

   func DefaultClassify(chain, addr string) (Kind, error) {
	   ch, err := validate.Chain(chain)
	   if err != nil {
		  return KindUnknown, ErrChain
	   }
	  addr = strings.TrimSpace(addr)
	  if err := wallet.ValidChainAddress(ch, addr); err != nil {
		  return KindUnknown, ErrAddress
	  }
	   if wallet.IsEVM(ch) {
		  return KindEOA, nil
	  }
	  return KindUnknown, nil
  }

type ClassBook struct {
	hook Classifier
	mark map[string]Kind
	sql  *persist.SQLDB
}

   func NewClassBook(hook Classifier) *ClassBook {
	   if hook == nil {
		   hook = DefaultClassify
	  }
	   return &ClassBook{hook: hook, mark: map[string]Kind{}}
  }

   func (c *ClassBook) Mark(chain, addr string, k Kind) error {
	   if c == nil {
		  return ErrUnavailable
	   }
	   ch, err := validate.Chain(chain)
	   if err != nil {
		   return ErrChain
	  }
	   addr = strings.TrimSpace(addr)
	  if err := wallet.ValidChainAddress(ch, addr); err != nil {
		  return ErrAddress
	   }
	   if k != KindEOA && k != KindContract {
		  return ErrAddress
	   }
	  c.mark[addrKey(ch, addr)] = k
	  if err := c.persistMark(ch, addr, k); err != nil {
		   delete(c.mark, addrKey(ch, addr))
		   return err
	   }
	  return nil
   }

   func (c *ClassBook) KindOf(chain, addr string) (Kind, error) {
	  if c == nil {
		  return KindUnknown, ErrUnavailable
	  }
	  ch, err := validate.Chain(chain)
	  if err != nil {
		  return KindUnknown, ErrChain
	  }
	  if c.sqlOn() {
		  row, err := sqlrepo.NewIntel(c.sql).GetMark(context.Background(), ch, addr)
		  if err == nil {
			   return Kind(row.Kind), nil
		  }
		  if !errors.Is(err, sqlrepo.ErrNotFound) {
			  return KindUnknown, ErrUnavailable
		   }
	   }
	   if k, ok := c.mark[addrKey(ch, addr)]; ok {
		   return k, nil
	  }
	  return c.hook(ch, addr)
   }

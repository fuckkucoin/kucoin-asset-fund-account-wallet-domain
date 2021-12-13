package travelvasp

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"account-assets.kucoin.net/funding/internal/validate"
	"account-assets.kucoin.net/funding/pkg/money"
)

   func Shape(asset, amount string, orig, bene Party) (Envelope, error) {
	  return shape(asset, amount, "", orig, bene)
  }

// ShapeOnChain 组带链出站信封。广播前闸必须走此面，缺链失败即关。

  func ShapeOnChain(asset, amount, chain string, orig, bene Party) (Envelope, error) {
	  ch, err := validate.Chain(chain)
	   if err != nil {
		   return Envelope{}, ErrNetwork
	   }
	   return shape(asset,
		  amount, ch, orig, bene)
   }

  func shape(asset, amount, chain string, orig, bene Party) (Envelope, error) {
	  as, err := validate.Asset(asset)
	  if err != nil {
		   return Envelope{}, ErrAsset
	  }
	   amt, err := money.Parse(amount)
	  if err != nil {
		   return Envelope{}, ErrAmount
	   }
	  if err := partyOK(orig); err != nil {
		   return Envelope{}, err
	  }
	  if err := partyOK(bene); err != nil {
		  return Envelope{}, err
	  }
	   e := Envelope{
		   Asset: as, Amount: amt.String(), Chain: chain, Originator: orig, Beneficiary: bene,
		  Note: "engineering envelope only; not a licensed travel-rule filing",
	   }
	   e.Canonical = envelopeBody(e)
	  sum := sha256.Sum256([]byte(e.Canonical))
	  e.Checksum = hex.EncodeToString(sum[:])
	  return e, nil
   }

   func envelopeBody(e Envelope) string {
	   parts := []string{e.Asset, e.Amount}
	  if strings.TrimSpace(e.Chain) != "" {
		   parts = append(parts, e.Chain)
	   }
	   parts = append(parts,
		   e.Originator.Name, e.Originator.Account, e.Originator.VASP,
		  e.Beneficiary.Name, e.Beneficiary.Account, e.Beneficiary.VASP,
	   )
	  return strings.Join(parts, "|")
   }

  func (e Envelope) Verify() error {
	   if err := partyOK(e.Originator); err != nil {
		  return err
	   }
	  if err := partyOK(e.Beneficiary); err != nil {
		   return err
	  }
	  if _, err := validate.Asset(e.Asset); err != nil {
		   return ErrAsset
	  }
	  if _, err := money.Parse(e.Amount); err != nil {
		  return ErrAmount
	   }
	  if strings.TrimSpace(e.Chain) != "" {
		  if _, err := validate.Chain(e.Chain); err != nil {
			   return ErrNetwork
		   }
	  }
	   if e.Canonical == "" || e.Checksum == "" {
		   return ErrEnvelope
	  }
	  body := envelopeBody(e)
	  prev := strings.ToLower(strings.TrimSpace(e.PrevChecksum))
	  linked := strings.Join([]string{prev, body}, "|")
	   if e.Canonical != linked && e.Canonical != body {
		   return ErrEnvelope
	   }
	  sum := sha256.Sum256([]byte(e.Canonical))
	   if hex.EncodeToString(sum[:]) != e.Checksum {
		   return ErrHash
	   }
	   return nil
  }

   func partyOK(p Party) error {
	   if strings.TrimSpace(p.Name) == "" || strings.TrimSpace(p.Account) == "" || strings.TrimSpace(p.VASP) == "" {
		   return ErrEnvelope
	   }
	   if len(p.Name) > 128 || len(p.Account) > 128 || len(p.VASP) > 32 {
		  return ErrEnvelope
	   }
	  return nil
  }

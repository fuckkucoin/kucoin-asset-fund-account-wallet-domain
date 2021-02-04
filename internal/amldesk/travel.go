package amldesk

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"account-assets.kucoin.net/funding/pkg/money"
	"account-assets.kucoin.net/funding/internal/validate"
)

type Party struct {
	Name    string `json:"name"`
	Account string `json:"account"`
	VASP string `json:"vasp,omitempty"`
	Country string `json:"country,omitempty"`
	Address string `json:"address,omitempty"`
}

type Payload struct {
	CaseID string `json:"case_id,omitempty"`
	Asset  string `json:"asset"`
	Amount string `json:"amount"`
	Chain  string `json:"chain,omitempty"`
	TxHash      string `json:"tx_hash,omitempty"`
	Originator  Party  `json:"originator"`
	Beneficiary Party  `json:"beneficiary"`
	Canonical   string `json:"canonical"`
	Checksum    string `json:"checksum"`
	Note string `json:"note"`
}

// ShapeTravel 组旅行规则载荷。这是工程信封，不是申报件，不得当持牌存在。

   func ShapeTravel(asset, amount, chain, hash string, orig, bene Party) (Payload, error) {
	  as, err := validate.Asset(asset)
	   if err != nil {
		   return Payload{}, err
	  }
	   amt, err := money.Parse(amount)
	  if err != nil {
		   return Payload{}, err
	   }
	  if chain != "" {
		  if _, err := validate.Chain(chain); err != nil {
			  return Payload{}, err
		  }
	  }
	  if hash != "" {
		  if _, err := validate.TxHash(hash); err != nil {
			   return Payload{}, err
		   }
	   }
	   if err := partyOK(orig); err != nil {
		  return Payload{}, fmt.Errorf("%w: originator", err)
	  }
	   if err := partyOK(bene); err != nil {
		   return Payload{}, fmt.Errorf("%w: beneficiary", err)
	   }
	   p := Payload{
		  Asset: as, Amount: amt.String(), Chain: strings.ToUpper(strings.TrimSpace(chain)),
		   TxHash: hash, Originator: orig, Beneficiary: bene,
		   Note: "engineering envelope only; not a licensed travel-rule filing",
	   }
	   p.Canonical = strings.Join([]string{
		   p.Asset, p.Amount, p.Chain, p.TxHash,
		  orig.Name,
		  orig.Account, orig.VASP,
		   bene.Name, bene.Account, bene.VASP,
	   }, "|")
	  sum := sha256.Sum256([]byte(p.Canonical))
	   p.Checksum = hex.EncodeToString(sum[:])
	  return p, nil
  }

  func partyOK(p Party) error {
	   if strings.TrimSpace(p.Name) == "" || strings.TrimSpace(p.Account) == "" {
		  return ErrTravel
	   }
	  if len(p.Name) > 128 || len(p.Account) > 128 {
		   return ErrTravel
	  }
	  return nil
   }

   func (b *Book) BindTravel(caseID string, p Payload) error {
	   if b == nil {
		  return ErrUnavailable
	   }
	  if p.Checksum == "" || p.Canonical == "" {
		  return ErrTravel
	  }
	   sum := sha256.Sum256([]byte(p.Canonical))
	  if hex.EncodeToString(sum[:]) != p.Checksum {
		   return fmt.Errorf("%w: checksum", ErrTravel)
	   }
	  b.mu.Lock()
	  defer b.mu.Unlock()
	  r, ok := b.rows[strings.TrimSpace(caseID)]
	  if !ok {
		   return ErrNotFound
	   }
	  if !r.Open() {
		   return ErrClosed
	  }
	  p.CaseID = r.ID
	   b.tr[r.ID] = p
	   return nil
  }

   func (b *Book) Travel(caseID string) (Payload, error) {
	  b.mu.Lock()
	   defer b.mu.Unlock()
	   p, ok := b.tr[strings.TrimSpace(caseID)]
	  if !ok {
		   return Payload{}, ErrNotFound
	  }
	  return p, nil
  }

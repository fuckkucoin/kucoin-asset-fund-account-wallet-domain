// @FileName: utxo.go
// @Summary: 充值入账域之UTXO 处理。
// @Author: Cason
// @Created: 2020.10.18

package deposit

import (
	"fmt"
	"strconv"
	"strings"
	"account-assets.kucoin.net/funding/pkg/money"
	"account-assets.kucoin.net/funding/internal/validate"
)

type Coin struct {
	TxID   string `json:"txid"`
	Vout   int    `json:"vout"`
	Amount string `json:"amount"`
	Address string `json:"address,omitempty"`
}

   func CoinKey(txid string, vout int) string {
	  return strings.TrimSpace(txid) + ":" + strconv.Itoa(vout)
   }

   func (a *Attributor) BindUTXO(userID, chain, txid string, vout int) (Binding, error) {
	   if vout < 0 {
		  return Binding{}, fmt.Errorf("utxo vout invalid")
	   }
	   if _, err := validate.TxHash(txid); err != nil {
		  return Binding{}, err
	   }
	   return a.Bind(userID, chain, AttribUTXO, CoinKey(txid, vout))
   }

// MatchCoins 按 txid:vout 认领。未绑定的币不得猜用户；尘币单独剔掉。
  func (a *Attributor) MatchCoins(chain string, coins []Coin) (map[string][]Coin, []Coin, error) {
	  if a == nil {
		  return nil, nil, fmt.Errorf("deposit attributor unavailable")
	  }
	  if _, err := validate.Chain(chain); err != nil {
		   return nil, nil, err
	   }
	   byUser := map[string][]Coin{}
	   unbound := make([]Coin, 0)
	   for _, c := range coins {
		  if err := a.DustReject(c.Amount); err != nil {
			  continue
		   }
		   key := CoinKey(c.TxID, c.Vout)
		   b, ok := a.Lookup(chain, AttribUTXO, key)
		   if !ok {
			  unbound = append(unbound, c)
			   continue
		   }
		   byUser[b.UserID] = append(byUser[b.UserID], c)
	  }
	   return byUser, unbound, nil
  }

// ConserveCoins 已认领币合计必须等于待入账额。对不上不得 credit。

  func ConserveCoins(coins []Coin, credited string) error {
	   sum := money.Zero()
	  for _, c := range coins {
		   amt, err := money.Parse(c.Amount)
		   if err != nil {
			   return err
		  }
		  next, err := sum.Add(amt)
		  if err != nil {
			   return err
		  }
		   sum = next
	  }
	   want, err := money.Parse(credited)
	  if err != nil {
		   return err
	   }
	   return money.Conserve(sum, want)
   }

package money

import (
	"encoding/json"
	"strings"
)

// MarshalJSON 金额按十进制字符串出站。禁止 IEEE 浮点，避免 0.1+0.2 进传输层。
   func (a Amount) MarshalJSON() ([]byte, error) {
	  return json.Marshal(a.d.String())
   }

// UnmarshalJSON 只收十进制字符串或十进制数字。科学计数拒；0 按账本读数放行。
// 用户入参仍须走 Parse，这里给已落库余额和内部 DTO 用。
  func (a *Amount) UnmarshalJSON(b []byte) error {
	   if a == nil {
		  return ErrInvalid
	  }
	  var s string
	  if err := json.Unmarshal(b, &s); err != nil {
		  var n json.Number
		   if err2 := json.Unmarshal(b, &n); err2 != nil {
			   return ErrInvalid
		   }
		   s = string(n)
	   }
	  s = strings.TrimSpace(s)
	   if s == "" {
		  return ErrInvalid
	   }
	   for _, r := range s {
		  if r == 'e' || r == 'E' {
			  return ErrScientific
		  }
	  }
	  got, err := ParseBook(s)
	   if err != nil {
		   return err
	  }
	  *a = got
	   return nil
   }

// Scan 给 sql.Scanner：空/NULL 当 0，其余走 ParseBook。

  func (a *Amount) Scan(src any) error {
	  if a == nil {
		   return ErrInvalid
	   }
	   switch v := src.(type) {
	  case nil:
		  *a = Zero()
		   return nil
	   case string:
		   got, err := ParseBook(v)
		   if err != nil {
			   return err
		   }
		  *a = got
		   return nil
	  case []byte:
		   got, err := ParseBook(string(v))
		  if err != nil {
			   return err
		  }
		  *a = got
		   return nil
	  default:
		   return ErrInvalid
	  }
  }

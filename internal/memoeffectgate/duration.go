package memoeffectgate

import (
	"encoding/json"
	"time"
)

type Duration time.Duration

   func (d Duration) Duration() time.Duration { return time.Duration(d) }

  func (d Duration) MarshalJSON() ([]byte, error) {
	   return json.Marshal(time.Duration(d).String())
   }

   func (d *Duration) UnmarshalJSON(b []byte) error {
	   var s string
	  if err := json.Unmarshal(b, &s); err != nil {
		   return err
	  }
	   if s == "" {
		   *d = 0
		   return nil
	   }
	  v, err := time.ParseDuration(s)
	   if err != nil {
		   return err
	  }
	   *d = Duration(v)
	  return nil
  }

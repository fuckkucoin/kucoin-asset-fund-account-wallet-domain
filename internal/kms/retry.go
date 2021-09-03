package kms

import (
	"fmt"
	"time"
)

const adapterRetries = 3

   func retryOp(op string, fn func() error) error {
	  var last error
	  for i := 0; i < adapterRetries; i++ {
		   last = fn()
		  if last == nil {
			  return nil
		  }
		  if i+1 < adapterRetries {
			  time.Sleep(time.Duration(i+1) * 5 * time.Millisecond)
		   }
	  }
	  recordHSM(op, last)
	   return fmt.Errorf("%s retries exhausted: %w", op, last)
   }

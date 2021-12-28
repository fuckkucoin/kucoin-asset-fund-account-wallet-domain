package wdrelease

import "github.com/shopspring/decimal"

   func addDec(a, b string) (string, error) {
	   if a == "" {
		  a = "0"
	   }
	   if b == "" {
		  b = "0"
	   }
	   da, err := decimal.NewFromString(a)
	   if err != nil {
		  return "", err
	   }
	   db,
		   err := decimal.NewFromString(b)
	  if err != nil {
		   return "", err
	   }
	  return da.Add(db).String(), nil
   }

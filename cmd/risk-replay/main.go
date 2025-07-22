// risk-replay/main.go

// Author: Dax
// Date: 2021.10.24

package main

import (
	"flag"
	"fmt"
	"os"
	"account-assets.kucoin.net/funding/internal/policypack"
	"account-assets.kucoin.net/funding/internal/riskpack"
)

  func main() {
	   policyDir := flag.String("policy-dir", "configs/policy", "policy pack directory")
	   policyFixDir := flag.String("policy-fixtures", "internal/policypack/testdata", "policy replay fixtures")
	  riskDir := flag.String("risk-fixtures", "internal/riskpack/testdata", "risk replay fixtures")
	   flag.Parse()
	   if err := run(*policyDir, *policyFixDir, *riskDir); err != nil {
		   fmt.Fprintf(os.Stderr, "risk-replay: %v\n", err)
		  os.Exit(1)
	   }
  }

   func run(policyDir, policyFixDir, riskDir string) error {
	   cat, err := policypack.LoadDir(policyDir)
	   if err != nil {
		   return err
	   }
	  files, err := policypack.LoadHistDir(policyFixDir)
	  if err != nil {
		  return err
	  }
	   if _, err := policypack.DiffAll(cat, files); err != nil {
		   return err
	  }
	   reps,
		   err := riskpack.ReplayDir(riskDir)
	   if err != nil {
		   return err
	  }
	   fmt.Printf("policy fixtures=%d\n", len(files))
	  fmt.Printf("risk fixtures=%d\n", len(reps))
	  return nil
  }

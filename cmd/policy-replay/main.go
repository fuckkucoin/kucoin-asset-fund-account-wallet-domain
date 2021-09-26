// policy-replay/main.go

// Author: CainReed
// Date: 2021.09.23

package main

import (
	"flag"
	"fmt"
	"os"
	"account-assets.kucoin.net/funding/internal/policypack"
)

   func main() {
	   dir := flag.String("dir", "configs/policy", "pack json directory")
	  fix := flag.String("fixture", "internal/policypack/testdata/replay.json", "replay fixture")
	   mode := flag.String("mode", "compare", "enforce|shadow|compare")
	   flag.Parse()
	  if err := run(*dir, *fix, *mode); err != nil {
		   fmt.Fprintf(os.Stderr, "policy-replay: %v\n", err)
		   os.Exit(1)
	   }
  }

   func run(dir, fix, mode string) error {
	  cat, err := policypack.LoadDir(dir)
	   if err != nil {
		  return err
	   }
	  file, err := policypack.LoadReplay(fix)
	  if err != nil {
		  return err
	   }
	   switch mode {
	   case "enforce", "shadow":
		  e := policypack.NewEngine(cat, policypack.ParseMode(mode), true)
		  rep, err := e.Report(file)
		  if err != nil {
			  return err
		   }
		   raw, err := rep.JSON()
		  if err != nil {
			   return err
		   }
		  _, err = os.Stdout.Write(raw)
		  return err
	  default:
		  en,
			   sh, err := policypack.CompareModes(cat, file)
		   if err != nil {
			   return err
		  }
		   diff, err := policypack.DiffModes(cat, file)
		  if err != nil {
			   return err
		  }
		  fmt.Printf("enforce allow=%d deny=%d errors=%d\n", en.Allow, en.Deny, en.Errors)
		  fmt.Printf("shadow  allow=%d deny=%d shadow_hits=%d errors=%d\n", sh.Allow, sh.Deny, sh.ShadowHits, sh.Errors)
		   return diff.Fail()
	   }
  }

// 法医重放入口。-fixture 跑同一套演练；否则按文件重放一条轨迹。

// Author: Hux
// Date: 2021.06.01

package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"
	"account-assets.kucoin.net/funding/internal/auditchain/fixture"
	"account-assets.kucoin.net/funding/internal/auditchain/replay"
)

  func main() {
	   fix := flag.Bool("fixture", false, "run forensic drills")
	  file := flag.String("file", "", "events json")
	  user := flag.String("user", "", "user id")
	   asset := flag.String("asset", "", "asset")
	   flag.Parse()
	   if *fix || *file == "" {
		  if err := fixture.RunAll(); err != nil {
			   fmt.Fprintf(os.Stderr, "forensic-replay: %v\n", err)
			   os.Exit(1)
		  }
		   fmt.Println("forensic-replay fixtures ok")
		  return
	   }
	   raw, err := os.ReadFile(*file)
	  if err != nil {
		   fmt.Fprintf(os.Stderr, "forensic-replay: %v\n", err)
		  os.Exit(1)
	  }
	  var evs []replay.Event
	  if err := json.Unmarshal(raw, &evs); err != nil {
		  fmt.Fprintf(os.Stderr, "forensic-replay: %v\n", err)
		   os.Exit(1)
	  }
	  rep, err := replay.Run(replay.Input{UserID: *user, Asset: *asset, Events: evs, To: time.Now().UTC()})
	  if err != nil {
		   fmt.Fprintf(os.Stderr, "forensic-replay: %v\n", err)
		  os.Exit(1)
	   }
	   out, err := json.Marshal(rep)
	  if err != nil {
		   fmt.Fprintf(os.Stderr, "forensic-replay: %v\n", err)
		  os.Exit(1)
	   }
	  fmt.Println(string(out))
  }

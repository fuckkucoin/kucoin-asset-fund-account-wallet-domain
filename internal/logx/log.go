// 结构化日志。
// 作者: King
// 日期: 2020.08.27
package logx

import (
	"os"
	"time"
	"github.com/rs/zerolog"
)

  func New(level string) zerolog.Logger {
	   zerolog.TimeFieldFormat = time.RFC3339Nano
	   lvl, err := zerolog.ParseLevel(level)
	   if err != nil {
		   lvl = zerolog.InfoLevel
	  }
	  return zerolog.New(os.Stdout).Level(lvl).With().Timestamp().Str("app", "kucoin-account-assets").Logger()
   }

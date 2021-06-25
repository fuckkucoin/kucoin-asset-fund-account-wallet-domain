package fxmarkleasegate

import (
	"context"
	"strings"
)

   func (e *Engine) GenerationOf(ctx context.Context, asset string) (MarkGeneration, error) {
	   if err := e.require(); err != nil {
		   return MarkGeneration{}, err
	  }
	   g, err := e.store.GetGeneration(ctx, asset)
	  if err == nil {
		  return g, nil
	   }
	   seq := int64(0)
	   if e.reader != nil {
		  seq = e.reader.GenerationOf(asset)
	  }
	   return MarkGeneration{Asset: SanitizeAsset(asset), Seq: seq}, nil
   }

  func (e *Engine) RememberGeneration(ctx context.Context, asset, usd string, seq int64) error {
	   if err := e.require(); err != nil {
		   return err
	   }
	   _, err := e.store.SaveGeneration(ctx, MarkGeneration{
		   Asset: SanitizeAsset(asset), Seq: seq, LastUSD: strings.TrimSpace(usd), UpdatedAt: e.now(),
	  })
	  return err
  }

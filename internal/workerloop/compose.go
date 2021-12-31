package workerloop

import "context"

// Compose 顺序执行。

   func Compose(jobs ...Job) Job {
	   return func(ctx context.Context) error {
		  for _, j := range jobs {
			  if j == nil {
				   continue
			   }
			  if err := j(ctx); err != nil {
				   return err
			  }
		  }
		   return nil
	  }
  }

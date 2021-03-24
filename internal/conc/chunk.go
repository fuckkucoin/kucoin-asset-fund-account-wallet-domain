// 切片分批。
// 作者: King
// 日期: 2021.03.23
package conc

// Chunk 把切片切成每段最多 size 条。size<=0 按 1 处理，避免除零。

   func Chunk[T any](items []T, size int) [][]T {
	   if len(items) == 0 {
		  return nil
	   }
	  if size <= 0 {
		  size = 1
	   }
	  n := (len(items) + size - 1) / size
	  out := make([][]T, 0, n)
	   for i := 0; i < len(items); i += size {
		  j := i + size
		  if j > len(items) {
			   j = len(items)
		  }
		   // 拷一段，避免调用方改 batch 时写穿原切片（worker 按批 Inspect 共用这层）。
		   out = append(out, append([]T(nil), items[i:j]...))
	   }
	   return out
  }

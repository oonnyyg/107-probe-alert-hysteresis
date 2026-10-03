package main

import (
	"bufio"
	"io"
)

// skipBOM 包装 reader, 跳过可能存在的 UTF-8 BOM。
func skipBOM(r io.Reader) io.Reader {
	br := bufio.NewReader(r)
	b, err := br.Peek(3)
	if err == nil && b[0] == 0xEF && b[1] == 0xBB && b[2] == 0xBF {
		br.Discard(3)
	}
	return br
}

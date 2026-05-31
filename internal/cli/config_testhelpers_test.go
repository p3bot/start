package cli

import (
	"io"
	"strings"
)

// slowReader returns one byte per Read so sequential bufio.NewReader callers
// don't over-consume the shared underlying reader.
type slowReader struct{ r io.Reader }

func (s *slowReader) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	return s.r.Read(p[:1])
}

func slowStdin(data string) io.Reader {
	return &slowReader{r: strings.NewReader(data)}
}

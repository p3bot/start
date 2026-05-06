package cli

import (
	"io"
	"strings"
)

// slowReader wraps an io.Reader and returns one byte per Read call.
// This prevents bufio.NewReader from over-consuming the underlying reader when
// multiple sequential prompt functions each create their own bufio.NewReader.
type slowReader struct{ r io.Reader }

func (s *slowReader) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	return s.r.Read(p[:1])
}

// slowStdin returns a slowReader wrapping the given string as stdin.
func slowStdin(data string) io.Reader {
	return &slowReader{r: strings.NewReader(data)}
}

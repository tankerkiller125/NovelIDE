package ai

import (
	"bufio"
	"io"
	"strings"
)

// scanSSE parses a Server-Sent Events stream, calling handle for each event
// (dispatched on a blank line, per the SSE spec). event is "" when the stream
// carries only data lines (OpenAI); Anthropic sets it from "event:" lines.
// handle returns stop=true to end reading early (e.g. on "[DONE]").
func scanSSE(r io.Reader, handle func(event, data string) (stop bool, err error)) error {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 8*1024*1024) // tolerate large data lines

	var event string
	var data strings.Builder
	flush := func() (bool, error) {
		if data.Len() == 0 && event == "" {
			return false, nil
		}
		stop, err := handle(event, data.String())
		event = ""
		data.Reset()
		return stop, err
	}

	for sc.Scan() {
		line := sc.Text()
		switch {
		case line == "":
			if stop, err := flush(); stop || err != nil {
				return err
			}
		case strings.HasPrefix(line, ":"):
			// comment / keep-alive — ignore
		case strings.HasPrefix(line, "event:"):
			event = strings.TrimSpace(line[len("event:"):])
		case strings.HasPrefix(line, "data:"):
			if data.Len() > 0 {
				data.WriteByte('\n')
			}
			data.WriteString(strings.TrimSpace(line[len("data:"):]))
		}
	}
	if _, err := flush(); err != nil {
		return err
	}
	return sc.Err()
}

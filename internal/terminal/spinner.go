package terminal

import (
	"fmt"
	"io"
	"time"
)

type Spinner struct {
	writer      io.Writer
	interactive bool
	frames      []string
	message     string
	interval    time.Duration
	stop        chan struct{}
	done        chan struct{}
}

func NewSpinner(writer io.Writer, features Features, message string) *Spinner {
	frames := []string{"-", "\\", "|", "/"}
	if features.HasUnicode {
		frames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
	}
	colors := NewColorizer(features)

	return &Spinner{
		writer:      writer,
		interactive: features.IsTTY,
		frames:      frames,
		message:     colors.Muted(message),
		interval:    100 * time.Millisecond,
		stop:        make(chan struct{}),
		done:        make(chan struct{}),
	}
}

func (s *Spinner) Start() {
	if !s.interactive {
		close(s.done)
		return
	}

	go func() {
		defer close(s.done)

		ticker := time.NewTicker(s.interval)
		defer ticker.Stop()

		index := 0
		s.render(index)

		for {
			select {
			case <-s.stop:
				s.clear()
				return
			case <-ticker.C:
				index = (index + 1) % len(s.frames)
				s.render(index)
			}
		}
	}()
}

func (s *Spinner) Stop() {
	select {
	case <-s.done:
		return
	default:
	}

	if s.interactive {
		close(s.stop)
		<-s.done
		return
	}

	<-s.done
}

func Clear(writer io.Writer, features Features) {
	if !features.IsTTY {
		return
	}

	_, _ = fmt.Fprint(writer, "\033[H\033[2J")
}

func (s *Spinner) render(index int) {
	_, _ = fmt.Fprintf(s.writer, "\r%s %s", s.frames[index], s.message)
}

func (s *Spinner) clear() {
	_, _ = fmt.Fprint(s.writer, "\r\033[2K\r")
}

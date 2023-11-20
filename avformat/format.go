package avformat

import (
	"io"
	"time"
)

// Format is a registered format for a given media file type
type Format struct {
	Name      string
	Extension string // .flv, etc
	Probe     func(io.ReaderAt) (FileInfo, error)
}

// FileInfo represents various information about a file
type FileInfo interface {
	Format() Format
	Duration() time.Duration
	Streams() []Stream
}

package avformat

import "time"

type PacketType struct {
	Codec *Codec
	Name  string
}

type Packet struct {
	Type       *PacketType
	Stream     Stream
	IsKeyFrame bool
	CTime      time.Duration
	Time       time.Duration
	Data       []byte
	ASeqHdr    []byte
	VSeqHdr    []byte
	Metadata   []byte
}

type PacketReader interface {
	ReadPacket() (Packet, error)
}

type PacketReadCloser interface {
	PacketReader
	Close() error
}

type PacketWriter interface {
	WritePacket(Packet) error
}

type PacketWriteCloser interface {
	PacketWriter
	Close() error
}

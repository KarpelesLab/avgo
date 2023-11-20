package avformat

type Codec struct {
	Name   string
	Type   CodecType
	FourCC []string
}

type CodecType int

const (
	InvalidCodec CodecType = iota
	AudioCodec
	VideoCodec
	CaptionsCodec
)

var (
	codecByName   = make(map[string]*Codec)
	codecByFourCC = make(map[string]*Codec)
)

func RegisterCodec(codec *Codec) {
	codecByName[codec.Name] = codec
	for _, fcc := range codec.FourCC {
		codecByFourCC[fcc] = codec
	}
}

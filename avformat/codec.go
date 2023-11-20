package avformat

type Codec struct {
	Name string
}

var (
	codecByName = make(map[string]*Codec)
)

func RegisterCodec(codec *Codec) {
	codecByName[codec.Name] = codec
}

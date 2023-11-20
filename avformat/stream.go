package avformat

type Stream interface {
	Config() any
	Codec() *Codec
}

func StreamConfig[T any](s Stream) *T {
	if v, ok := s.Config().(*T); ok {
		return v
	}
	return nil
}

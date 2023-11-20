package flv

import (
	"io"
	"io/ioutil"

	"github.com/KarpelesLab/avgo/avformat"
	"github.com/KarpelesLab/avgo/codec/aac"
	"github.com/KarpelesLab/avgo/codec/h264"
	"github.com/KarpelesLab/avgo/format/flv/flvio"
)

const SetDataFrame = "@setDataFrame"
const OnMetaData = "onMetaData"

var flvMetadata = &avformat.PacketType{
	Name: "FLV Metadata",
}

func convertToAMF0Metadata(data []byte, amf3 bool) (newdata []byte) {
	arr, err := flvio.ParseAMFVals(data, amf3)
	if err != nil {
		return
	}

	if len(arr) == 0 {
		return
	}
	if s, _ := arr[0].(string); s == SetDataFrame {
		arr = arr[1:]
	}

	if len(arr) == 0 {
		return
	}
	if s, _ := arr[0].(string); s == OnMetaData {
		arr = arr[1:]
	} else {
		return
	}

	if len(arr) == 0 {
		arr = append(arr, flvio.AMFMap{})
	}
	newdata = make([]byte, flvio.FillAMF0Vals(nil, arr))
	flvio.FillAMF0Vals(newdata, arr)
	return
}

type Muxer struct {
	W              io.Writer
	b              []byte
	filehdrwritten bool
	HasVideo       bool
	HasAudio       bool
	Publishing     bool
	AAC            *aac.Codec
}

func NewMuxer(w io.Writer) *Muxer {
	m := &Muxer{
		W: w,
		b: make([]byte, 256),
	}
	return m
}

func (w *Muxer) WriteFileHeader() (err error) {
	if w.filehdrwritten {
		return
	}

	var flags uint8
	if w.HasVideo {
		flags |= flvio.FILE_HAS_VIDEO
	}
	if w.HasAudio {
		flags |= flvio.FILE_HAS_AUDIO
	}

	flvio.FillFileHeader(w.b, flags)
	if _, err = w.W.Write(w.b[:flvio.FileHeaderLength]); err != nil {
		return
	}
	w.filehdrwritten = true
	return
}

func (w *Muxer) WriteTag(tag flvio.Tag) (err error) {
	if err = w.WriteFileHeader(); err != nil {
		return
	}
	return flvio.WriteTag(w.W, tag, w.b)
}

func AACTagFromCodec(stream avformat.Stream) flvio.Tag {
	aac := avformat.StreamConfig[aac.Codec](stream)
	ch := 1
	if aac != nil {
		ch = aac.Config.ChannelLayout.Count()
	}
	tag := flvio.Tag{
		Type:        flvio.TAG_AUDIO,
		SoundFormat: flvio.SOUND_AAC,
		SoundRate:   flvio.SOUND_44Khz,
		SoundSize:   flvio.SOUND_16BIT,
	}
	switch ch {
	case 1:
		tag.SoundType = flvio.SOUND_MONO
	default:
		tag.SoundType = flvio.SOUND_STEREO
	}
	return tag
}

func (w *Muxer) WritePacket(pkt *avformat.Packet) (err error) {
	return WritePacket(pkt, w.WriteTag, w.Publishing)
}

func WritePacket(pkt *avformat.Packet, writeTag func(flvio.Tag) error, publishing bool) (err error) {
	switch pkt.Type {
	case aac.Packet:
		tag := AACTagFromCodec(pkt.Stream)
		tag.AACPacketType = flvio.AAC_RAW
		tag.Time = uint32(flvio.TimeToTs(pkt.Time))
		tag.Data = pkt.Data
		return writeTag(tag)

	case h264.Config:
		tag := flvio.Tag{
			Type:          flvio.TAG_VIDEO,
			FrameType:     flvio.FRAME_KEY,
			AVCPacketType: flvio.AVC_SEQHDR,
			VideoFormat:   flvio.VIDEO_H264,
			Data:          pkt.Data,
			Time:          uint32(flvio.TimeToTs(pkt.Time)),
		}
		return writeTag(tag)

	case h264.Packet:
		tag := flvio.Tag{
			Type:          flvio.TAG_VIDEO,
			AVCPacketType: flvio.AVC_NALU,
			VideoFormat:   flvio.VIDEO_H264,
			CTime:         int32(flvio.TimeToTs(pkt.CTime)),
		}
		if pkt.IsKeyFrame {
			tag.FrameType = flvio.FRAME_KEY
		} else {
			tag.FrameType = flvio.FRAME_INTER
		}
		tag.Time = uint32(flvio.TimeToTs(pkt.Time))
		tag.Data = pkt.Data
		return writeTag(tag)

	case aac.Config:
		tag := AACTagFromCodec(pkt.Stream)
		tag.AACPacketType = flvio.AAC_SEQHDR
		tag.Data = pkt.Data
		return writeTag(tag)

	case flvMetadata:
		arr, perr := flvio.ParseAMFVals(pkt.Data, false)
		if perr != nil {
			return
		}
		narr := []interface{}{}
		if publishing {
			narr = append(narr, SetDataFrame)
		}
		narr = append(narr, OnMetaData)
		narr = append(narr, arr...)
		tagdata := flvio.FillAMF0ValsMalloc(narr)
		tag := flvio.Tag{
			Type: flvio.TAG_AMF0,
			Data: tagdata,
			Time: uint32(flvio.TimeToTs(pkt.Time)),
		}
		return writeTag(tag)
	}

	return
}

type flvStream struct {
	config any
	codec  *avformat.Codec
}

type Demuxer struct {
	r          io.Reader
	b          []byte
	gotfilehdr bool
	Malloc     func(int) ([]byte, error)
	streams    []*flvStream

	LogHeaderEvent func(flags uint8)
}

func NewDemuxer(r io.Reader) *Demuxer {
	d := &Demuxer{
		r:       r,
		b:       make([]byte, 256),
		streams: make([]*flvStream, 2),
		Malloc: func(n int) ([]byte, error) {
			return make([]byte, n), nil
		},
	}
	return d
}

func (r *Demuxer) ReadFileHeader() (err error) {
	if r.gotfilehdr {
		return
	}
	if _, err = io.ReadFull(r.r, r.b[:flvio.FileHeaderLength]); err != nil {
		return
	}
	var flags uint8
	var skip int
	if flags, skip, err = flvio.ParseFileHeader(r.b); err != nil {
		return
	}
	if r.LogHeaderEvent != nil {
		r.LogHeaderEvent(flags)
	}
	if _, err = io.CopyN(ioutil.Discard, r.r, int64(skip)); err != nil {
		return
	}
	r.gotfilehdr = true
	return
}

func (r *Demuxer) ReadTag() (tag flvio.Tag, err error) {
	if err = r.ReadFileHeader(); err != nil {
		return
	}
	if tag, err = flvio.ReadTag(r.r, r.b, r.Malloc); err != nil {
		return
	}
	return
}

func (r *Demuxer) ReadPacket() (pkt *avformat.Packet, err error) {
	return ReadPacket(r.ReadTag)
}

func ReadPacket(readTag func() (flvio.Tag, error)) (pkt *avformat.Packet, err error) {
	for {
		var tag flvio.Tag
		if tag, err = readTag(); err != nil {
			return
		}

		// TODO fill Packet.Stream

		switch tag.Type {
		case flvio.TAG_AMF0, flvio.TAG_AMF3:
			data := convertToAMF0Metadata(tag.Data, tag.Type == flvio.TAG_AMF3)
			if data != nil {
				pkt = &avformat.Packet{
					Type: flvMetadata,
					Data: data,
					Time: flvio.TsToTime(int64(tag.Time)),
				}
				return
			}

		case flvio.TAG_VIDEO:
			switch tag.VideoFormat {
			case flvio.VIDEO_H264:
				switch tag.AVCPacketType {
				case flvio.AVC_SEQHDR:
					// header (config) packet
					pkt = &avformat.Packet{
						Type: h264.Config,
						Data: tag.Data,
					}
					return
				case flvio.AVC_NALU:
					pkt = &avformat.Packet{
						Type:       h264.Packet,
						Data:       tag.Data,
						Time:       flvio.TsToTime(int64(tag.Time)),
						CTime:      flvio.TsToTime(int64(tag.CTime)),
						IsKeyFrame: tag.FrameType == flvio.FRAME_KEY,
					}
					return
				}
			}

		case flvio.TAG_AUDIO:
			switch tag.SoundFormat {
			case flvio.SOUND_AAC:
				switch tag.AACPacketType {
				case flvio.AAC_SEQHDR:
					pkt = &avformat.Packet{
						Type: aac.Config,
						Data: tag.Data,
					}
					return
				case flvio.AAC_RAW:
					pkt = &avformat.Packet{
						Type: aac.Packet,
						Data: tag.Data,
						Time: flvio.TsToTime(int64(tag.Time)),
					}
					return
				}
			}
		}
	}
}

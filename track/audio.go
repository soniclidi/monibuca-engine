package track

import (
	"net"

	"m7s.live/engine/v4/codec"
	. "m7s.live/engine/v4/common"
	"m7s.live/engine/v4/config"
)

var adcflv1 = []byte{codec.FLV_TAG_TYPE_AUDIO, 0, 0, 4, 0, 0, 0, 0, 0, 0, 0}
var adcflv2 = []byte{0, 0, 0, 15}

type Audio struct {
	Media[AudioSlice]
	CodecID  codec.AudioCodecID
	Channels byte
	AVCCHead []byte // 音频包在AVCC格式中，AAC会有两个字节，其他的只有一个字节
	// Profile:
	// 0: Main profile
	// 1: Low Complexity profile(LC)
	// 2: Scalable Sampling Rate profile(SSR)
	// 3: Reserved
	Profile byte
}

func (a *Audio) IsAAC() bool {
	return a.CodecID == codec.CodecID_AAC
}
func (a *Audio) GetDecConfSeq() int {
	return a.DecoderConfiguration.Seq
}
func (a *Audio) Attach() {
	a.Stream.AddTrack(a)
}
func (a *Audio) Detach() {
	a.Stream = nil
	a.Stream.RemoveTrack(a)
}
func (a *Audio) GetName() string {
	if a.Name == "" {
		return a.CodecID.String()
	}
	return a.Name
}
func (a *Audio) GetInfo() *Audio {
	return a
}

func (a *Audio) WriteADTS(adts []byte) {
	a.Profile = ((adts[2] & 0xc0) >> 6) + 1
	sampleRate := (adts[2] & 0x3c) >> 2
	channel := ((adts[2] & 0x1) << 2) | ((adts[3] & 0xc0) >> 6)
	config1 := (a.Profile << 3) | ((sampleRate & 0xe) >> 1)
	config2 := ((sampleRate & 0x1) << 7) | (channel << 3)
	a.SampleRate = uint32(codec.SamplingFrequencies[sampleRate])
	a.Channels = channel
	avcc := []byte{0xAF, 0x00, config1, config2}
	a.DecoderConfiguration = DecoderConfiguration[AudioSlice]{
		97,
		net.Buffers{avcc},
		avcc[2:],
		net.Buffers{adcflv1, avcc, adcflv2},
		0,
	}
	a.Attach()
}

func (a *Audio) Flush() {
	// AVCC 格式补完
	value := a.Media.RingBuffer.Value
	if len(value.AVCC) == 0 && (config.Global.EnableAVCC || config.Global.EnableFLV) {
		value.AppendAVCC(a.AVCCHead)
		for _, raw := range value.Raw {
			value.AppendAVCC(raw)
		}
	}
	// FLV tag 补完
	if len(value.FLV) == 0 && config.Global.EnableFLV {
		value.FillFLV(codec.FLV_TAG_TYPE_AUDIO, value.AbsTime)
	}
	if value.RTP == nil && config.Global.EnableRTP {
		var o []byte
		for _, raw := range value.Raw {
			o = append(o, raw...)
		}
		a.PacketizeRTP(o)
	}
	a.Media.Flush()
}

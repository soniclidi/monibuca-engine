package track

import (
	"net"
	"time"

	"github.com/pion/rtp"
	"go.uber.org/zap"
	"m7s.live/engine/v4/codec"
	. "m7s.live/engine/v4/common"
	"m7s.live/engine/v4/config"
	"m7s.live/engine/v4/util"
)

func NewAAC(stream IStream) (aac *AAC) {
	aac = &AAC{}
	aac.Audio.Name = "aac"
	aac.Audio.Stream = stream
	aac.CodecID = codec.CodecID_AAC
	aac.Init(32)
	aac.Audio.Media.Poll = time.Millisecond * 10
	aac.AVCCHead = []byte{0xAF, 1}
	aac.Audio.SampleSize = 16
	aac.Audio.DecoderConfiguration.PayloadType = 97
	if config.Global.RTPReorder {
		aac.Audio.orderQueue = make([]*RTPFrame, 20)
	}
	return
}

type AAC struct {
	Audio
}

// WriteRTPPack 写入已反序列化的RTP包
func (aac *AAC) WriteRTPPack(p *rtp.Packet) {
	for frame := aac.UnmarshalRTPPacket(p); frame != nil; frame = aac.nextRTPFrame() {
		aac.writeRTPFrame(frame)
	}
}

// WriteRTP 写入未反序列化的RTP包
func (aac *AAC) WriteRTP(raw []byte) {
	for frame := aac.UnmarshalRTP(raw); frame != nil; frame = aac.nextRTPFrame() {
		aac.writeRTPFrame(frame)
	}
}

func (aac *AAC) writeRTPFrame(frame *RTPFrame) {
	for _, payload := range codec.ParseRTPAAC(frame.Payload) {
		aac.WriteSlice(payload)
	}
	aac.Audio.Media.AVRing.RingBuffer.Value.AppendRTP(frame)
	if frame.Marker {
		aac.generateTimestamp()
		aac.Flush()
	}
}

func (aac *AAC) WriteAVCC(ts uint32, frame AVCCFrame) {
	if frame.IsSequence() {
		if len(frame) < 2 {
			aac.Audio.Stream.Error("AVCC sequence header too short", zap.ByteString("data", frame))
			return
		}
		aac.Audio.DecoderConfiguration.AVCC = net.Buffers{frame}
		config1, config2 := frame[2], frame[3]
		aac.Profile = (config1 & 0xF8) >> 3
		aac.Channels = ((config2 >> 3) & 0x0F) //声道
		aac.Audio.SampleRate = uint32(codec.SamplingFrequencies[((config1&0x7)<<1)|(config2>>7)])
		aac.Audio.DecoderConfiguration.Raw = AudioSlice(frame[2:])
		aac.Audio.DecoderConfiguration.FLV = net.Buffers{adcflv1, frame, adcflv2}
		aac.Attach()
	} else {
		aac.Audio.WriteAVCC(ts, frame)
		aac.Flush()
	}
}

func (aac *AAC) Flush() {
	// RTP格式补完
	// TODO: MTU 分割
	value := aac.Audio.Media.RingBuffer.Value
	if value.RTP == nil && config.Global.EnableRTP {
		l := util.SizeOfBuffers(value.Raw)
		o := make([]byte, 4, l+4)
		//AU_HEADER_LENGTH,因为单位是bit, 除以8就是auHeader的字节长度；又因为单个auheader字节长度2字节，所以再除以2就是auheader的个数。
		o[0] = 0x00 //高位
		o[1] = 0x10 //低位
		//AU_HEADER
		o[2] = (byte)((l & 0x1fe0) >> 5) //高位
		o[3] = (byte)((l & 0x1f) << 3)   //低位
		for _, raw := range value.Raw {
			o = append(o, raw...)
		}
		aac.PacketizeRTP(o)
	}
	aac.Audio.Flush()
}

package common

import (
	"net"
	"time"

	"github.com/pion/rtp"
	"m7s.live/engine/v4/codec"
	"m7s.live/engine/v4/log"
	"m7s.live/engine/v4/util"
)

type NALUSlice net.Buffers

// type H264Slice NALUSlice
// type H265Slice NALUSlice

// type H264NALU []H264Slice
// type H265NALU []H265Slice

type AudioSlice []byte

// type AACSlice AudioSlice
// type G711Slice AudioSlice

// 裸数据片段
type RawSlice interface {
	~[][]byte | ~[]byte
}

// func (nalu *H264NALU) Append(slice ...NALUSlice) {
// 	*nalu = append(*nalu, slice...)
// }
func (nalu NALUSlice) H264Type() (naluType codec.H264NALUType) {
	return naluType.Parse(nalu[0][0])
}
func (nalu NALUSlice) RefIdc() byte {
	return nalu[0][0] & 0x60
}
func (nalu NALUSlice) H265Type() (naluType codec.H265NALUType) {
	return naluType.Parse(nalu[0][0])
}
func (nalu NALUSlice) Bytes() (b []byte) {
	for _, slice := range nalu {
		b = append(b, slice...)
	}
	return
}

func (nalu *NALUSlice) Reset() *NALUSlice {
	if len(*nalu) > 0 {
		*nalu = (*nalu)[:0]
	}
	return nalu
}

func (nalu *NALUSlice) Append(b ...[]byte) {
	*nalu = append(*nalu, b...)
}

// func (nalu *H265NALU) Append(slice ...NALUSlice) {
// 	*nalu = append(*nalu, slice...)
// }
// func (nalu H265NALU) IFrame() bool {
// 	switch H265Slice(nalu[0]).Type() {
// 	case codec.NAL_UNIT_CODED_SLICE_BLA,
// 		codec.NAL_UNIT_CODED_SLICE_BLANT,
// 		codec.NAL_UNIT_CODED_SLICE_BLA_N_LP,
// 		codec.NAL_UNIT_CODED_SLICE_IDR,
// 		codec.NAL_UNIT_CODED_SLICE_IDR_N_LP,
// 		codec.NAL_UNIT_CODED_SLICE_CRA:
// 		return true
// 	}
// 	return false
// }

type AVCCFrame []byte   // 一帧AVCC格式的数据
type AnnexBFrame []byte // 一帧AnnexB格式数据
type RTPFrame struct {
	rtp.Packet
	Raw []byte // 序列化后的数据，避免反复序列化
}

func (rtp *RTPFrame) H264Type() (naluType codec.H264NALUType) {
	return naluType.Parse(rtp.Payload[0])
}
func (rtp *RTPFrame) H265Type() (naluType codec.H265NALUType) {
	return naluType.Parse(rtp.Payload[0])
}
func (rtp *RTPFrame) Marshal() *RTPFrame {
	rtp.Raw, _ = rtp.Packet.Marshal()
	return rtp
}

func (rtp *RTPFrame) Unmarshal(raw []byte) *RTPFrame {
	rtp.Raw = raw
	if err := rtp.Packet.Unmarshal(raw); err != nil {
		log.Error(err)
		return nil
	}
	return rtp
}

type BaseFrame struct {
	DeltaTime uint32    // 相对上一帧时间戳，毫秒
	AbsTime   uint32    // 绝对时间戳，毫秒
	Timestamp time.Time // 写入时间,可用于比较两个帧的先后
	Sequence  uint32    // 在一个Track中的序号
	BytesIn   int       // 输入字节数用于计算BPS
}

type DataFrame[T any] struct {
	BaseFrame
	Value T
}
type MediaFrame interface {
	GetFLV() net.Buffers
	GetAVCC() net.Buffers
	GetRTP() []*RTPFrame
}
type AVFrame[T RawSlice] struct {
	BaseFrame
	IFrame  bool
	PTS     uint32
	DTS     uint32
	FLV     net.Buffers // 打包好的FLV Tag
	AVCC    net.Buffers // 打包好的AVCC格式
	RTP     []*RTPFrame
	Raw     []T // 裸数据
	canRead bool
}

func (av *AVFrame[T]) AppendRaw(raw ...T) {
	av.Raw = append(av.Raw, raw...)
}
func (av *AVFrame[T]) FillFLV(t byte, ts uint32) {
	var bytes []byte
	if cap(av.FLV) > 0 {
		if flv := av.FLV[:1]; cap(flv[0]) == 15 {
			bytes = flv[0][:0]
		}
	}
	if bytes == nil {
		bytes = make([]byte, 0, 15)
	}
	b := util.Buffer(bytes)
	b.WriteByte(t)
	dataSize := util.SizeOfBuffers(av.AVCC)
	b.WriteUint24(uint32(dataSize))
	b.WriteUint24(ts)
	b.WriteByte(byte(ts >> 24))
	b.WriteUint24(0)
	av.FLV = append(append(append(av.FLV, b), av.AVCC...), util.PutBE(b.Malloc(4), dataSize+11))
}
func (av *AVFrame[T]) AppendAVCC(avcc ...[]byte) {
	av.AVCC = append(av.AVCC, avcc...)
}
func (av *AVFrame[T]) AppendRTP(rtp ...*RTPFrame) {
	av.RTP = append(av.RTP, rtp...)
}

func (av *AVFrame[T]) Reset() {
	if av.FLV != nil {
		av.FLV = av.FLV[:0]
	}
	if av.AVCC != nil {
		av.AVCC = av.AVCC[:0]
	}
	av.RTP = nil
	av.Raw = nil
}

func (avcc AVCCFrame) IsIDR() bool {
	v := avcc[0] >> 4
	return v == 1 || v == 4 //generated keyframe
}
func (avcc AVCCFrame) IsSequence() bool {
	return avcc[1] == 0
}
func (avcc AVCCFrame) CTS() uint32 {
	return uint32(avcc[2])<<24 | uint32(avcc[3])<<8 | uint32(avcc[4])
}
func (avcc AVCCFrame) VideoCodecID() codec.VideoCodecID {
	return codec.VideoCodecID(avcc[0] & 0x0F)
}
func (avcc AVCCFrame) AudioCodecID() codec.AudioCodecID {
	return codec.AudioCodecID(avcc[0] >> 4)
}

// func (annexb AnnexBFrame) ToSlices() (ret []NALUSlice) {
// 	for len(annexb) > 0 {
// 		before, after, found := bytes.Cut(annexb, codec.NALU_Delimiter1)
// 		if !found {
// 			return append(ret, NALUSlice{annexb})
// 		}
// 		if len(before) > 0 {
// 			ret = append(ret, NALUSlice{before})
// 		}
// 		annexb = after
// 	}
// 	return
// }
// func (annexb AnnexBFrame) ToNALUs() (ret [][]NALUSlice) {
// 	for len(annexb) > 0 {
// 		before, after, found := bytes.Cut(annexb, codec.NALU_Delimiter1)
// 		if !found {
// 			return append(ret, annexb.ToSlices())
// 		}
// 		if len(before) > 0 {
// 			ret = append(ret, AnnexBFrame(before).ToSlices())
// 		}
// 		annexb = after
// 	}
// 	return
// }
type DecoderConfiguration[T RawSlice] struct {
	PayloadType byte
	AVCC        net.Buffers
	Raw         T
	FLV         net.Buffers
	Seq         int //收到第几个序列帧，用于变码率时让订阅者发送序列帧
}

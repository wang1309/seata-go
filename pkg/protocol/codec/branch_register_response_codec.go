package codec

import (
	"github.com/fagongzi/goetty"
)

import (
	"github.com/seata/seata-go/pkg/protocol/message"
	"github.com/seata/seata-go/pkg/protocol/transaction"
)

func init() {
	GetCodecManager().RegisterCodec(CodeTypeSeata, &BranchRegisterResponseCodec{})
}

type BranchRegisterResponseCodec struct {
}

func (g *BranchRegisterResponseCodec) Decode(in []byte) interface{} {
	buf := goetty.NewByteBuf(len(in))
	buf.Write(in)
	msg := message.BranchRegisterResponse{}

	resultCode := ReadByte(buf)
	msg.ResultCode = message.ResultCode(resultCode)
	if msg.ResultCode == message.ResultCodeFailed {
		length := ReadUInt16(buf)
		if length > 0 {
			bytes := make([]byte, length)
			msg.Msg = string(Read(buf, bytes))
		}
	}

	exceptionCode := ReadByte(buf)
	msg.TransactionExceptionCode = transaction.TransactionExceptionCode(exceptionCode)
	msg.BranchId = int64(ReadUInt64(buf))

	return msg
}

func (c *BranchRegisterResponseCodec) Encode(in interface{}) []byte {
	buf := goetty.NewByteBuf(0)
	resp, _ := in.(message.BranchRegisterResponse)

	resultCode := ReadByte(buf)
	if resultCode == byte(message.ResultCodeFailed) {
		var msg string
		if len(resp.Msg) > 128 {
			msg = resp.Msg[:128]
		} else {
			msg = resp.Msg
		}
		Write16String(msg, buf)
	}

	buf.WriteByte(byte(resp.TransactionExceptionCode))
	branchID := uint64(resp.BranchId)
	branchIdBytes := []byte{
		byte(branchID >> 56),
		byte(branchID >> 48),
		byte(branchID >> 40),
		byte(branchID >> 32),
		byte(branchID >> 24),
		byte(branchID >> 16),
		byte(branchID >> 8),
		byte(branchID),
	}
	buf.Write(branchIdBytes)
	return buf.RawBuf()
}

func (g *BranchRegisterResponseCodec) GetMessageType() message.MessageType {
	return message.MessageType_BranchRegisterResult
}

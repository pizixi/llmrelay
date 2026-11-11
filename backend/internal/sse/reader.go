package sse

import (
	"errors"
	"io"
)

// 转换器需要完整事件；超过上限必须报错，不能像旁路观察器那样静默放弃解析。
// 上限高于观察器，允许工具参数、加密推理等较大的合法事件。
const MaxReadEventBytes = 64 << 20

var ErrEventTooLarge = errors.New("upstream SSE event exceeds 64 MiB")

// Reader 复用 Parser 的事件边界规则，向协议转换器提供完整的多行 data。
// EOF 或传输截断时先交付未以空行结尾的最后一个事件，再保留原始读取错误，
// 由调用方根据协议结束信号判断响应是否完整。
type Reader struct {
	source io.Reader
	parser *Parser
	buffer []byte
	events []Event
	err    error
}

func NewReader(source io.Reader) *Reader {
	return &Reader{source: source, parser: NewParser(MaxReadEventBytes), buffer: make([]byte, 32<<10)}
}

func (r *Reader) Next() (Event, error) {
	for {
		if len(r.events) > 0 {
			event := r.events[0]
			r.events[0] = Event{}
			r.events = r.events[1:]
			return event, nil
		}
		if r.err != nil {
			return Event{}, r.err
		}
		n, err := r.source.Read(r.buffer)
		r.events = r.parser.Feed(r.buffer[:n])
		if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
			r.events = append(r.events, r.parser.Flush()...)
		}
		if r.parser.Dropped() {
			r.err = ErrEventTooLarge
		} else {
			r.err = err
		}
	}
}

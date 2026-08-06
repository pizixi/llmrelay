package sse

import (
	"bytes"
	"strings"
)

// Event 是一个已经按 SSE 空行边界组装完成的事件。
// Data 按 SSE 规范把同一事件的多个 data 行用换行拼接。
type Event struct {
	Name string
	Data string
}

// DefaultMaxEventBytes 限制旁路观察器缓存的单个 SSE 事件大小。
// 原始流转发不受此限制；超过限制时仅停止观察，避免观测逻辑影响透传。
const DefaultMaxEventBytes = 1 << 20

// Parser 是可增量 Feed 的 SSE parser。它支持 LF、CRLF、CR 换行、注释、
// 多 data 行和跨网络 chunk 的事件。解析器只用于旁路观察，不应承担原始流转发。
type Parser struct {
	pending  []byte
	name     string
	data     []string
	maxBytes int
	dropped  bool
}

func NewParser(maxBytes int) *Parser {
	if maxBytes <= 0 {
		maxBytes = DefaultMaxEventBytes
	}
	return &Parser{maxBytes: maxBytes}
}

// Dropped 表示某个事件超过了观察缓存上限。此状态只影响观察，不影响调用方继续转发原始字节。
func (p *Parser) Dropped() bool { return p == nil || p.dropped }

// Feed 处理一段网络数据并返回其中已经结束的 SSE 事件。
func (p *Parser) Feed(chunk []byte) []Event {
	if p == nil || p.dropped || len(chunk) == 0 {
		return nil
	}
	p.pending = append(p.pending, chunk...)
	return p.drain(false)
}

// Flush 在 EOF 时处理最后一个没有空行结尾的事件。
func (p *Parser) Flush() []Event {
	if p == nil || p.dropped {
		return nil
	}
	return p.drain(true)
}

func (p *Parser) drain(flush bool) []Event {
	var events []Event
	for {
		line, ok := p.nextLine(flush)
		if !ok {
			// A peer can keep sending a single unterminated line forever. Check
			// the pending bytes even when no complete line is available so the
			// observer cannot become an unbounded memory sink. Raw forwarding is
			// deliberately handled by the caller and is unaffected by this drop.
			if p.bufferedSize() > p.maxBytes {
				p.dropped = true
				p.pending = nil
				p.name = ""
				p.data = nil
			}
			break
		}
		if len(line) == 0 {
			if event, ok := p.dispatch(); ok {
				events = append(events, event)
			}
			continue
		}
		if line[0] == ':' {
			continue
		}
		field, value := splitField(line)
		switch field {
		case "event":
			p.name = value
		case "data":
			p.data = append(p.data, value)
		case "id", "retry":
			// 旁路观察不需要连接状态或重试时间。
		}
		if p.bufferedSize() > p.maxBytes {
			p.dropped = true
			p.pending = nil
			p.name = ""
			p.data = nil
			return events
		}
	}
	if flush && len(p.pending) == 0 {
		if event, ok := p.dispatch(); ok {
			events = append(events, event)
		}
	}
	return events
}

// nextLine 返回一行以及是否成功消费。CR 在 chunk 尾部时等待下一个 chunk，
// 避免把后续的 LF 误判成空行。
func (p *Parser) nextLine(flush bool) ([]byte, bool) {
	lf := bytes.IndexByte(p.pending, '\n')
	cr := bytes.IndexByte(p.pending, '\r')
	idx := -1
	if lf >= 0 && (cr < 0 || lf < cr) {
		idx = lf
	} else if cr >= 0 {
		if cr == len(p.pending)-1 && !flush {
			return nil, false
		}
		idx = cr
	}
	if idx < 0 {
		if flush && len(p.pending) > 0 {
			line := append([]byte(nil), p.pending...)
			p.pending = nil
			return line, true
		}
		return nil, false
	}
	line := append([]byte(nil), p.pending[:idx]...)
	consume := idx + 1
	if p.pending[idx] == '\r' && consume < len(p.pending) && p.pending[consume] == '\n' {
		consume++
	}
	p.pending = p.pending[consume:]
	return line, true
}

func splitField(line []byte) (string, string) {
	idx := bytes.IndexByte(line, ':')
	if idx < 0 {
		return string(line), ""
	}
	value := line[idx+1:]
	if len(value) > 0 && value[0] == ' ' {
		value = value[1:]
	}
	return string(line[:idx]), string(value)
}

func (p *Parser) bufferedSize() int {
	total := len(p.pending) + len(p.name)
	for _, value := range p.data {
		total += len(value)
	}
	return total
}

func (p *Parser) dispatch() (Event, bool) {
	if len(p.data) == 0 && p.name == "" {
		return Event{}, false
	}
	event := Event{Name: p.name, Data: strings.Join(p.data, "\n")}
	p.name = ""
	p.data = nil
	return event, true
}

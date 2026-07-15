package ledger

import (
	"bytes"

	"github.com/charlesnpx/paperclip/internal/domain"
)

const Header = "# PAPERCLIP.md\n\nThis file is managed by paperclip. Events are append-only fenced JSON blocks.\n\n"

func Parse(body []byte) (domain.Snapshot, error) {
	if len(body) == 0 {
		return domain.EmptySnapshot(), nil
	}
	if !bytes.HasPrefix(body, []byte(Header)) {
		return domain.Snapshot{}, MalformedError{Offset: 0, Message: "missing fixed V1 header"}
	}
	var events []domain.Event
	offset := len(Header)
	block := 0
	for offset < len(body) {
		for offset < len(body) && isSpace(body[offset]) {
			offset++
		}
		if offset >= len(body) {
			break
		}
		if !bytes.HasPrefix(body[offset:], []byte("```json\n")) {
			return domain.Snapshot{}, MalformedError{Block: block + 1, Offset: offset, Message: "expected json fence"}
		}
		block++
		contentStart := offset + len("```json\n")
		closeRel := bytes.Index(body[contentStart:], []byte("\n```"))
		if closeRel < 0 {
			return domain.Snapshot{}, MalformedError{Block: block, Offset: contentStart, Message: "missing closing fence"}
		}
		contentEnd := contentStart + closeRel
		content := body[contentStart:contentEnd]
		if len(bytes.TrimSpace(content)) == 0 {
			return domain.Snapshot{}, MalformedError{Block: block, Offset: contentStart, Message: "empty event block"}
		}
		if bytes.ContainsAny(content, "\r\n") {
			return domain.Snapshot{}, MalformedError{Block: block, Offset: contentStart, Message: "event JSON must be compact"}
		}
		event, err := domain.DecodeEvent(content)
		if err != nil {
			return domain.Snapshot{}, MalformedError{Block: block, Offset: contentStart, Message: "invalid event"}
		}
		events = append(events, event)
		offset = contentEnd + len("\n```")
	}
	snapshot, err := domain.ApplyEvents(events)
	if err != nil {
		return domain.Snapshot{}, MalformedError{Block: block, Offset: 0, Message: "invalid event sequence"}
	}
	return snapshot, nil
}

func AppendBlocks(existing []byte, events []domain.Event) ([]byte, error) {
	out := append([]byte(nil), existing...)
	if len(out) == 0 {
		out = append(out, []byte(Header)...)
	} else if out[len(out)-1] != '\n' {
		out = append(out, '\n')
	}
	for _, event := range events {
		body, err := domain.MarshalEvent(event)
		if err != nil {
			return nil, err
		}
		out = append(out, []byte("```json\n")...)
		out = append(out, body...)
		out = append(out, []byte("\n```\n\n")...)
	}
	return out, nil
}

func isSpace(b byte) bool {
	return b == ' ' || b == '\t' || b == '\n' || b == '\r'
}

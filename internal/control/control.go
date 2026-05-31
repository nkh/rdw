// Package control implements the inline control sequence parser.
//
// A control sequence is a line whose first two bytes are a single lowercase
// letter followed by a colon. Such lines are intercepted by the pipeline and
// acted on instead of being forwarded to the renderer.
//
// The verbatim prefix "v:" is special: it strips the prefix and passes the
// remainder through as literal content, allowing callers to send data that
// would otherwise be treated as a control sequence.
package control

import (
	"fmt"
	"strings"
)

// Kind identifies the type of control sequence.
type Kind string

const (
	KindVerbatim  Kind = "v"
	KindQuit      Kind = "q"
	KindSemaphore Kind = "s"
	KindClear     Kind = "c"
	KindTimestamp Kind = "t"
	KindFormatter Kind = "f"
	KindRelay     Kind = "r"
	KindKV        Kind = "="
)

var knownKinds = map[string]Kind{
	"v": KindVerbatim,
	"q": KindQuit,
	"s": KindSemaphore,
	"c": KindClear,
	"t": KindTimestamp,
	"f": KindFormatter,
	"r": KindRelay,
	"=": KindKV,
}

// Sequence is a parsed control sequence.
type Sequence struct {
	Kind    Kind
	Payload string // the part after "X:"
}

// Parse attempts to parse line as a control sequence.
// Returns (seq, true) on success, or (zero, false) if the line is not a
// control sequence.
func Parse(line string) (Sequence, bool) {
	if len(line) < 2 {
		return Sequence{}, false
	}

	if line[1] != ':' {
		return Sequence{}, false
	}

	prefix := string(line[0])
	k, ok := knownKinds[prefix]

	if !ok {
		return Sequence{}, false
	}

	return Sequence{Kind: k, Payload: line[2:]}, true
}

// IsVerbatim reports whether the sequence should pass its payload through
// to the renderer as literal content.
func (s Sequence) IsVerbatim() bool {
	return s.Kind == KindVerbatim
}

// KVPairs parses the payload of a KindKV sequence into key=value pairs.
// The format is "key=value;key2=value2". An error is returned if any pair
// is malformed.
func (s Sequence) KVPairs() ([][2]string, error) {
	if s.Kind != KindKV {
		return nil, fmt.Errorf("KVPairs called on non-KV sequence (kind=%q)", s.Kind)
	}

	if s.Payload == "" {
		return nil, nil
	}

	raw := strings.Split(s.Payload, ";")
	pairs := make([][2]string, 0, len(raw))

	for _, entry := range raw {
		entry = strings.TrimSpace(entry)

		if entry == "" {
			continue
		}

		idx := strings.IndexByte(entry, '=')

		if idx <= 0 {
			return nil, fmt.Errorf("malformed KV pair %q: missing '='", entry)
		}

		key := entry[:idx]
		val := entry[idx+1:]
		pairs = append(pairs, [2]string{key, val})
	}

	return pairs, nil
}

package tree

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
)

// MarshalJSON returns the JSON encoding of the specified node.
func MarshalJSON(n Node) ([]byte, error) {
	return json.Marshal(n)
}

// DecodeJSON decodes the next JSON value from dec; io.EOF means the input ended before a top-level value, io.ErrUnexpectedEOF inside one.
// Inside a container the caller opened with dec.Token, a cut between its elements is also io.EOF, so read the closing delimiter to detect it; tree caps nesting at 10,000 levels from the value, json/v2 from the stream root.
func DecodeJSON(dec *json.Decoder) (Node, error) {
	t, err := dec.Token()
	if err != nil {
		return nil, err
	}
	switch t.(type) {
	case nil, string, float64, bool, json.Delim:
		return jsonElem(dec, t, 1)
	}
	return nil, fmt.Errorf("unknown token %s", tokenText(t))
}

// jsonEnd rejects anything after the top-level value, as json.Unmarshal does.
func jsonEnd(dec *json.Decoder) error {
	t, err := dec.Token()
	if err == io.EOF {
		return nil
	}
	if err != nil {
		return err
	}
	return fmt.Errorf("unexpected %s after top-level value", tokenText(t))
}

// tokenText prints t as it reads in JSON, for error messages.
func tokenText(t json.Token) string {
	switch tt := t.(type) {
	case nil:
		return "null"
	case string:
		return strconv.Quote(tt)
	}
	return fmt.Sprint(t)
}

// unexpectedEOF turns io.EOF inside a container into io.ErrUnexpectedEOF, as encoding/json does.
func unexpectedEOF(err error) error {
	if err == io.EOF {
		return io.ErrUnexpectedEOF
	}
	return err
}

// UnmarshalJSON parses the JSON-encoded data to a Node, rejecting trailing data as json.Unmarshal does.
func UnmarshalJSON(data []byte) (Node, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	n, err := DecodeJSON(dec)
	if err != nil {
		return nil, err
	}
	if err := jsonEnd(dec); err != nil {
		return nil, err
	}
	return n, nil
}

// UnmarshalJSON implements json.Unmarshaler, leaving n as it is on error; null gives Nil.
func (n *Any) UnmarshalJSON(data []byte) error {
	v, err := UnmarshalJSON(data)
	if err != nil {
		return err
	}
	n.Node = v
	return nil
}

// UnmarshalJSON implements json.Unmarshaler: it sets each key of the object in n, keeping n's other keys, and leaves n as it is on null.
// Like encoding/json, it decodes in place, so on error n may hold some of the new keys.
func (n *Map) UnmarshalJSON(data []byte) error {
	return unmarshalContainer(data, '{', func(dec *json.Decoder) (func(), error) {
		m := *n
		if m == nil {
			m = make(Map)
		}
		if err := jsonMap(dec, m, 1); err != nil {
			return nil, err
		}
		return func() { *n = m }, nil
	})
}

// UnmarshalJSON implements json.Unmarshaler: it resets n to length zero and refills it, leaving n as it is on null.
// It reuses n's backing array, which other nodes may share, and on error n keeps its length, holding what was decoded into its backing array.
func (n *Array) UnmarshalJSON(data []byte) error {
	return unmarshalContainer(data, '[', func(dec *json.Decoder) (func(), error) {
		base := (*n)[:0]
		if base == nil {
			base = Array{} // [] gives an empty Array, not a nil one that marshals as null
		}
		a, err := jsonArray(dec, base, 1)
		if err != nil {
			return nil, err
		}
		return func() { *n = a }, nil
	})
}

// unmarshalContainer reads data as one container opened by open, treating null as a no-op and running decode's assign only after the trailing-data check.
func unmarshalContainer(data []byte, open json.Delim, decode func(dec *json.Decoder) (assign func(), err error)) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	t, err := dec.Token()
	if err != nil {
		return err
	}
	if t == nil {
		return jsonEnd(dec)
	}
	if d, ok := t.(json.Delim); !ok || d != open {
		return fmt.Errorf("unknown token %s", tokenText(t))
	}
	assign, err := decode(dec)
	if err != nil {
		return err
	}
	if err := jsonEnd(dec); err != nil {
		return err
	}
	assign()
	return nil
}

// MarshalJSON is an implementation of json.Marshaler.
func (n NilValue) MarshalJSON() ([]byte, error) {
	return []byte("null"), nil
}

// jsonMap decodes the members of a JSON object into m, after its opening brace has been read.
func jsonMap(dec *json.Decoder, m Map, depth int) error {
	for {
		t, err := dec.Token()
		if err != nil {
			return unexpectedEOF(err)
		}
		if d, ok := t.(json.Delim); ok {
			if d == '}' {
				return nil
			}
			return fmt.Errorf("unknown token %s", tokenText(t))
		}
		key, ok := t.(string)
		if !ok {
			return fmt.Errorf("unknown token %s", tokenText(t))
		}

		t, err = dec.Token()
		if err != nil {
			return unexpectedEOF(err)
		}
		v, err := jsonElem(dec, t, depth+1)
		if err != nil {
			return err
		}
		m[key] = v
	}
}

// jsonArray appends the elements of a JSON array to a, after its opening bracket has been read.
func jsonArray(dec *json.Decoder, a Array, depth int) (Array, error) {
	for {
		t, err := dec.Token()
		if err != nil {
			return nil, unexpectedEOF(err)
		}
		if d, ok := t.(json.Delim); ok && d == ']' {
			return a, nil
		}
		v, err := jsonElem(dec, t, depth+1)
		if err != nil {
			return nil, err
		}
		a = append(a, v)
	}
}

// maxJSONDepth bounds the recursion per decoded value; json.Decoder.Token enforces no cap before json/v2.
const maxJSONDepth = 10000

// errMaxDepth matches json/v2's depth error text, which Go 1.27 returns from Token first.
var errMaxDepth = errors.New("exceeded max depth")

// jsonElem decodes the value that starts with t at nesting depth, descending into an object or array.
func jsonElem(dec *json.Decoder, t json.Token, depth int) (Node, error) {
	d, ok := t.(json.Delim)
	if !ok {
		return jsonValue(t), nil
	}
	if d != '{' && d != '[' {
		return nil, fmt.Errorf("unknown token %s", tokenText(t))
	}
	if depth > maxJSONDepth {
		return nil, errMaxDepth
	}
	if d == '{' {
		m := Map{}
		if err := jsonMap(dec, m, depth); err != nil {
			return nil, err
		}
		return m, nil
	}
	a, err := jsonArray(dec, Array{}, depth)
	if err != nil {
		return nil, err
	}
	return a, nil
}

// jsonValue converts a JSON token to a Node value.
// Handles primitive types: string, bool, float64, and nil.
func jsonValue(t json.Token) Node {
	if t == nil {
		return Nil
	}
	switch tt := t.(type) {
	case string:
		return StringValue(tt)
	case bool:
		return BoolValue(tt)
	case float64:
		return NumberValue(tt)
	}
	return StringValue(fmt.Sprintf("%#v", t))
}

// MarshalViaJSON returns the node encoding of v via "encoding/json".
func MarshalViaJSON(v any) (Node, error) {
	if v == nil {
		return Nil, nil
	}
	if n, ok := v.(Node); ok {
		return n, nil
	}
	data, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	return UnmarshalJSON(data)
}

// UnmarshalViaJSON stores the node in the value pointed to by v via "encoding/json".
func UnmarshalViaJSON(n Node, v any) error {
	data, err := MarshalJSON(n)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, v)
}

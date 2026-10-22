package tree

import (
	"bytes"
	"fmt"
	"maps"

	"go.yaml.in/yaml/v3"
)

// yamlIndent is the block sequence/mapping indent used by tree's
// YAML encoder. yaml.v3 defaults to 4 spaces; we pick 2 to stay closer
// to yaml.v2's output and to common convention.
const yamlIndent = 2

// MarshalYAML returns the YAML encoding of the specified node using
// tree's default 2-space indentation.
func MarshalYAML(n Node) ([]byte, error) {
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(yamlIndent)
	if err := enc.Encode(n); err != nil {
		_ = enc.Close()
		return nil, err
	}
	if err := enc.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// DecodeYAML decodes YAML as a node using the provided decoder.
func DecodeYAML(dec *yaml.Decoder) (Node, error) {
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, err
	}
	return ToNode(v), nil
}

// UnmarshalYAML returns the YAML encoding of the specified node.
func UnmarshalYAML(data []byte) (Node, error) {
	var v any
	if err := yaml.Unmarshal(data, &v); err != nil {
		return nil, err
	}
	return ToNode(v), nil
}

// UnmarshalYAML is an implementation of yaml.Unmarshaler (yaml.v3).
func (n *Map) UnmarshalYAML(value *yaml.Node) error {
	var v any
	if err := value.Decode(&v); err != nil {
		return err
	}
	tn := ToNode(v)
	m, ok := tn.(Map)
	if !ok {
		return &yaml.TypeError{Errors: []string{fmt.Sprintf("line %d: cannot unmarshal %s into tree.Map", value.Line, value.Tag)}} // lets yaml.v3 go on decoding
	}
	if *n == nil {
		*n = m
		return nil
	}
	maps.Copy(*n, m)
	return nil
}

// UnmarshalYAML is an implementation of yaml.Unmarshaler (yaml.v3).
func (n *Array) UnmarshalYAML(value *yaml.Node) error {
	var v any
	if err := value.Decode(&v); err != nil {
		return err
	}
	tn := ToNode(v)
	a, ok := tn.(Array)
	if !ok {
		return &yaml.TypeError{Errors: []string{fmt.Sprintf("line %d: cannot unmarshal %s into tree.Array", value.Line, value.Tag)}} // lets yaml.v3 go on decoding
	}
	*n = a // a reused Array is replaced here; UnmarshalJSON refills it in place
	return nil
}

// MarshalYAML is an implementation of yaml.Marshaler.
func (n NilValue) MarshalYAML() (any, error) {
	return nil, nil
}

// MarshalViaYAML returns the node encoding of v via "go.yaml.in/yaml/v3".
func MarshalViaYAML(v any) (Node, error) {
	if v == nil {
		return Nil, nil
	}
	if n, ok := v.(Node); ok {
		return n, nil
	}
	data, err := yaml.Marshal(v)
	if err != nil {
		return nil, err
	}
	return UnmarshalYAML(data)
}

// UnmarshalViaYAML stores the node in the value pointed to by v via "go.yaml.in/yaml/v3".
func UnmarshalViaYAML(n Node, v any) error {
	data, err := MarshalYAML(n)
	if err != nil {
		return err
	}
	return yaml.Unmarshal(data, v)
}

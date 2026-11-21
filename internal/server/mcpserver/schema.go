package mcpserver

import (
	_ "embed"
	"fmt"
	"strings"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
)

// ledgerDescriptor is proto/float/v1 as a FileDescriptorSet with source info,
// written by `mise run proto-gen` (buf build). protoc-gen-go strips comments
// from the generated descriptors, so this is where tool input schemas get
// their field descriptions. `mise run proto-check` fails if it is stale.
//
//go:embed ledger.binpb
var ledgerDescriptor []byte

// docs maps proto element full names (messages, fields) to their source
// comments, collapsed to one line.
type docs map[protoreflect.FullName]string

func loadDocs(b []byte) (docs, error) {
	var fds descriptorpb.FileDescriptorSet
	if err := proto.Unmarshal(b, &fds); err != nil {
		return nil, fmt.Errorf("unmarshal descriptor set: %w", err)
	}
	files, err := protodesc.NewFiles(&fds)
	if err != nil {
		return nil, fmt.Errorf("build descriptor files: %w", err)
	}
	d := docs{}
	files.RangeFiles(func(fd protoreflect.FileDescriptor) bool {
		locs := fd.SourceLocations()
		add := func(desc protoreflect.Descriptor) {
			loc := locs.ByDescriptor(desc)
			if text := joinComments(loc.LeadingComments, loc.TrailingComments); text != "" {
				d[desc.FullName()] = text
			}
		}
		var walk func(protoreflect.MessageDescriptors)
		walk = func(msgs protoreflect.MessageDescriptors) {
			for i := range msgs.Len() {
				md := msgs.Get(i)
				add(md)
				for j := range md.Fields().Len() {
					add(md.Fields().Get(j))
				}
				walk(md.Messages())
			}
		}
		walk(fd.Messages())
		return true
	})
	return d, nil
}

func joinComments(parts ...string) string {
	var words []string
	for _, p := range parts {
		words = append(words, strings.Fields(p)...)
	}
	return strings.Join(words, " ")
}

// inputSchema returns the JSON Schema for md as protojson (with proto field
// names) reads it, annotated with the proto comments in d.
func (d docs) inputSchema(md protoreflect.MessageDescriptor) map[string]any {
	return d.messageSchema(md, map[protoreflect.FullName]bool{})
}

func (d docs) messageSchema(md protoreflect.MessageDescriptor, stack map[protoreflect.FullName]bool) map[string]any {
	if stack[md.FullName()] {
		return map[string]any{"type": "object", "description": "(recursive " + string(md.Name()) + ")"}
	}
	stack[md.FullName()] = true
	defer delete(stack, md.FullName())

	props := map[string]any{}
	fields := md.Fields()
	for i := range fields.Len() {
		fd := fields.Get(i)
		s := d.fieldSchema(fd, stack)
		desc := d[fd.FullName()]
		if oo := fd.ContainingOneof(); oo != nil && !oo.IsSynthetic() {
			var names []string
			for j := range oo.Fields().Len() {
				names = append(names, string(oo.Fields().Get(j).Name()))
			}
			desc = strings.TrimSpace(fmt.Sprintf("Set at most one of: %s. %s", strings.Join(names, ", "), desc))
		}
		if desc != "" {
			s["description"] = desc
		}
		props[string(fd.Name())] = s
	}
	s := map[string]any{
		"type":                 "object",
		"properties":           props,
		"additionalProperties": false,
	}
	if desc := d[md.FullName()]; desc != "" {
		s["description"] = desc
	}
	return s
}

func (d docs) fieldSchema(fd protoreflect.FieldDescriptor, stack map[protoreflect.FullName]bool) map[string]any {
	switch {
	case fd.IsMap():
		return map[string]any{
			"type":                 "object",
			"additionalProperties": d.singularSchema(fd.MapValue(), stack),
		}
	case fd.IsList():
		return map[string]any{
			"type":  "array",
			"items": d.singularSchema(fd, stack),
		}
	default:
		return d.singularSchema(fd, stack)
	}
}

func (d docs) singularSchema(fd protoreflect.FieldDescriptor, stack map[protoreflect.FullName]bool) map[string]any {
	switch fd.Kind() {
	case protoreflect.BoolKind:
		return map[string]any{"type": "boolean"}
	case protoreflect.Int32Kind, protoreflect.Sint32Kind, protoreflect.Sfixed32Kind,
		protoreflect.Uint32Kind, protoreflect.Fixed32Kind,
		protoreflect.Int64Kind, protoreflect.Sint64Kind, protoreflect.Sfixed64Kind,
		protoreflect.Uint64Kind, protoreflect.Fixed64Kind:
		return map[string]any{"type": "integer"}
	case protoreflect.FloatKind, protoreflect.DoubleKind:
		return map[string]any{"type": "number"}
	case protoreflect.StringKind:
		return map[string]any{"type": "string"}
	case protoreflect.BytesKind:
		return map[string]any{"type": "string", "contentEncoding": "base64"}
	case protoreflect.EnumKind:
		values := fd.Enum().Values()
		names := make([]string, values.Len())
		for i := range values.Len() {
			names[i] = string(values.Get(i).Name())
		}
		return map[string]any{"type": "string", "enum": names}
	case protoreflect.MessageKind, protoreflect.GroupKind:
		return d.messageSchema(fd.Message(), stack)
	default:
		return map[string]any{}
	}
}

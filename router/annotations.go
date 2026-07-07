// annotations.go reads the versionintercept microversion annotations off
// protobuf descriptors so the router and interceptor can enforce method
// lifecycles and gate fields based on the negotiated microversion.
package router

import (
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"

	annotationsv1 "go.f0o.dev/version-intercept/gen/versionintercept/annotations/v1"
)

// MethodLifecycle describes the microversion lifecycle of an RPC method as
// declared by its annotations. Unset bounds are reported via their ok flags.
type MethodLifecycle struct {
	Introduced    Version
	HasIntroduced bool

	Deprecated    Version
	HasDeprecated bool

	Removed    Version
	HasRemoved bool
}

// Available reports whether the method is callable at the negotiated version.
// A method is available when v >= Introduced (if set) and v < Removed (if set).
//
// Arguments:
//   - v: The negotiated microversion.
//
// Returns true if the method may be invoked at v.
func (l MethodLifecycle) Available(v Version) bool {
	if l.HasIntroduced && v.LessThan(l.Introduced) {
		return false
	}
	if l.HasRemoved && v.AtLeast(l.Removed) {
		return false
	}
	return true
}

// IsDeprecated reports whether the method is deprecated at the negotiated
// version (v >= Deprecated).
//
// Arguments:
//   - v: The negotiated microversion.
//
// Returns true if the method is deprecated at v.
func (l MethodLifecycle) IsDeprecated(v Version) bool {
	return l.HasDeprecated && v.AtLeast(l.Deprecated)
}

// MethodLifecycleOf extracts the microversion lifecycle from a method
// descriptor's annotations. Methods without the annotation yield a zero
// MethodLifecycle (always available).
//
// Arguments:
//   - md: The method descriptor to inspect.
//
// Returns the parsed MethodLifecycle.
func MethodLifecycleOf(md protoreflect.MethodDescriptor) MethodLifecycle {
	var out MethodLifecycle
	if md == nil {
		return out
	}
	ext := proto.GetExtension(md.Options(), annotationsv1.E_MethodVersioning)

	mv, ok := ext.(*annotationsv1.MethodVersioning)
	if !ok || mv == nil {
		return out
	}
	if v, err := ParseVersion(mv.GetIntroducedIn()); err == nil {
		out.Introduced, out.HasIntroduced = v, true
	}
	if v, err := ParseVersion(mv.GetDeprecatedIn()); err == nil {
		out.Deprecated, out.HasDeprecated = v, true
	}
	if v, err := ParseVersion(mv.GetRemovedIn()); err == nil {
		out.Removed, out.HasRemoved = v, true
	}
	return out
}

// fieldVisibleAt reports whether a field is visible (honored/populated) at the
// negotiated version according to its annotations. Fields without the
// annotation are always visible.
//
// Arguments:
//   - fd: The field descriptor to inspect.
//   - v: The negotiated microversion.
//
// Returns true if the field should be retained at v.
func fieldVisibleAt(fd protoreflect.FieldDescriptor, v Version) bool {
	ext := proto.GetExtension(fd.Options(), annotationsv1.E_FieldVersioning)
	fv, ok := ext.(*annotationsv1.FieldVersioning)
	if !ok || fv == nil {
		return true
	}
	if intro, err := ParseVersion(fv.GetIntroducedIn()); err == nil && v.LessThan(intro) {
		return false
	}
	if removed, err := ParseVersion(fv.GetRemovedIn()); err == nil && v.AtLeast(removed) {
		return false
	}
	return true
}

// gateFields clears every field of msg (recursively) that is not visible at the
// negotiated version. It mutates msg in place.
//
// Arguments:
//   - msg: The message to gate.
//   - v: The negotiated microversion.
func gateFields(msg protoreflect.Message, v Version) {
	if msg == nil || !msg.IsValid() {
		return
	}
	fields := msg.Descriptor().Fields()
	for i := 0; i < fields.Len(); i++ {
		fd := fields.Get(i)
		if !fieldVisibleAt(fd, v) {
			msg.Clear(fd)
			continue
		}
		if fd.Kind() == protoreflect.MessageKind && !fd.IsMap() && !fd.IsList() && msg.Has(fd) {
			gateFields(msg.Get(fd).Message(), v)
		}
	}
}

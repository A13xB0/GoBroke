package types

// Key is a typed name for a message tag. Declare keys once and use Get and
// Set instead of indexing Message.Tags and asserting the type, which panics
// when a tag is missing or has another type.
//
//	var UserID = types.NewKey[string]("userid")
//	UserID.Set(&msg, "42")
//	id, ok := UserID.Get(msg) // "42", true
type Key[T any] struct {
	name string
}

// NewKey returns a Key for the tag called name.
func NewKey[T any](name string) Key[T] {
	return Key[T]{name: name}
}

// Name returns the tag name, the key used in Message.Tags.
func (k Key[T]) Name() string {
	return k.name
}

// Get returns the tag's value. ok is false if the tag is missing or holds a
// value of another type.
func (k Key[T]) Get(m Message) (value T, ok bool) {
	value, ok = m.Tags[k.name].(T)
	return value, ok
}

// Set stores the tag's value, allocating Tags if needed.
func (k Key[T]) Set(m *Message, value T) {
	m.AddTag(k.name, value)
}

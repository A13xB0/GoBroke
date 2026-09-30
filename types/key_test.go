package types

import "testing"

func TestKey(t *testing.T) {
	userID := NewKey[string]("userid")
	isGM := NewKey[bool]("gm")
	if userID.Name() != "userid" {
		t.Fatalf("Name() = %q", userID.Name())
	}

	var m Message
	if _, ok := userID.Get(m); ok {
		t.Fatal("missing tag should report ok=false")
	}
	userID.Set(&m, "42")
	isGM.Set(&m, true)
	if v, ok := userID.Get(m); !ok || v != "42" {
		t.Fatalf("Get = %q, %v", v, ok)
	}
	if v, ok := isGM.Get(m); !ok || !v {
		t.Fatalf("Get = %v, %v", v, ok)
	}

	// A tag stored with another type is reported, not panicked on.
	m.Tags["userid"] = 42
	if _, ok := userID.Get(m); ok {
		t.Fatal("wrong type should report ok=false")
	}
}

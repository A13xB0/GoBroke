package types

import (
	"errors"
	"testing"

	brokeerrors "github.com/A13xB0/GoBroke/errors"
)

// Regression for review finding GB-4: AddTag on a message without a Tags map panicked.
func TestTagsOnZeroMessage(t *testing.T) {
	var m Message
	if _, err := m.GetTag("missing", nil); !errors.Is(err, brokeerrors.ErrorTagDoesNotExist) {
		t.Fatalf("want ErrorTagDoesNotExist, got %v", err)
	}
	m.AddTag("user", "alex")
	v, err := m.GetTag("user", nil)
	if err != nil || v != "alex" {
		t.Fatalf("GetTag = %v, %v", v, err)
	}
}

func TestAcceptReject(t *testing.T) {
	var m Message
	if m.State != ACCEPTED {
		t.Fatal("zero message should be ACCEPTED")
	}
	m.Reject()
	if m.State != REJECTED {
		t.Fatal("Reject did not reject")
	}
	m.Accept()
	if m.State != ACCEPTED {
		t.Fatal("Accept did not accept")
	}
}

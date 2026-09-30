package GoBroke_test

import (
	"context"
	"errors"
	"testing"

	"github.com/A13xB0/GoBroke"
	"github.com/A13xB0/GoBroke/clients"
	"github.com/A13xB0/GoBroke/endpoint"
	brokeerrors "github.com/A13xB0/GoBroke/errors"
)

func newTestBroker(t *testing.T) *GoBroke.Broke {
	t.Helper()
	b, err := GoBroke.New(endpoint.NewStubEndpoint(), GoBroke.WithContext(t.Context()))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestNewRequiresEndpoint(t *testing.T) {
	_, err := GoBroke.New(nil, GoBroke.WithContext(context.Background()))
	if !errors.Is(err, brokeerrors.ErrorNoEndpointProvided) {
		t.Fatalf("want ErrorNoEndpointProvided, got %v", err)
	}
}

func TestClientRegistry(t *testing.T) {
	b := newTestBroker(t)
	c := clients.New(clients.WithUUID("c1"))

	if err := b.RegisterClient(c); err != nil {
		t.Fatal(err)
	}
	if err := b.RegisterClient(c); !errors.Is(err, brokeerrors.ErrorClientAlreadyExists) {
		t.Fatalf("second register: want ErrorClientAlreadyExists, got %v", err)
	}
	if c.GetLastMessage().IsZero() {
		t.Error("RegisterClient should stamp the last-message time")
	}

	// localOnly is accepted and ignored since Redis support was removed.
	for _, localOnly := range [][]bool{nil, {true}, {false}} {
		got, err := b.GetClient("c1", localOnly...)
		if err != nil || got != c {
			t.Fatalf("GetClient(localOnly=%v) = %v, %v", localOnly, got, err)
		}
		if all := b.GetAllClients(localOnly...); len(all) != 1 || all[0] != c {
			t.Fatalf("GetAllClients(localOnly=%v) = %v", localOnly, all)
		}
	}

	if err := b.RemoveClient(c); err != nil {
		t.Fatal(err)
	}
	if _, err := b.GetClient("c1"); !errors.Is(err, brokeerrors.ErrorClientDoesNotExist) {
		t.Fatalf("after remove: want ErrorClientDoesNotExist, got %v", err)
	}
	if err := b.RemoveClient(c); !errors.Is(err, brokeerrors.ErrorClientDoesNotExist) {
		t.Fatalf("second remove: want ErrorClientDoesNotExist, got %v", err)
	}
	if all := b.GetAllClients(); len(all) != 0 {
		t.Fatalf("GetAllClients after remove = %v", all)
	}
}

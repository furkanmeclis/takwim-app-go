package usecase_test

import (
	"context"
	"errors"
	"testing"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/integrations/jobs/usecase"
	"github.com/google/uuid"
)

func TestHandleInboundOnce(t *testing.T) {
	e := newEnv(t, &fakeOp{})
	key := "evt-" + uuid.NewString()
	in := usecase.InboundInput{
		OrganizationID: e.org, AccountID: e.acc, Provider: "log", Kind: "sms.delivery", Key: key,
		Payload: map[string]any{"message_id": "m1"},
	}
	calls := 0
	handle := func(context.Context) (string, map[string]any, error) {
		calls++
		return usecase.StatusSucceeded, map[string]any{"applied": true}, nil
	}
	first, err := e.runner.HandleInbound(e.ctx, in, handle)
	if err != nil || first.Duplicate || first.Status != usecase.StatusSucceeded {
		t.Fatalf("first delivery = %+v %v", first, err)
	}
	second, err := e.runner.HandleInbound(e.ctx, in, handle)
	if err != nil || !second.Duplicate || second.Deliveries != 2 || second.JobUUID != first.JobUUID {
		t.Fatalf("second delivery = %+v %v", second, err)
	}
	if calls != 1 {
		t.Fatalf("handler ran %d times", calls)
	}
	d := e.detail(t, first.JobUUID)
	if d.Status != usecase.StatusSucceeded || d.Direction != "inbound" {
		t.Fatalf("job = %+v", d)
	}
}

func TestHandleInboundRetriesAfterFailure(t *testing.T) {
	e := newEnv(t, &fakeOp{})
	in := usecase.InboundInput{OrganizationID: e.org, Provider: "log", Kind: "sms.delivery", Key: "evt-" + uuid.NewString()}
	boom := errors.New("not yet")
	r, err := e.runner.HandleInbound(e.ctx, in, func(context.Context) (string, map[string]any, error) {
		return "", nil, boom
	})
	if !errors.Is(err, boom) || r.Status != usecase.StatusFailed {
		t.Fatalf("failed delivery = %+v %v", r, err)
	}
	r, err = e.runner.HandleInbound(e.ctx, in, func(context.Context) (string, map[string]any, error) {
		panic("bad payload")
	})
	if err == nil || r.Status != usecase.StatusFailed {
		t.Fatalf("panicking delivery = %+v %v", r, err)
	}
	r, err = e.runner.HandleInbound(e.ctx, in, func(context.Context) (string, map[string]any, error) {
		return usecase.StatusIgnored, nil, nil
	})
	if err != nil || r.Duplicate || r.Status != usecase.StatusIgnored || r.Deliveries != 3 {
		t.Fatalf("third delivery = %+v %v", r, err)
	}
	if _, err := e.runner.HandleInbound(e.ctx, usecase.InboundInput{Provider: "log"}, nil); !errors.Is(err, usecase.ErrInboundInput) {
		t.Fatalf("invalid input err = %v", err)
	}
}

package usecase

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/auth/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/jwt"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/resourceevents"
	"github.com/google/uuid"
)

type recordedEvent struct {
	resource, action string
	uuids            []string
}

type recordingEvents struct {
	mu     sync.Mutex
	events []recordedEvent
}

func (r *recordingEvents) Publish(_ context.Context, resource, action string, uuids ...string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, recordedEvent{resource: resource, action: action, uuids: uuids})
}

func TestUserMutationsPublishResourceEvents(t *testing.T) {
	repo := newMemRepo()
	tokens, err := jwt.NewManager("test-secret-key-32-bytes-minimum!", time.Minute, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	uc := New(repo, tokens)
	uc.SetAuthSettings(testAuthGate{passwordRegister: true, passwordLogin: true, registration: true})
	rec := &recordingEvents{}
	uc.SetResourceEvents(rec)

	user, err := uc.Register(context.Background(), model.RegisterInput{
		Email: "live@example.com", Password: "Password1!", Name: "Live", Surname: "User",
	})
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	name := "Renamed"
	if _, err := uc.PatchPlatformUser(context.Background(), user.UUID, model.PatchPlatformUserInput{Name: &name}); err != nil {
		t.Fatalf("patch: %v", err)
	}

	if len(rec.events) != 2 {
		t.Fatalf("events = %+v, want 2", rec.events)
	}
	want := []string{resourceevents.ActionCreated, resourceevents.ActionUpdated}
	for i, ev := range rec.events {
		if ev.resource != ResourceUsers || ev.action != want[i] ||
			len(ev.uuids) != 1 || ev.uuids[0] != user.UUID.String() {
			t.Fatalf("event %d = %+v", i, ev)
		}
	}
}

func TestCanSubscribeChannelRunsRegisteredRules(t *testing.T) {
	tokens, _ := jwt.NewManager("test-secret-key-32-bytes-minimum!", time.Minute, time.Hour)
	uc := New(newMemRepo(), tokens)
	id := uuid.New()

	// Without the rule, resource channels fall through and are denied.
	if ok, _ := uc.CanSubscribeChannel(context.Background(), id, true, "resource:platform.users"); ok {
		t.Fatal("resource channel allowed without a registered rule")
	}

	reg := resourceevents.NewRegistry()
	reg.Register(ResourceUsers, "platform.users.read")
	uc.AddChannelRules(reg.ChannelRule())

	if ok, _ := uc.CanSubscribeChannel(context.Background(), id, true, "resource:platform.users"); !ok {
		t.Fatal("super admin denied a registered resource channel")
	}
	if ok, _ := uc.CanSubscribeChannel(context.Background(), id, false, "resource:platform.users"); ok {
		t.Fatal("principal without permission allowed")
	}
	// Built-in checks still apply to other channels.
	if ok, _ := uc.CanSubscribeChannel(context.Background(), id, false, "user:"+id.String()); !ok {
		t.Fatal("own user channel denied")
	}
}

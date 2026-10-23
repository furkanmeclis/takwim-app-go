package pushtext

import (
	"strings"
	"testing"
)

func TestForRendersSafeVarsOnly(t *testing.T) {
	got := For("exports.ready", "tr", map[string]string{"resource": "Kullanıcılar", "email": "a@b.c"})
	if got.Title != "Dışa aktarım hazır" || !strings.Contains(got.Body, "Kullanıcılar") || strings.Contains(got.Body, "a@b.c") {
		t.Fatalf("got %+v", got)
	}
	en := For("bulk.completed", "en", map[string]string{"succeeded": "12"})
	if en.Title != "Bulk action finished" || en.Body != "The bulk action on 12 records has finished." {
		t.Fatalf("en %+v", en)
	}
}

func TestForFallsBackWhenVarMissing(t *testing.T) {
	got := For("exports.ready", "tr", nil)
	if strings.Contains(got.Title+got.Body, "{{") || got.Body != "Yeni bir bildiriminiz var." {
		t.Fatalf("missing var leaked: %+v", got)
	}
	unknown := For("auth.welcome", "en", nil)
	if unknown.Title != "Notification" || unknown.Body != "You have a new notification." {
		t.Fatalf("unknown kind %+v", unknown)
	}
	if !Known("bulk.failed") || Known("nope") {
		t.Fatal("Known mismatch")
	}
}

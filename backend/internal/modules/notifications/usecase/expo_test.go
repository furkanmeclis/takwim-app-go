package usecase

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// expoTestQuerier is a Querier + pushDeviceStore backed by memory.
type expoTestQuerier struct {
	pushTestQuerier
	mu       sync.Mutex
	row      db.Notification
	user     db.User
	devices  []db.PushDevice
	disabled map[int64]string
	tickets  []db.ListDuePushTicketsRow
	deleted  []int64
}

func (m *expoTestQuerier) GetNotificationByID(context.Context, int64) (db.Notification, error) {
	return m.row, nil
}

func (m *expoTestQuerier) GetUserByID(context.Context, int64) (db.User, error) { return m.user, nil }

func (m *expoTestQuerier) UpsertPushDevice(_ context.Context, arg db.UpsertPushDeviceParams) (db.PushDevice, error) {
	d := db.PushDevice{ID: int64(len(m.devices) + 1), Uuid: uuid.New(), UserID: arg.UserID, Token: arg.Token, Platform: arg.Platform, Locale: arg.Locale}
	m.devices = append(m.devices, d)
	return d, nil
}

func (m *expoTestQuerier) DeletePushDeviceForUser(context.Context, db.DeletePushDeviceForUserParams) (int64, error) {
	return 1, nil
}

func (m *expoTestQuerier) ListActivePushDevicesByUser(context.Context, int64) ([]db.PushDevice, error) {
	return m.devices, nil
}

func (m *expoTestQuerier) HasActivePushDevices(context.Context, int64) (bool, error) {
	return len(m.devices) > 0, nil
}

func (m *expoTestQuerier) DisablePushDevice(_ context.Context, arg db.DisablePushDeviceParams) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.disabled == nil {
		m.disabled = map[int64]string{}
	}
	m.disabled[arg.ID] = arg.Reason
	return nil
}

func (m *expoTestQuerier) InsertPushTicket(_ context.Context, arg db.InsertPushTicketParams) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.tickets = append(m.tickets, db.ListDuePushTicketsRow{
		ID: int64(len(m.tickets) + 1), TicketID: arg.TicketID, PushDeviceID: arg.PushDeviceID,
		CreatedAt: pgtype.Timestamptz{Time: time.Now().Add(-time.Hour), Valid: true},
	})
	return nil
}

func (m *expoTestQuerier) ListDuePushTickets(context.Context, db.ListDuePushTicketsParams) ([]db.ListDuePushTicketsRow, error) {
	var out []db.ListDuePushTicketsRow
	for _, t := range m.tickets {
		gone := false
		for _, d := range m.deleted {
			if d == t.ID {
				gone = true
			}
		}
		if !gone {
			out = append(out, t)
		}
	}
	return out, nil
}

func (m *expoTestQuerier) DeletePushTickets(_ context.Context, ids []int64) error {
	m.deleted = append(m.deleted, ids...)
	return nil
}

func (m *expoTestQuerier) EnsureNotificationPreferencesPushDefault(context.Context, int64) error {
	return nil
}

var _ pushDeviceStore = (*expoTestQuerier)(nil)

func exportRow(t *testing.T) db.Notification {
	t.Helper()
	payload, _ := json.Marshal(map[string]any{
		"kind": "exports.ready", "notification_uuid": "x",
		"push_vars": map[string]string{"resource": "Users", "email": "ahmet@example.com"},
	})
	return db.Notification{
		ID: 9, Uuid: uuid.New(), UserID: pgtype.Int8{Int64: 42, Valid: true}, Channel: "inapp",
		Title: "Export ready: users.xlsx", Body: "Ahmet Yılmaz (0532 000 00 00) exported 1.250 users.",
		Payload:   payload,
		ActionUrl: pgtype.Text{String: "/platform/exports/0b7c", Valid: true},
	}
}

func TestBuildPushMessageIsLockScreenSafe(t *testing.T) {
	row := exportRow(t)
	msg := BuildPushMessage(row, "tr", db.PushDevice{Token: "ExponentPushToken[abc]", Locale: "en"})
	if msg.Title != "Dışa aktarım hazır" || msg.Body != "Users dışa aktarımınız indirilmeye hazır." {
		t.Fatalf("tr text: %q / %q", msg.Title, msg.Body)
	}
	for _, leak := range []string{"1.250", "0532", "Ahmet", "example.com"} {
		if strings.Contains(msg.Title+msg.Body, leak) {
			t.Fatalf("push leaks %q: %q %q", leak, msg.Title, msg.Body)
		}
	}
	if msg.Data["url"] != "/platform/exports/0b7c" || msg.Data["notification_uuid"] != row.Uuid.String() || msg.Data["kind"] != "exports.ready" {
		t.Fatalf("data=%v", msg.Data)
	}
	// User locale wins; device locale is the fallback.
	if en := BuildPushMessage(row, "en", db.PushDevice{Locale: "tr"}); en.Title != "Export ready" {
		t.Fatalf("en title %q", en.Title)
	}
	if fb := BuildPushMessage(row, "", db.PushDevice{Locale: "en"}); fb.Title != "Export ready" {
		t.Fatalf("device fallback title %q", fb.Title)
	}
	// Unknown kind → generic text, never the in-app body.
	row.Payload = []byte(`{}`)
	if g := BuildPushMessage(row, "tr", db.PushDevice{}); g.Body != "Yeni bir bildiriminiz var." {
		t.Fatalf("generic body %q", g.Body)
	}
}

func TestSendMobilePushBatchesAndHandlesTickets(t *testing.T) {
	var (
		mu      sync.Mutex
		batches []int
		auth    string
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var msgs []expoMessage
		_ = json.NewDecoder(r.Body).Decode(&msgs)
		mu.Lock()
		batches = append(batches, len(msgs))
		auth = r.Header.Get("Authorization")
		mu.Unlock()
		out := make([]map[string]any, 0, len(msgs))
		for i, m := range msgs {
			if m.To == "ExponentPushToken[dead]" {
				out = append(out, map[string]any{"status": "error", "message": "gone", "details": map[string]string{"error": "DeviceNotRegistered"}})
				continue
			}
			out = append(out, map[string]any{"status": "ok", "id": fmt.Sprintf("ticket-%d-%d", len(msgs), i)})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": out})
	}))
	defer srv.Close()

	q := &expoTestQuerier{row: exportRow(t), user: db.User{ID: 42, Locale: "tr"}}
	q.prefs = db.NotificationPreference{PushEnabled: true}
	for i := range 150 {
		tok := fmt.Sprintf("ExponentPushToken[d%d]", i)
		if i == 3 {
			tok = "ExponentPushToken[dead]"
		}
		q.devices = append(q.devices, db.PushDevice{ID: int64(i + 1), UserID: 42, Token: tok, Platform: "ios"})
	}
	svc := &Service{q: q, log: slog.Default()}
	svc.WithExpo(ExpoConfig{AccessToken: "secret", PushURL: srv.URL, ReceiptsURL: srv.URL})

	if err := svc.SendMobilePush(context.Background(), 9); err != nil {
		t.Fatal(err)
	}
	if len(batches) != 2 || batches[0] != 100 || batches[1] != 50 {
		t.Fatalf("batches=%v", batches)
	}
	if auth != "Bearer secret" {
		t.Fatalf("authorization=%q", auth)
	}
	if q.disabled[4] != "DeviceNotRegistered" || len(q.disabled) != 1 {
		t.Fatalf("disabled=%v", q.disabled)
	}
	if len(q.tickets) != 149 {
		t.Fatalf("tickets=%d", len(q.tickets))
	}

	// push_enabled=false → nothing is sent.
	q.prefs.PushEnabled = false
	batches = nil
	if err := svc.SendMobilePush(context.Background(), 9); err != nil {
		t.Fatal(err)
	}
	if len(batches) != 0 {
		t.Fatalf("push disabled but sent %v", batches)
	}
}

func TestProcessPushReceiptsDisablesDeadTokens(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			IDs []string `json:"ids"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		data := map[string]any{}
		for _, id := range body.IDs {
			switch id {
			case "t-dead":
				data[id] = map[string]any{"status": "error", "details": map[string]string{"error": "DeviceNotRegistered"}}
			case "t-ok":
				data[id] = map[string]any{"status": "ok"}
			}
			// "t-pending" has no receipt yet.
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": data})
	}))
	defer srv.Close()
	q := &expoTestQuerier{}
	for i, id := range []string{"t-ok", "t-dead", "t-pending"} {
		_ = q.InsertPushTicket(context.Background(), db.InsertPushTicketParams{TicketID: id, PushDeviceID: int64(10 + i)})
	}
	svc := &Service{q: q, log: slog.Default()}
	svc.WithExpo(ExpoConfig{PushURL: srv.URL, ReceiptsURL: srv.URL})
	n, err := svc.ProcessPushReceipts(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 || q.disabled[11] != "DeviceNotRegistered" {
		t.Fatalf("disabled=%d %v", n, q.disabled)
	}
	if len(q.deleted) != 2 {
		t.Fatalf("deleted tickets=%v (pending one must stay)", q.deleted)
	}
}

func TestValidatePushDevice(t *testing.T) {
	ok := PushDeviceInput{Token: "ExponentPushToken[xxxxxxxxxxxxxxxxxxxxxx]", Platform: "ios"}
	if errs := ValidatePushDevice(ok); len(errs) != 0 {
		t.Fatalf("valid input rejected: %v", errs)
	}
	bad := PushDeviceInput{Token: "fcm:abc", Platform: "web"}
	errs := ValidatePushDevice(bad)
	if errs["token"] == "" || errs["platform"] == "" {
		t.Fatalf("errs=%v", errs)
	}
	for in, want := range map[string]string{"tr-TR": "tr", "en_US": "en", "EN": "en", "de": "", "": ""} {
		if got := normalizePushLocale(in); got != want {
			t.Fatalf("normalizePushLocale(%q)=%q want %q", in, got, want)
		}
	}
}

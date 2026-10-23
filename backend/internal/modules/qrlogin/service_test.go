package qrlogin

import (
	"context"
	"errors"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/auth/model"
	authusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/auth/usecase"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

type fakeIssuer struct{}

func (fakeIssuer) Enabled() bool { return true }
func (fakeIssuer) WSURL() string { return "wss://rt.example/connection/websocket" }
func (fakeIssuer) AnonymousConnectionToken(time.Time) (string, error) {
	return "conn-token", nil
}
func (fakeIssuer) AnonymousSubscriptionToken(ch string, _ time.Time) (string, error) {
	return "sub:" + ch, nil
}

type event struct {
	channel string
	data    map[string]any
}

type recorder struct {
	mu     sync.Mutex
	events []event
}

func (r *recorder) Publish(_ context.Context, ch string, data any) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	m, _ := data.(map[string]any)
	r.events = append(r.events, event{ch, m})
	return nil
}

func (r *recorder) types() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]string, 0, len(r.events))
	for _, e := range r.events {
		out = append(out, e.data["type"].(string))
	}
	return out
}

type fakeAccounts struct {
	deactivated map[uuid.UUID]bool
	issued      atomic.Int32
	lastMeta    model.SessionMeta
}

func (f *fakeAccounts) CheckQRLoginApprover(_ context.Context, id uuid.UUID) error {
	if f.deactivated[id] {
		return authusecase.ErrAccountDeactivated
	}
	return nil
}

func (f *fakeAccounts) IssueQRLoginSession(_ context.Context, id uuid.UUID, meta model.SessionMeta) (model.Tokens, authusecase.QRLoginUser, error) {
	if f.deactivated[id] {
		return model.Tokens{}, authusecase.QRLoginUser{}, authusecase.ErrAccountDeactivated
	}
	f.issued.Add(1)
	f.lastMeta = meta
	return model.Tokens{AccessToken: "at", RefreshToken: "rt", TokenType: "Bearer"},
		authusecase.QRLoginUser{UUID: id, Email: "u@example.com", Name: "U"}, nil
}

type fixture struct {
	svc  *Service
	mr   *miniredis.Miniredis
	pub  *recorder
	acc  *fakeAccounts
	user uuid.UUID
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	pub := &recorder{}
	acc := &fakeAccounts{deactivated: map[uuid.UUID]bool{}}
	svc := NewService(NewStore(rdb, "test"), acc, fakeIssuer{}, pub, "https://app.example.com/", nil)
	return &fixture{svc: svc, mr: mr, pub: pub, acc: acc, user: uuid.New()}
}

func (f *fixture) approver() Approver {
	return Approver{UserUUID: f.user}
}

func TestCreateShape(t *testing.T) {
	f := newFixture(t)
	c, err := f.svc.Create(context.Background(), "Mozilla/5.0 (Windows NT 10.0) Chrome/128.0 Safari/537.36", "203.0.113.9", Location{CountryCode: "TR", City: "Istanbul", Source: "edge"})
	if err != nil {
		t.Fatal(err)
	}
	if !ValidID(c.SessionID) || !ValidID(c.BrowserSecret) || c.SessionID == c.BrowserSecret {
		t.Fatalf("bad ids: %+v", c)
	}
	if c.QRURL != "https://app.example.com/login/qr/"+c.SessionID {
		t.Fatalf("qr url %s", c.QRURL)
	}
	if !strings.HasPrefix(c.Realtime.Channel, "qrlogin:") || strings.Contains(c.Realtime.Channel, c.SessionID) {
		t.Fatalf("channel must be private and unrelated to the QR id: %s", c.Realtime.Channel)
	}
	if c.Realtime.SubscriptionToken != "sub:"+c.Realtime.Channel || c.ExpiresIn != 120 {
		t.Fatalf("realtime grant %+v", c)
	}
	if ttl := f.mr.TTL(f.svc.store.key(c.SessionID)); ttl <= 0 || ttl > SessionTTL {
		t.Fatalf("ttl %v", ttl)
	}
	// Redis never stores the raw id or secret.
	for _, k := range f.mr.Keys() {
		if strings.Contains(k, c.SessionID) {
			t.Fatal("raw session id in redis key")
		}
		if v := f.mr.HGet(k, "secret_hash"); v == c.BrowserSecret {
			t.Fatal("raw browser secret stored")
		}
	}
}

func TestApproveExchangeHappyPath(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	c, _ := f.svc.Create(ctx, "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) Version/17.4 Safari/605.1.15", "198.51.100.7", Location{})
	d, err := f.svc.Get(ctx, c.SessionID, f.approver())
	if err != nil {
		t.Fatal(err)
	}
	if d.Status != StatusPending || d.Client.Browser != "Safari" || d.Client.OS != "macOS" || d.IP != "198.51.100.7" || d.Location != nil {
		t.Fatalf("details %+v", d)
	}
	// A second look by the same user does not re-announce "scanned".
	if _, err := f.svc.Get(ctx, c.SessionID, f.approver()); err != nil {
		t.Fatal(err)
	}
	if err := f.svc.Approve(ctx, c.SessionID, f.approver()); err != nil {
		t.Fatal(err)
	}
	if got := f.pub.types(); strings.Join(got, ",") != "scanned,approved" {
		t.Fatalf("events %v", got)
	}
	approved := f.pub.events[1]
	if approved.channel != c.Realtime.Channel {
		t.Fatalf("published to %s", approved.channel)
	}
	exch, _ := approved.data["exchange_token"].(string)
	// The tab's catch-up read returns the same token.
	st, err := f.svc.State(ctx, c.SessionID, c.BrowserSecret)
	if err != nil || st.Status != StatusApproved || st.ExchangeToken != exch {
		t.Fatalf("state %+v %v", st, err)
	}
	out, err := f.svc.Exchange(ctx, c.SessionID, c.BrowserSecret, exch)
	if err != nil {
		t.Fatal(err)
	}
	if out.AccessToken != "at" || out.User.UUID != f.user.String() {
		t.Fatalf("exchange %+v", out)
	}
	if f.acc.lastMeta.IP != "198.51.100.7" {
		t.Fatalf("meta not carried: %+v", f.acc.lastMeta)
	}
	// Single use.
	if _, err := f.svc.Exchange(ctx, c.SessionID, c.BrowserSecret, exch); !errors.Is(err, ErrNotFound) {
		t.Fatalf("replay err = %v", err)
	}
}

func TestExchangeNeedsBrowserSecretAndBurnsAttempts(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	c, _ := f.svc.Create(ctx, "ua", "1.2.3.4", Location{})
	_ = f.svc.Approve(ctx, c.SessionID, f.approver())
	exch := f.pub.events[0].data["exchange_token"].(string)
	other, _ := randomToken()
	// Someone who only saw the channel event (or the QR) cannot redeem it.
	for i := 0; i < maxSecretAttempts; i++ {
		if _, err := f.svc.Exchange(ctx, c.SessionID, other, exch); !errors.Is(err, ErrInvalidSecret) {
			t.Fatalf("attempt %d: %v", i, err)
		}
	}
	// After too many wrong secrets the session is gone, even for the real tab.
	if _, err := f.svc.Exchange(ctx, c.SessionID, c.BrowserSecret, exch); !errors.Is(err, ErrNotFound) {
		t.Fatalf("burned session err = %v", err)
	}
	if f.acc.issued.Load() != 0 {
		t.Fatal("no session may be issued")
	}
}

func TestExchangeBeforeApprovalAndWrongToken(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	c, _ := f.svc.Create(ctx, "ua", "1.2.3.4", Location{})
	guess, _ := randomToken()
	if _, err := f.svc.Exchange(ctx, c.SessionID, c.BrowserSecret, guess); !errors.Is(err, ErrInvalidSecret) {
		t.Fatalf("pending exchange err = %v", err)
	}
	if _, err := f.svc.Exchange(ctx, c.SessionID, c.BrowserSecret, "short"); !errors.Is(err, ErrInvalidSecret) {
		t.Fatalf("malformed token err = %v", err)
	}
}

func TestRejectAndDoubleResolve(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	c, _ := f.svc.Create(ctx, "ua", "1.2.3.4", Location{})
	if err := f.svc.Reject(ctx, c.SessionID, f.approver()); err != nil {
		t.Fatal(err)
	}
	if err := f.svc.Approve(ctx, c.SessionID, f.approver()); !errors.Is(err, ErrResolved) {
		t.Fatalf("approve after reject = %v", err)
	}
	st, err := f.svc.State(ctx, c.SessionID, c.BrowserSecret)
	if err != nil || st.Status != StatusRejected || st.ExchangeToken != "" {
		t.Fatalf("state %+v %v", st, err)
	}
	if got := f.pub.types(); strings.Join(got, ",") != "rejected" {
		t.Fatalf("events %v", got)
	}
}

func TestConcurrentApproveOnlyOneWins(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	c, _ := f.svc.Create(ctx, "ua", "1.2.3.4", Location{})
	var wins atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if f.svc.Approve(ctx, c.SessionID, f.approver()) == nil {
				wins.Add(1)
			}
		}()
	}
	wg.Wait()
	if wins.Load() != 1 {
		t.Fatalf("wins = %d", wins.Load())
	}
}

func TestOtherAccountCannotTakeOverClaimedSession(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	c, _ := f.svc.Create(ctx, "ua", "1.2.3.4", Location{})
	if _, err := f.svc.Get(ctx, c.SessionID, f.approver()); err != nil {
		t.Fatal(err)
	}
	intruder := Approver{UserUUID: uuid.New()}
	if _, err := f.svc.Get(ctx, c.SessionID, intruder); !errors.Is(err, ErrClaimed) {
		t.Fatalf("get = %v", err)
	}
	if err := f.svc.Approve(ctx, c.SessionID, intruder); !errors.Is(err, ErrClaimed) {
		t.Fatalf("approve = %v", err)
	}
	if err := f.svc.Reject(ctx, c.SessionID, intruder); !errors.Is(err, ErrClaimed) {
		t.Fatalf("reject = %v", err)
	}
}

func TestDeactivatedAndImpersonatingCannotApprove(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	c, _ := f.svc.Create(ctx, "ua", "1.2.3.4", Location{})
	f.acc.deactivated[f.user] = true
	if err := f.svc.Approve(ctx, c.SessionID, f.approver()); !errors.Is(err, authusecase.ErrAccountDeactivated) {
		t.Fatalf("deactivated = %v", err)
	}
	imp := f.approver()
	imp.Impersonating = true
	if err := f.svc.Approve(ctx, c.SessionID, imp); !errors.Is(err, ErrImpersonating) {
		t.Fatalf("impersonating = %v", err)
	}
	if len(f.pub.events) != 0 {
		t.Fatal("nothing may be published")
	}
}

func TestExpiry(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	c, _ := f.svc.Create(ctx, "ua", "1.2.3.4", Location{})
	f.mr.FastForward(SessionTTL + time.Second)
	if _, err := f.svc.Get(ctx, c.SessionID, f.approver()); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expired get = %v", err)
	}
	if err := f.svc.Approve(ctx, c.SessionID, f.approver()); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expired approve = %v", err)
	}
}

func TestStateWrongSecret(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	c, _ := f.svc.Create(ctx, "ua", "1.2.3.4", Location{})
	other, _ := randomToken()
	if _, err := f.svc.State(ctx, c.SessionID, other); !errors.Is(err, ErrInvalidSecret) {
		t.Fatalf("state = %v", err)
	}
	if _, err := f.svc.Get(ctx, "not-an-id", f.approver()); !errors.Is(err, ErrNotFound) {
		t.Fatalf("malformed id = %v", err)
	}
}

func TestParseUserAgent(t *testing.T) {
	cases := []struct{ ua, browser, ver, os, device string }{
		{"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/128.0.0.0 Safari/537.36 Edg/128.0.2739.42", "Edge", "128", "Windows", "desktop"},
		{"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.4 Safari/605.1.15", "Safari", "17", "macOS", "desktop"},
		{"Mozilla/5.0 (X11; Linux x86_64; rv:129.0) Gecko/20100101 Firefox/129.0", "Firefox", "129", "Linux", "desktop"},
		{"Mozilla/5.0 (iPhone; CPU iPhone OS 17_5 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) CriOS/127.0.6533.107 Mobile/15E148 Safari/604.1", "Chrome", "127", "iOS", "mobile"},
		{"Mozilla/5.0 (Linux; Android 14; SM-S918B) AppleWebKit/537.36 (KHTML, like Gecko) SamsungBrowser/25.0 Chrome/121.0.0.0 Mobile Safari/537.36", "Samsung Internet", "25", "Android", "mobile"},
		{"Mozilla/5.0 (X11; CrOS x86_64 14541.0.0) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/128.0.0.0 Safari/537.36 OPR/112.0.0.0", "Opera", "112", "ChromeOS", "desktop"},
		{"curl/8.0", "", "", "", "desktop"},
	}
	for _, c := range cases {
		got := ParseUserAgent(c.ua)
		if got.Browser != c.browser || got.BrowserVersion != c.ver || got.OS != c.os || got.DeviceType != c.device {
			t.Errorf("%q → %+v", c.ua, got)
		}
	}
}

func TestLocatorHeaders(t *testing.T) {
	r := httptest.NewRequest("POST", "/", nil)
	r.Header.Set("CF-IPCountry", "tr")
	r.Header.Set("CF-IPCity", "%C4%B0stanbul")
	r.Header.Set("CF-Region", "Istanbul")

	untrusted, _ := NewLocator("", false)
	if loc := untrusted.Locate(r, "203.0.113.1"); !loc.Empty() {
		t.Fatalf("untrusted headers must be ignored: %+v", loc)
	}
	trusted, _ := NewLocator("", true)
	loc := trusted.Locate(r, "203.0.113.1")
	if loc.CountryCode != "TR" || loc.City != "İstanbul" || loc.Source != "edge" {
		t.Fatalf("loc %+v", loc)
	}
	r.Header.Set("CF-IPCountry", "XX")
	r.Header.Del("CF-IPCity")
	r.Header.Del("CF-Region")
	if loc := trusted.Locate(r, "203.0.113.1"); !loc.Empty() {
		t.Fatalf("unknown country must be empty: %+v", loc)
	}
	if _, err := NewLocator("/nonexistent.mmdb", false); err == nil {
		t.Fatal("missing db must error")
	}
}

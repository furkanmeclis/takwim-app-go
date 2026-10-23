package usecase

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/notifications/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/notifications/pushtext"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/queue"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// Expo Push Service endpoints and limits.
const (
	DefaultExpoPushURL     = "https://exp.host/--/api/v2/push/send"
	DefaultExpoReceiptsURL = "https://exp.host/--/api/v2/push/getReceipts"

	expoSendBatch     = 100
	expoReceiptsBatch = 1000
	// Receipts are read ~15 minutes after sending and kept by Expo ~24 hours.
	expoReceiptDelay  = 15 * time.Minute
	expoReceiptMaxAge = 24 * time.Hour

	expoErrDeviceNotRegistered = "DeviceNotRegistered"
)

var expoTokenRE = regexp.MustCompile(`^(ExponentPushToken|ExpoPushToken)\[[^\]\s]{1,200}\]$`)

// ExpoConfig configures the Expo Push Service client. AccessToken is optional
// (required only when "enhanced push security" is on for the Expo project).
type ExpoConfig struct {
	AccessToken string
	PushURL     string
	ReceiptsURL string
	HTTP        *http.Client
}

type expoClient struct {
	cfg  ExpoConfig
	http *http.Client
}

// WithExpo configures the Expo push client (always usable; token optional).
func (s *Service) WithExpo(cfg ExpoConfig) *Service {
	if cfg.PushURL == "" {
		cfg.PushURL = DefaultExpoPushURL
	}
	if cfg.ReceiptsURL == "" {
		cfg.ReceiptsURL = DefaultExpoReceiptsURL
	}
	hc := cfg.HTTP
	if hc == nil {
		hc = &http.Client{Timeout: 20 * time.Second}
	}
	s.expo = &expoClient{cfg: cfg, http: hc}
	return s
}

// WithPushQueue sends mobile pushes through the queue (retries on Expo
// errors) even when notification delivery itself runs inline (worker).
func (s *Service) WithPushQueue(enq Enqueuer) *Service {
	s.pushQueue = enq
	return s
}

// pushDeviceStore is the persistence surface for mobile devices.
type pushDeviceStore interface {
	UpsertPushDevice(ctx context.Context, arg db.UpsertPushDeviceParams) (db.PushDevice, error)
	DeletePushDeviceForUser(ctx context.Context, arg db.DeletePushDeviceForUserParams) (int64, error)
	ListActivePushDevicesByUser(ctx context.Context, userID int64) ([]db.PushDevice, error)
	HasActivePushDevices(ctx context.Context, userID int64) (bool, error)
	DisablePushDevice(ctx context.Context, arg db.DisablePushDeviceParams) error
	InsertPushTicket(ctx context.Context, arg db.InsertPushTicketParams) error
	ListDuePushTickets(ctx context.Context, arg db.ListDuePushTicketsParams) ([]db.ListDuePushTicketsRow, error)
	DeletePushTickets(ctx context.Context, ids []int64) error
	EnsureNotificationPreferencesPushDefault(ctx context.Context, userID int64) error
}

func (s *Service) devices() (pushDeviceStore, bool) {
	st, ok := s.q.(pushDeviceStore)
	return st, ok
}

// PushDeviceInput registers a mobile device.
type PushDeviceInput struct {
	Token      string
	Platform   string
	Locale     string
	AppVersion string
	DeviceName string
}

// ValidatePushDevice returns field errors (field → code) for a registration.
func ValidatePushDevice(in PushDeviceInput) map[string]string {
	errs := map[string]string{}
	if !expoTokenRE.MatchString(strings.TrimSpace(in.Token)) {
		errs["token"] = "invalid"
	}
	switch strings.ToLower(strings.TrimSpace(in.Platform)) {
	case "ios", "android":
	default:
		errs["platform"] = "invalid"
	}
	return errs
}

// RegisterPushDevice upserts the device for the user (token moves between
// accounts on the same phone) and refreshes locale / app version / last seen.
// The first registration opts the user into push; an explicit
// push_enabled=false preference is kept.
func (s *Service) RegisterPushDevice(ctx context.Context, userID int64, in PushDeviceInput) (model.PushDevice, bool, error) {
	if errs := ValidatePushDevice(in); len(errs) > 0 {
		return model.PushDevice{}, false, fmt.Errorf("%w: invalid push device", ErrInvalidRequest)
	}
	st, ok := s.devices()
	if !ok {
		return model.PushDevice{}, false, ErrInvalidRequest
	}
	row, err := st.UpsertPushDevice(ctx, db.UpsertPushDeviceParams{
		UserID:     userID,
		Token:      strings.TrimSpace(in.Token),
		Platform:   strings.ToLower(strings.TrimSpace(in.Platform)),
		Locale:     normalizePushLocale(in.Locale),
		AppVersion: truncate(strings.TrimSpace(in.AppVersion), 32),
		DeviceName: truncate(strings.TrimSpace(in.DeviceName), 120),
	})
	if err != nil {
		return model.PushDevice{}, false, err
	}
	if err := st.EnsureNotificationPreferencesPushDefault(ctx, userID); err != nil {
		return model.PushDevice{}, false, err
	}
	prefs, err := s.GetPreferences(ctx, userID)
	if err != nil {
		return model.PushDevice{}, false, err
	}
	return projectPushDevice(row), prefs.PushEnabled, nil
}

// UnregisterPushDevice removes the caller's device (idempotent).
func (s *Service) UnregisterPushDevice(ctx context.Context, userID int64, token string) error {
	st, ok := s.devices()
	if !ok {
		return ErrInvalidRequest
	}
	_, err := st.DeletePushDeviceForUser(ctx, db.DeletePushDeviceForUserParams{UserID: userID, Token: strings.TrimSpace(token)})
	return err
}

// withPushVars copies the lock-screen-safe template variables (pushtext
// SafeVars) into an in-app payload under "push_vars" for the mobile mirror.
func withPushVars(channel string, payload map[string]any, vars map[string]string) map[string]any {
	out := map[string]any{}
	for k, v := range payload {
		out[k] = v
	}
	if channel != model.ChannelInapp {
		return out
	}
	if _, set := out["push_vars"]; set {
		return out
	}
	pv := map[string]string{}
	for _, k := range pushtext.SafeVars {
		if v := vars[k]; v != "" {
			pv[k] = v
		}
	}
	if len(pv) > 0 {
		out["push_vars"] = pv
	}
	return out
}

// scheduleMobilePush queues the Expo mirror of a delivered in-app / push row
// when the user has an active device. Inline when there is no queue.
func (s *Service) scheduleMobilePush(ctx context.Context, row db.Notification) {
	if s.expo == nil || !row.UserID.Valid {
		return
	}
	st, ok := s.devices()
	if !ok {
		return
	}
	has, err := st.HasActivePushDevices(ctx, row.UserID.Int64)
	if err != nil || !has {
		return
	}
	enq := s.pushQueue
	if enq == nil && !s.syncMode {
		enq = s.queue
	}
	if enq != nil {
		task, err := queue.NewPushSendTask(row.ID)
		if err == nil {
			_, err = enq.Enqueue(task, queue.PushSendOptions()...)
		}
		if err != nil {
			s.log.Error("push_enqueue_failed", "notification_id", row.ID, "error", err)
		}
		return
	}
	if err := s.SendMobilePush(ctx, row.ID); err != nil {
		s.log.Error("push_send_failed", "notification_id", row.ID, "error", err)
	}
}

type expoMessage struct {
	To       string         `json:"to"`
	Title    string         `json:"title"`
	Body     string         `json:"body"`
	Data     map[string]any `json:"data"`
	Sound    string         `json:"sound,omitempty"`
	Priority string         `json:"priority,omitempty"`
}

type expoTicket struct {
	Status  string `json:"status"`
	ID      string `json:"id"`
	Message string `json:"message"`
	Details struct {
		Error string `json:"error"`
	} `json:"details"`
}

// BuildPushMessage renders the lock-screen-safe message for one device.
// Title/body come from pushtext (never the in-app text, which may carry
// amounts or phone numbers); data.url is the web path of the notification.
func BuildPushMessage(row db.Notification, userLocale string, dev db.PushDevice) expoMessage {
	var payload map[string]any
	_ = json.Unmarshal(row.Payload, &payload)
	kind, _ := payload["kind"].(string)
	if kind == "" && row.TemplateCode.Valid {
		kind = row.TemplateCode.String
	}
	vars := map[string]string{}
	if pv, ok := payload["push_vars"].(map[string]any); ok {
		for k, v := range pv {
			if sv, ok := v.(string); ok {
				vars[k] = sv
			}
		}
	}
	locale := normalizePushLocale(userLocale)
	if locale == "" {
		locale = dev.Locale
	}
	if locale == "" {
		locale = "tr"
	}
	txt := pushtext.For(kind, locale, vars)
	data := map[string]any{"notification_uuid": row.Uuid.String()}
	if kind != "" {
		data["kind"] = kind
	}
	if row.ActionUrl.Valid && strings.HasPrefix(row.ActionUrl.String, "/") {
		data["url"] = row.ActionUrl.String
	}
	return expoMessage{To: dev.Token, Title: txt.Title, Body: txt.Body, Data: data, Sound: "default", Priority: "high"}
}

// SendMobilePush sends the Expo push for a notification row to all active
// devices of its user (batches of 100). Honors push_enabled.
func (s *Service) SendMobilePush(ctx context.Context, notificationID int64) error {
	if s.expo == nil {
		return nil
	}
	st, ok := s.devices()
	if !ok {
		return nil
	}
	row, err := s.q.GetNotificationByID(ctx, notificationID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if !row.UserID.Valid {
		return nil
	}
	userID := row.UserID.Int64
	prefs, err := s.GetPreferences(ctx, userID)
	if err != nil {
		return err
	}
	if !prefs.PushEnabled {
		return nil
	}
	devs, err := st.ListActivePushDevicesByUser(ctx, userID)
	if err != nil || len(devs) == 0 {
		return err
	}
	userLocale := ""
	if u, err := s.q.GetUserByID(ctx, userID); err == nil {
		userLocale = u.Locale
	}
	msgs := make([]expoMessage, 0, len(devs))
	for _, d := range devs {
		msgs = append(msgs, BuildPushMessage(row, userLocale, d))
	}
	for start := 0; start < len(msgs); start += expoSendBatch {
		end := min(start+expoSendBatch, len(msgs))
		tickets, err := s.expo.send(ctx, msgs[start:end])
		if err != nil {
			return err
		}
		for i, t := range tickets {
			if start+i >= len(devs) {
				break
			}
			dev := devs[start+i]
			switch {
			case t.Status == "ok" && t.ID != "":
				_ = st.InsertPushTicket(ctx, db.InsertPushTicketParams{
					TicketID: t.ID, PushDeviceID: dev.ID,
					NotificationID: pgtype.Int8{Int64: row.ID, Valid: true},
				})
			case t.Details.Error == expoErrDeviceNotRegistered:
				_ = st.DisablePushDevice(ctx, db.DisablePushDeviceParams{ID: dev.ID, Reason: expoErrDeviceNotRegistered})
			case t.Status == "error":
				s.log.Warn("expo_push_ticket_error", "device_id", dev.ID, "error", t.Details.Error, "message", t.Message)
			}
		}
	}
	return nil
}

// ProcessPushReceipts reads receipts for tickets older than 15 minutes,
// disables tokens reported as DeviceNotRegistered and deletes processed
// tickets. Returns the number of disabled devices.
func (s *Service) ProcessPushReceipts(ctx context.Context) (int, error) {
	if s.expo == nil {
		return 0, nil
	}
	st, ok := s.devices()
	if !ok {
		return 0, nil
	}
	now := time.Now()
	disabled := 0
	for range 20 {
		rows, err := st.ListDuePushTickets(ctx, db.ListDuePushTicketsParams{
			BeforeAt:   pgtype.Timestamptz{Time: now.Add(-expoReceiptDelay), Valid: true},
			LimitCount: expoReceiptsBatch,
		})
		if err != nil || len(rows) == 0 {
			return disabled, err
		}
		ids := make([]string, 0, len(rows))
		for _, r := range rows {
			ids = append(ids, r.TicketID)
		}
		receipts, err := s.expo.receipts(ctx, ids)
		if err != nil {
			return disabled, err
		}
		var done []int64
		for _, r := range rows {
			rc, ok := receipts[r.TicketID]
			if !ok {
				if now.Sub(r.CreatedAt.Time) > expoReceiptMaxAge {
					done = append(done, r.ID)
				}
				continue
			}
			if rc.Status == "error" {
				if rc.Details.Error == expoErrDeviceNotRegistered {
					if err := st.DisablePushDevice(ctx, db.DisablePushDeviceParams{ID: r.PushDeviceID, Reason: expoErrDeviceNotRegistered}); err == nil {
						disabled++
					}
				} else {
					s.log.Warn("expo_push_receipt_error", "device_id", r.PushDeviceID, "error", rc.Details.Error, "message", rc.Message)
				}
			}
			done = append(done, r.ID)
		}
		if len(done) == 0 {
			return disabled, nil
		}
		if err := st.DeletePushTickets(ctx, done); err != nil {
			return disabled, err
		}
		if len(rows) < expoReceiptsBatch {
			return disabled, nil
		}
	}
	return disabled, nil
}

func (c *expoClient) post(ctx context.Context, url string, body any, out any) error {
	buf, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(buf))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if c.cfg.AccessToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.cfg.AccessToken)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("expo: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if resp.StatusCode >= 300 {
		return fmt.Errorf("expo: http %d: %s", resp.StatusCode, truncate(string(raw), 300))
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("expo: decode: %w", err)
	}
	return nil
}

func (c *expoClient) send(ctx context.Context, msgs []expoMessage) ([]expoTicket, error) {
	var out struct {
		Data   []expoTicket `json:"data"`
		Errors []struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := c.post(ctx, c.cfg.PushURL, msgs, &out); err != nil {
		return nil, err
	}
	if len(out.Data) == 0 && len(out.Errors) > 0 {
		return nil, fmt.Errorf("expo: %s: %s", out.Errors[0].Code, out.Errors[0].Message)
	}
	return out.Data, nil
}

func (c *expoClient) receipts(ctx context.Context, ids []string) (map[string]expoTicket, error) {
	var out struct {
		Data map[string]expoTicket `json:"data"`
	}
	if err := c.post(ctx, c.cfg.ReceiptsURL, map[string]any{"ids": ids}, &out); err != nil {
		return nil, err
	}
	if out.Data == nil {
		out.Data = map[string]expoTicket{}
	}
	return out.Data, nil
}

func projectPushDevice(r db.PushDevice) model.PushDevice {
	return model.PushDevice{
		UUID: r.Uuid, Token: r.Token, Platform: r.Platform, Locale: r.Locale,
		AppVersion: r.AppVersion, DeviceName: r.DeviceName,
		LastSeenAt: r.LastSeenAt.Time, CreatedAt: r.CreatedAt.Time,
	}
}

// normalizePushLocale maps "tr-TR" / "en_US" / "EN" to tr|en; others → "".
func normalizePushLocale(v string) string {
	v = strings.ToLower(strings.TrimSpace(v))
	if len(v) > 2 {
		v = v[:2]
	}
	switch v {
	case "tr", "en":
		return v
	}
	return ""
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}

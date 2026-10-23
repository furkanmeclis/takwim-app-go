package sms

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"
)

func TestMask(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"+905321234567": "+90532*****67",
		"+4930123456":   "+49301***56",
		"12345":         "*****",
	}
	for in, want := range cases {
		if got := Mask(in); got != want {
			t.Errorf("Mask(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestLogSenderHidesTextUnlessRevealed(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	log := slog.New(slog.NewTextHandler(&buf, nil))
	msg := Message{To: "+905321234567", Text: "Kod: 123456", Purpose: PurposeCustomerOTP}
	if err := NewLogSender(log, false).Send(context.Background(), msg); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(buf.String(), "123456") || strings.Contains(buf.String(), "+905321234567") {
		t.Fatalf("hidden sender leaked: %s", buf.String())
	}
	buf.Reset()
	if err := NewLogSender(log, true).Send(context.Background(), msg); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "123456") || strings.Contains(buf.String(), "+905321234567") {
		t.Fatalf("dev sender = %s", buf.String())
	}
}

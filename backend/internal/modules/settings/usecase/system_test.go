package usecase

import (
	"context"
	"testing"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/config"
)

func TestSystemService_GetInfo(t *testing.T) {
	cfg := config.Config{
		App: config.AppConfig{
			Name: "app-test",
			Env:  "test",
		},
		Log: config.LogConfig{
			Level: "info",
		},
	}

	svc := NewSystemService(cfg, nil, nil, nil, nil)
	info := svc.GetInfo(context.Background())

	if info.Environment.AppName != "app-test" {
		t.Errorf("expected app_name 'app-test', got %s", info.Environment.AppName)
	}
	if info.Runtime.GoVersion == "" {
		t.Errorf("expected GoVersion to be set")
	}
	if info.Runtime.NumCPU <= 0 {
		t.Errorf("expected NumCPU > 0")
	}
	if info.Services.Database.Status != "disabled" {
		t.Errorf("expected Database.Status 'disabled' without db, got %s", info.Services.Database.Status)
	}
}

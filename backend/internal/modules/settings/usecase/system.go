package usecase

import (
	"context"
	"math"
	"runtime"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/config"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/searchengine"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/storage"
)

var processStartTime = time.Now()

type MemoryStats struct {
	AllocBytes      uint64 `json:"alloc_bytes"`
	TotalAllocBytes uint64 `json:"total_alloc_bytes"`
	SysBytes        uint64 `json:"sys_bytes"`
	HeapAllocBytes  uint64 `json:"heap_alloc_bytes"`
	HeapSysBytes    uint64 `json:"heap_sys_bytes"`
	HeapIdleBytes   uint64 `json:"heap_idle_bytes"`
	HeapInuseBytes  uint64 `json:"heap_inuse_bytes"`
	NumGC           uint32 `json:"num_gc"`
}

type RuntimeInfo struct {
	GoVersion     string      `json:"go_version"`
	NumCPU        int         `json:"num_cpu"`
	NumGoroutine  int         `json:"num_goroutine"`
	Compiler      string      `json:"compiler"`
	Arch          string      `json:"arch"`
	OS            string      `json:"os"`
	Memory        MemoryStats `json:"memory"`
	UptimeSeconds float64     `json:"uptime_seconds"`
	ServerTime    time.Time   `json:"server_time"`
}

type PoolStats struct {
	TotalConns    int32 `json:"total_conns"`
	IdleConns     int32 `json:"idle_conns"`
	AcquiredConns int32 `json:"acquired_conns"`
	MaxConns      int32 `json:"max_conns"`
}

type ServiceHealth struct {
	Status    string  `json:"status"` // "healthy", "degraded", "down", "disabled"
	LatencyMS float64 `json:"latency_ms"`
	Message   string  `json:"message,omitempty"`
}

type DatabaseHealth struct {
	ServiceHealth
	Pool PoolStats `json:"pool"`
}

type StorageHealth struct {
	ServiceHealth
	Driver string `json:"driver"`
}

type SystemServicesHealth struct {
	Database    DatabaseHealth `json:"database"`
	Redis       ServiceHealth  `json:"redis"`
	Storage     StorageHealth  `json:"storage"`
	Meilisearch ServiceHealth  `json:"meilisearch"`
}

type EnvironmentInfo struct {
	AppName           string `json:"app_name"`
	Env               string `json:"env"`
	LogLevel          string `json:"log_level"`
	CentrifugoEnabled bool   `json:"centrifugo_enabled"`
	VAPIDEnabled      bool   `json:"vapid_enabled"`
	SearchEnabled     bool   `json:"search_enabled"`
}

type SystemInfo struct {
	Status      string               `json:"status"` // "healthy", "degraded", "down"
	Runtime     RuntimeInfo          `json:"runtime"`
	Services    SystemServicesHealth `json:"services"`
	Environment EnvironmentInfo      `json:"environment"`
}

type SystemService struct {
	cfg          config.Config
	db           *pgxpool.Pool
	redis        *redis.Client
	storage      storage.Driver
	searchClient *searchengine.Client
}

func NewSystemService(
	cfg config.Config,
	db *pgxpool.Pool,
	rdb *redis.Client,
	store storage.Driver,
	searchClient *searchengine.Client,
) *SystemService {
	return &SystemService{
		cfg:          cfg,
		db:           db,
		redis:        rdb,
		storage:      store,
		searchClient: searchClient,
	}
}

func (s *SystemService) GetInfo(ctx context.Context) SystemInfo {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	now := time.Now().UTC()
	uptime := math.Round(time.Since(processStartTime).Seconds()*10) / 10

	runtimeInfo := RuntimeInfo{
		GoVersion:    runtime.Version(),
		NumCPU:       runtime.NumCPU(),
		NumGoroutine: runtime.NumGoroutine(),
		Compiler:     runtime.Compiler,
		Arch:         runtime.GOARCH,
		OS:           runtime.GOOS,
		Memory: MemoryStats{
			AllocBytes:      m.Alloc,
			TotalAllocBytes: m.TotalAlloc,
			SysBytes:        m.Sys,
			HeapAllocBytes:  m.HeapAlloc,
			HeapSysBytes:    m.HeapSys,
			HeapIdleBytes:   m.HeapIdle,
			HeapInuseBytes:  m.HeapInuse,
			NumGC:           m.NumGC,
		},
		UptimeSeconds: uptime,
		ServerTime:    now,
	}

	servicesHealth := s.checkServices(ctx)

	overallStatus := "healthy"
	if servicesHealth.Database.Status == "down" || servicesHealth.Redis.Status == "down" {
		overallStatus = "down"
	} else if servicesHealth.Database.Status == "degraded" ||
		servicesHealth.Redis.Status == "degraded" ||
		servicesHealth.Storage.Status == "down" ||
		servicesHealth.Meilisearch.Status == "down" {
		overallStatus = "degraded"
	}

	envInfo := EnvironmentInfo{
		AppName:           s.cfg.App.Name,
		Env:               s.cfg.App.Env,
		LogLevel:          s.cfg.Log.Level,
		CentrifugoEnabled: s.cfg.Centrifugo.Enabled,
		VAPIDEnabled:      s.cfg.VAPID.PublicKey != "",
		SearchEnabled:     s.searchClient != nil && s.searchClient.Enabled(),
	}

	return SystemInfo{
		Status:      overallStatus,
		Runtime:     runtimeInfo,
		Services:    servicesHealth,
		Environment: envInfo,
	}
}

func (s *SystemService) checkServices(ctx context.Context) SystemServicesHealth {
	var health SystemServicesHealth

	// DB Check
	if s.db != nil {
		t0 := time.Now()
		err := s.db.Ping(ctx)
		lat := roundLatency(time.Since(t0))
		stat := s.db.Stat()

		poolStats := PoolStats{
			TotalConns:    stat.TotalConns(),
			IdleConns:     stat.IdleConns(),
			AcquiredConns: stat.AcquiredConns(),
			MaxConns:      stat.MaxConns(),
		}

		if err != nil {
			health.Database = DatabaseHealth{
				ServiceHealth: ServiceHealth{
					Status:    "down",
					LatencyMS: lat,
					Message:   err.Error(),
				},
				Pool: poolStats,
			}
		} else {
			status := "healthy"
			if lat > 200 {
				status = "degraded"
			}
			health.Database = DatabaseHealth{
				ServiceHealth: ServiceHealth{
					Status:    status,
					LatencyMS: lat,
				},
				Pool: poolStats,
			}
		}
	} else {
		health.Database = DatabaseHealth{
			ServiceHealth: ServiceHealth{
				Status:  "disabled",
				Message: "no database configured",
			},
		}
	}

	// Redis Check
	if s.redis != nil {
		t0 := time.Now()
		err := s.redis.Ping(ctx).Err()
		lat := roundLatency(time.Since(t0))
		if err != nil {
			health.Redis = ServiceHealth{
				Status:    "down",
				LatencyMS: lat,
				Message:   err.Error(),
			}
		} else {
			status := "healthy"
			if lat > 100 {
				status = "degraded"
			}
			health.Redis = ServiceHealth{
				Status:    status,
				LatencyMS: lat,
			}
		}
	} else {
		health.Redis = ServiceHealth{
			Status:  "disabled",
			Message: "no redis configured",
		}
	}

	// Storage Check
	driverName := s.cfg.Storage.Driver
	if driverName == "" {
		driverName = "unknown"
	}
	if s.storage != nil {
		t0 := time.Now()
		err := s.storage.Ping(ctx)
		lat := roundLatency(time.Since(t0))
		if err != nil {
			health.Storage = StorageHealth{
				ServiceHealth: ServiceHealth{
					Status:    "down",
					LatencyMS: lat,
					Message:   err.Error(),
				},
				Driver: driverName,
			}
		} else {
			health.Storage = StorageHealth{
				ServiceHealth: ServiceHealth{
					Status:    "healthy",
					LatencyMS: lat,
				},
				Driver: driverName,
			}
		}
	} else {
		health.Storage = StorageHealth{
			ServiceHealth: ServiceHealth{
				Status:  "disabled",
				Message: "storage driver not initialized",
			},
			Driver: "none",
		}
	}

	// Meilisearch Check
	if s.searchClient != nil && s.searchClient.Enabled() {
		t0 := time.Now()
		err := s.searchClient.Ping(ctx)
		lat := roundLatency(time.Since(t0))
		if err != nil {
			health.Meilisearch = ServiceHealth{
				Status:    "down",
				LatencyMS: lat,
				Message:   err.Error(),
			}
		} else {
			health.Meilisearch = ServiceHealth{
				Status:    "healthy",
				LatencyMS: lat,
			}
		}
	} else {
		health.Meilisearch = ServiceHealth{
			Status:  "disabled",
			Message: "search engine not enabled",
		}
	}

	return health
}

func roundLatency(d time.Duration) float64 {
	ms := float64(d.Microseconds()) / 1000.0
	return math.Round(ms*100) / 100
}

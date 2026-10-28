package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/yin/alipay-nowpay-gateway/internal/alipay"
	"github.com/yin/alipay-nowpay-gateway/internal/commission"
	"github.com/yin/alipay-nowpay-gateway/internal/config"
	"github.com/yin/alipay-nowpay-gateway/internal/dbmigrate"
	"github.com/yin/alipay-nowpay-gateway/internal/httpserver"
	"github.com/yin/alipay-nowpay-gateway/internal/service"
	"github.com/yin/alipay-nowpay-gateway/internal/store"
	"golang.org/x/crypto/bcrypt"
)

func main() {
	cfg := config.Load()
	if len(cfg.AdminPassword) < 8 {
		slog.Error("ADMIN_PASSWORD 至少 8 位")
		os.Exit(1)
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		slog.Error("database", "err", err)
		os.Exit(1)
	}
	defer pool.Close()
	pingCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if err := waitDB(pingCtx, pool); err != nil {
		slog.Error("database ping", "err", err)
		os.Exit(1)
	}
	if err := dbmigrate.Up(ctx, pool); err != nil {
		slog.Error("migrate", "err", err)
		os.Exit(1)
	}
	st := store.New(pool)
	hash, err := bcrypt.GenerateFromPassword([]byte(cfg.AdminPassword), bcrypt.DefaultCost)
	if err != nil {
		slog.Error("admin hash", "err", err)
		os.Exit(1)
	}
	if err := st.Seed(ctx, cfg.AdminEmail, cfg.AdminName, string(hash)); err != nil {
		slog.Error("seed", "err", err)
		os.Exit(1)
	}

	var client alipay.Client
	var sim alipay.Simulator
	switch cfg.AlipayMode {
	case "mock", "":
		m := alipay.NewMock(alipay.NewPGStore(pool), alipay.MockOptions{PublicBaseURL: cfg.PublicBaseURL})
		client = m
		sim = m
	case "live":
		client = alipay.NewLive(cfg.AlipayAppID, cfg.AlipayPrivateKey, cfg.AlipayPublicKey, cfg.AlipayGatewayURL, cfg.AlipayAppAuth)
		slog.Warn("ALIPAY_MODE=live 不会发起网络请求", "err", alipay.ErrNotReady.Error())
	default:
		slog.Error("ALIPAY_MODE 只能是 mock 或 live")
		os.Exit(1)
	}
	engine, err := commission.New(cfg.AutoCommission)
	if err != nil {
		slog.Error("commission", "err", err)
		os.Exit(1)
	}
	svc := service.New(st, client, sim, engine, cfg.WebhookSecret, cfg.SessionTTL)
	api := httpserver.New(svc, cfg.AlipayMode, cfg.AllowedOrigins, cfg.CookieSecure, cfg.SessionTTL)
	srv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           api.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	go func() {
		<-ctx.Done()
		shut, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()
		_ = srv.Shutdown(shut)
	}()
	slog.Info("api listening", "addr", cfg.HTTPAddr, "alipay_mode", cfg.AlipayMode, "engine", engine.Name())
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		slog.Error("server", "err", err)
		os.Exit(1)
	}
}

func waitDB(ctx context.Context, pool *pgxpool.Pool) error {
	var err error
	for {
		err = pool.Ping(ctx)
		if err == nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return err
		case <-time.After(time.Second):
		}
	}
}

// Copyright (C) 2026 WuuBoLin
// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"labbit/internal/server"
)

const version = "dev"

type cliOptions struct {
	Bind        string
	Port        int
	DB          string
	PublicURL   string
	LogFormat   string
	LogLevel    string
	DisableAuth bool
	Help        bool
	Version     bool
}

func gracefulShutdown(apiServer *http.Server, done chan bool) {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	<-ctx.Done()

	slog.Info("shutdown signal received")
	stop()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := apiServer.Shutdown(ctx); err != nil {
		slog.Error("server shutdown failed", "error", err)
	}

	slog.Info("server stopped")
	done <- true
}

func main() {
	options, err := parseCLI(os.Args[1:], os.Stdout)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	if options.Help {
		return
	}
	if options.Version {
		fmt.Printf("labbit %s\n", version)
		return
	}

	if err := configureLogger(options.LogFormat, options.LogLevel); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}

	appServer := server.NewServer(server.Config{
		Bind:        options.Bind,
		Port:        options.Port,
		DB:          options.DB,
		PublicURL:   options.PublicURL,
		DisableAuth: options.DisableAuth,
	})

	done := make(chan bool, 1)

	go gracefulShutdown(appServer, done)

	slog.Info("server listening", "addr", appServer.Addr)
	err = appServer.ListenAndServe()
	if err != nil && err != http.ErrServerClosed {
		slog.Error("http server failed", "error", err)
		panic(fmt.Sprintf("http server error: %s", err))
	}

	<-done
	slog.Info("graceful shutdown complete")
}

func parseCLI(args []string, output io.Writer) (cliOptions, error) {
	options := cliOptions{
		Bind:      envString("BIND", "0.0.0.0"),
		Port:      envInt("PORT", 80),
		DB:        envString("DB_URL", "./db/labbit.db"),
		PublicURL: envString("PUBLIC_URL", ""),
		LogFormat: defaultLogFormat(),
		LogLevel:  envString("LOG_LEVEL", "info"),
	}
	flags := flag.NewFlagSet("labbit", flag.ContinueOnError)
	flags.SetOutput(output)
	flags.BoolVar(&options.Help, "h", false, "show help")
	flags.BoolVar(&options.Help, "help", false, "show help")
	flags.BoolVar(&options.Version, "version", false, "show version")
	flags.StringVar(&options.Bind, "b", options.Bind, "address to bind")
	flags.StringVar(&options.Bind, "bind", options.Bind, "address to bind")
	flags.IntVar(&options.Port, "p", options.Port, "port to listen on")
	flags.IntVar(&options.Port, "port", options.Port, "port to listen on")
	flags.StringVar(&options.DB, "d", options.DB, "sqlite database path or DSN")
	flags.StringVar(&options.DB, "db", options.DB, "sqlite database path or DSN")
	flags.StringVar(&options.PublicURL, "public-url", options.PublicURL, "public base URL for identity callbacks and passkeys")
	flags.BoolVar(&options.DisableAuth, "disable-auth", false, "disable passkeys, OIDC, sessions, onboarding, and auth-only routes")
	flags.StringVar(&options.LogFormat, "log-format", options.LogFormat, "log format: text or json")
	flags.StringVar(&options.LogLevel, "log-level", options.LogLevel, "log level: debug, info, warn, or error")
	flags.Usage = func() {
		fmt.Fprintf(output, "Labbit - lab contest exam question viewer\n\n")
		fmt.Fprintf(output, "Usage:\n  labbit [flags]\n\n")
		fmt.Fprintf(output, "Flags:\n")
		flags.PrintDefaults()
		fmt.Fprintf(output, "\nDefaults come from .env/env vars when present. Built-in default address is 0.0.0.0:80.\n")
	}
	if err := flags.Parse(args); err != nil {
		return options, err
	}
	if options.Help {
		flags.Usage()
	}
	if options.Port < 1 || options.Port > 65535 {
		return options, fmt.Errorf("port must be between 1 and 65535")
	}
	options.LogFormat = strings.ToLower(strings.TrimSpace(options.LogFormat))
	options.LogLevel = strings.ToLower(strings.TrimSpace(options.LogLevel))
	return options, nil
}

func configureLogger(format, levelName string) error {
	var level slog.Level
	switch strings.ToLower(levelName) {
	case "debug":
		level = slog.LevelDebug
	case "info", "":
		level = slog.LevelInfo
	case "warn", "warning":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		return fmt.Errorf("unsupported log level %q", levelName)
	}
	opts := &slog.HandlerOptions{Level: level}
	switch strings.ToLower(format) {
	case "text", "":
		slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, opts)))
	case "json":
		slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, opts)))
	default:
		return fmt.Errorf("unsupported log format %q", format)
	}
	return nil
}

func envString(name, fallback string) string {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	return value
}

func envInt(name string, fallback int) int {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func defaultLogFormat() string {
	if os.Getenv("APP_ENV") == "local" {
		return "text"
	}
	return "json"
}

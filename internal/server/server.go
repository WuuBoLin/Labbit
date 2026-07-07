// Copyright (C) 2026 WuuBoLin
// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/go-webauthn/webauthn/webauthn"
	_ "github.com/joho/godotenv/autoload"

	"labbit/internal/labbit"
)

type Server struct {
	bind string
	port int

	labs          *labbit.Store
	id            idConfig
	publicURL     string
	webauthn      *webauthn.WebAuthn
	oidcProviders map[string]*oidcProvider
	disableAuth   bool
	localUser     *labbit.User
}

type Config struct {
	Bind        string
	Port        int
	DB          string
	PublicURL   string
	DisableAuth bool
}

func NewServer(config Config) *http.Server {
	if config.Bind == "" {
		config.Bind = "0.0.0.0"
	}
	if config.Port == 0 {
		config.Port = 80
	}
	if config.DB == "" {
		config.DB = os.Getenv("DB_URL")
	}
	if config.DB == "" {
		config.DB = "./db/labbit.db"
	}
	if config.PublicURL == "" {
		config.PublicURL = os.Getenv("PUBLIC_URL")
	}
	publicURL := strings.TrimRight(strings.TrimSpace(config.PublicURL), "/")
	if publicURL == "" {
		publicURL = localPublicURL(config.Port)
	}
	if _, _, _, err := originHost(publicURL); err != nil {
		panic(fmt.Sprintf("public URL config: %v", err))
	}
	store, err := labbit.NewStore(config.DB)
	if err != nil {
		panic(fmt.Sprintf("labbit store: %v", err))
	}
	_ = os.Setenv("DB_URL", config.DB)
	slog.Info("labbit store initialized", "dsn", config.DB)
	NewServer := &Server{
		bind:        config.Bind,
		port:        config.Port,
		labs:        store,
		publicURL:   publicURL,
		disableAuth: config.DisableAuth,
	}
	if config.DisableAuth {
		NewServer.ensureAuthDisabledUser()
		slog.Warn("auth disabled; passkeys, OIDC, sessions, onboarding, and auth-only routes are unavailable")
	} else {
		id, err := newIDConfig(publicURL, config.Port)
		if err != nil {
			panic(fmt.Sprintf("id config: %v", err))
		}
		webAuthn, err := webauthn.New(&webauthn.Config{
			RPID:          id.rpID,
			RPDisplayName: "Labbit",
			RPOrigins:     []string{id.origin},
		})
		if err != nil {
			panic(fmt.Sprintf("webauthn config: %v", err))
		}
		NewServer.id = id
		NewServer.webauthn = webAuthn
		NewServer.oidcProviders = loadOIDCProviders(id.publicURL)
	}

	server := &http.Server{
		Addr:         fmt.Sprintf("%s:%d", NewServer.bind, NewServer.port),
		Handler:      NewServer.RegisterRoutes(),
		IdleTimeout:  time.Minute,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
	}
	slog.Info("http server configured", "addr", server.Addr)

	return server
}

func (s *Server) ensureAuthDisabledUser() {
	if s.labs == nil || s.localUser != nil {
		return
	}
	user, err := s.labs.EnsureLocalUser(context.Background())
	if err != nil {
		panic(fmt.Sprintf("local user: %v", err))
	}
	s.localUser = user
}

func originHost(rawURL string) (origin string, host string, secure bool, err error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", "", false, err
	}
	if u.Scheme == "" || u.Host == "" {
		return "", "", false, fmt.Errorf("PUBLIC_URL must include scheme and host")
	}
	return u.Scheme + "://" + u.Host, u.Hostname(), u.Scheme == "https", nil
}

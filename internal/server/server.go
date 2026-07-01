// Copyright (C) 2026 WuuBoLin
// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	_ "github.com/joho/godotenv/autoload"

	"labbit/internal/labbit"
)

type Server struct {
	bind string
	port int

	labs *labbit.Store
}

type Config struct {
	Bind string
	Port int
	DB   string
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
	store, err := labbit.NewStore(config.DB)
	if err != nil {
		panic(fmt.Sprintf("labbit store: %v", err))
	}
	_ = os.Setenv("DB_URL", config.DB)
	slog.Info("labbit store initialized", "dsn", config.DB)
	NewServer := &Server{
		bind: config.Bind,
		port: config.Port,
		labs: store,
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

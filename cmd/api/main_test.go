// Copyright (C) 2026 WuuBoLin
// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"io"
	"testing"
)

func TestParseCLIShortFlags(t *testing.T) {
	options, err := parseCLI([]string{"-b", "127.0.0.1", "-p", "18080", "-d", "/tmp/labbit.db", "--log-format", "json", "--log-level", "debug"}, io.Discard)
	if err != nil {
		t.Fatalf("parseCLI() error = %v", err)
	}
	if options.Bind != "127.0.0.1" || options.Port != 18080 || options.DB != "/tmp/labbit.db" {
		t.Fatalf("options = %#v", options)
	}
	if options.LogFormat != "json" || options.LogLevel != "debug" {
		t.Fatalf("logging options = %#v", options)
	}
}

func TestParseCLILongFlags(t *testing.T) {
	options, err := parseCLI([]string{"--bind", "0.0.0.0", "--port", "80", "--db", "./db/labbit.db", "--public-url", "https://labbit.example", "--disable-auth"}, io.Discard)
	if err != nil {
		t.Fatalf("parseCLI() error = %v", err)
	}
	if options.Bind != "0.0.0.0" || options.Port != 80 || options.DB != "./db/labbit.db" || options.PublicURL != "https://labbit.example" || !options.DisableAuth {
		t.Fatalf("options = %#v", options)
	}
}

func TestParseCLIHelp(t *testing.T) {
	options, err := parseCLI([]string{"--help"}, io.Discard)
	if err != nil {
		t.Fatalf("parseCLI() error = %v", err)
	}
	if !options.Help {
		t.Fatal("expected help flag")
	}
}

func TestParseCLIRejectsInvalidPort(t *testing.T) {
	_, err := parseCLI([]string{"--port", "70000"}, io.Discard)
	if err == nil {
		t.Fatal("expected invalid port error")
	}
}

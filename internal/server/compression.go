// Copyright (C) 2026 WuuBoLin
// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"bufio"
	"bytes"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"

	"github.com/klauspost/compress/zstd"
	"github.com/labstack/echo/v4"
)

const (
	zstdEncoding             = "zstd"
	zstdCompressionMinLength = 512
)

func zstdCompression() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			req := c.Request()
			if shouldSkipZstdCompression(req) {
				return next(c)
			}

			res := c.Response()
			addVary(res.Header(), echo.HeaderAcceptEncoding)
			if !acceptsZstd(req.Header.Get(echo.HeaderAcceptEncoding)) {
				return next(c)
			}

			original := res.Writer
			writer := &zstdResponseWriter{
				ResponseWriter: original,
				buffer:         &bytes.Buffer{},
				minLength:      zstdCompressionMinLength,
			}
			res.Writer = writer
			defer func() {
				writer.close()
				res.Writer = original
			}()

			return next(c)
		}
	}
}

type zstdResponseWriter struct {
	http.ResponseWriter
	buffer    *bytes.Buffer
	encoder   *zstd.Encoder
	minLength int

	code        int
	wroteBody   bool
	wroteHead   bool
	compressed  bool
	passThrough bool
}

func (w *zstdResponseWriter) WriteHeader(code int) {
	w.wroteHead = true
	w.code = code
}

func (w *zstdResponseWriter) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	w.wroteBody = true
	if w.Header().Get(echo.HeaderContentType) == "" {
		w.Header().Set(echo.HeaderContentType, http.DetectContentType(p))
	}
	if w.passThrough {
		w.writeDelayedHeader()
		return w.ResponseWriter.Write(p)
	}
	if w.compressed {
		_, err := w.encoder.Write(p)
		return len(p), err
	}

	n, err := w.buffer.Write(p)
	if err != nil {
		return n, err
	}
	if w.buffer.Len() < w.minLength {
		return len(p), nil
	}
	if !w.canCompress() {
		w.passThrough = true
		w.writeDelayedHeader()
		if _, err := w.buffer.WriteTo(w.ResponseWriter); err != nil {
			return n, err
		}
		return len(p), nil
	}
	if err := w.startCompression(); err != nil {
		return n, err
	}
	return len(p), nil
}

func (w *zstdResponseWriter) Flush() {
	if w.passThrough {
		w.writeDelayedHeader()
		_ = http.NewResponseController(w.ResponseWriter).Flush()
		return
	}
	if !w.compressed && w.buffer.Len() > 0 {
		if !w.canCompress() {
			w.passThrough = true
			w.writeDelayedHeader()
			_, _ = w.buffer.WriteTo(w.ResponseWriter)
			_ = http.NewResponseController(w.ResponseWriter).Flush()
			return
		}
		_ = w.startCompression()
	}
	if w.compressed {
		_ = w.encoder.Flush()
	}
	_ = http.NewResponseController(w.ResponseWriter).Flush()
}

func (w *zstdResponseWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}

func (w *zstdResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return http.NewResponseController(w.ResponseWriter).Hijack()
}

func (w *zstdResponseWriter) Push(target string, opts *http.PushOptions) error {
	if pusher, ok := w.ResponseWriter.(http.Pusher); ok {
		return pusher.Push(target, opts)
	}
	return http.ErrNotSupported
}

func (w *zstdResponseWriter) close() {
	if !w.wroteBody {
		if w.wroteHead {
			w.ResponseWriter.WriteHeader(w.code)
		}
		return
	}
	if w.compressed {
		_ = w.encoder.Close()
		return
	}
	w.writeDelayedHeader()
	_, _ = w.buffer.WriteTo(w.ResponseWriter)
}

func (w *zstdResponseWriter) startCompression() error {
	w.Header().Set(echo.HeaderContentEncoding, zstdEncoding)
	w.Header().Del(echo.HeaderContentLength)
	w.writeDelayedHeader()

	encoder, err := zstd.NewWriter(w.ResponseWriter, zstd.WithEncoderLevel(zstd.SpeedFastest), zstd.WithEncoderConcurrency(1))
	if err != nil {
		w.Header().Del(echo.HeaderContentEncoding)
		return err
	}
	w.encoder = encoder
	w.compressed = true
	if w.buffer.Len() == 0 {
		return nil
	}
	_, err = io.Copy(encoder, w.buffer)
	return err
}

func (w *zstdResponseWriter) canCompress() bool {
	if w.Header().Get(echo.HeaderContentEncoding) != "" {
		return false
	}
	if strings.Contains(strings.ToLower(w.Header().Get(echo.HeaderCacheControl)), "no-transform") {
		return false
	}
	if bodylessStatus(w.statusCode()) {
		return false
	}
	return compressibleContentType(w.Header().Get(echo.HeaderContentType))
}

func (w *zstdResponseWriter) statusCode() int {
	if w.wroteHead {
		return w.code
	}
	return http.StatusOK
}

func (w *zstdResponseWriter) writeDelayedHeader() {
	if w.wroteHead {
		w.ResponseWriter.WriteHeader(w.code)
		w.wroteHead = false
	}
}

func shouldSkipZstdCompression(r *http.Request) bool {
	if r.Method == http.MethodHead || r.Header.Get("Range") != "" {
		return true
	}
	if r.URL != nil && r.URL.Path == "/_/websocket" {
		return true
	}
	if strings.EqualFold(r.Header.Get(echo.HeaderUpgrade), "websocket") {
		return true
	}
	for _, value := range r.Header.Values(echo.HeaderConnection) {
		for _, part := range strings.Split(value, ",") {
			if strings.EqualFold(strings.TrimSpace(part), "upgrade") {
				return true
			}
		}
	}
	return false
}

func acceptsZstd(header string) bool {
	for _, value := range strings.Split(header, ",") {
		parts := strings.Split(value, ";")
		if len(parts) == 0 || !strings.EqualFold(strings.TrimSpace(parts[0]), zstdEncoding) {
			continue
		}
		q := 1.0
		for _, param := range parts[1:] {
			key, rawValue, ok := strings.Cut(strings.TrimSpace(param), "=")
			if !ok || !strings.EqualFold(strings.TrimSpace(key), "q") {
				continue
			}
			parsed, err := strconv.ParseFloat(strings.TrimSpace(rawValue), 64)
			if err == nil {
				q = parsed
			}
		}
		return q > 0
	}
	return false
}

func addVary(header http.Header, value string) {
	for _, existing := range header.Values(echo.HeaderVary) {
		for _, part := range strings.Split(existing, ",") {
			if strings.EqualFold(strings.TrimSpace(part), value) {
				return
			}
		}
	}
	header.Add(echo.HeaderVary, value)
}

func bodylessStatus(code int) bool {
	return code == http.StatusNoContent || code == http.StatusNotModified || (code >= 100 && code < 200)
}

func compressibleContentType(contentType string) bool {
	contentType = strings.ToLower(strings.TrimSpace(strings.Split(contentType, ";")[0]))
	if contentType == "" {
		return false
	}
	if strings.HasPrefix(contentType, "text/") {
		return true
	}
	switch contentType {
	case echo.MIMEApplicationJSON,
		echo.MIMEApplicationJavaScript,
		"application/ld+json",
		"application/manifest+json",
		"application/markdown",
		"application/svg+xml",
		"application/xhtml+xml",
		"application/xml":
		return true
	}
	return strings.HasSuffix(contentType, "+json") || strings.HasSuffix(contentType, "+xml")
}

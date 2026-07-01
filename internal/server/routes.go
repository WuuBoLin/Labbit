// Copyright (C) 2026 WuuBoLin
// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/a-h/templ"
	"github.com/coder/websocket"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"labbit/cmd/web"
	"labbit/internal/labbit"
)

func (s *Server) RegisterRoutes() http.Handler {
	e := echo.New()
	e.Use(middleware.Recover())
	e.Use(requestLogger)

	e.Use(middleware.CORSWithConfig(middleware.CORSConfig{
		AllowOrigins:     []string{"https://*", "http://*"},
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS", "PATCH"},
		AllowHeaders:     []string{"Accept", "Authorization", "Content-Type", "X-CSRF-Token"},
		AllowCredentials: true,
		MaxAge:           300,
	}))

	fileServer := staticCache(http.FileServer(http.FS(web.Files)))
	e.GET("/assets/*", echo.WrapHandler(fileServer))

	e.GET("/", s.uploadPageHandler)
	e.GET("/favicon.ico", func(c echo.Context) error {
		return c.Redirect(http.StatusMovedPermanently, "/assets/img/favicon.ico")
	})
	e.GET("/apple-touch-icon.png", func(c echo.Context) error {
		return c.Redirect(http.StatusMovedPermanently, "/assets/img/icon-180.png")
	})
	e.POST("/upload", s.uploadHandler)
	e.GET("/docs/:uid", s.docUIDRedirectHandler)
	e.GET("/docs/:uid/:slug", s.viewerHandler)
	e.GET("/docs/:uid/:slug/section/:section", s.sectionHandler)
	e.GET("/docs/:uid/:slug/hints/:task/:hint", s.inlineHintHandler)
	e.GET("/docs/:uid/:slug/hints/:task", s.hintHandler)
	e.GET("/docs/:uid/:slug/answers/:task", s.hintHandler)
	e.POST("/docs/:uid/:slug/quiz/:question/check", s.quizCheckHandler)
	e.GET("/docs/:uid/:slug/search", s.searchHandler)

	e.GET("/health", s.healthHandler)

	e.GET("/websocket", s.websocketHandler)

	return e
}

func (s *Server) uploadPageHandler(c echo.Context) error {
	return render(c, http.StatusOK, web.UploadPage(""))
}

func (s *Server) uploadHandler(c echo.Context) error {
	file, err := c.FormFile("labfile")
	if err != nil {
		slog.Warn("upload missing lab file", "remote_addr", c.RealIP())
		return s.renderUploadError(c, http.StatusBadRequest, "Choose a Labbit file to upload.")
	}
	src, err := file.Open()
	if err != nil {
		slog.Warn("upload file open failed", "filename", file.Filename, "error", err)
		return s.renderUploadError(c, http.StatusBadRequest, "The uploaded file could not be opened.")
	}
	defer src.Close()

	body, err := io.ReadAll(io.LimitReader(src, 2<<20+1))
	if err != nil {
		slog.Warn("upload read failed", "filename", file.Filename, "error", err)
		return s.renderUploadError(c, http.StatusBadRequest, "The uploaded file could not be read.")
	}
	if len(body) > 2<<20 {
		slog.Warn("upload too large", "filename", file.Filename, "size", len(body))
		return s.renderUploadError(c, http.StatusBadRequest, "The uploaded file is too large. Keep it under 2 MiB.")
	}
	hash := fileHash(body)
	if existing, err := s.labs.GetDocumentByHash(c.Request().Context(), hash); err == nil {
		slog.Info("duplicate lab upload reused", "uid", existing.UID, "slug", existing.Slug, "filename", file.Filename)
		c.Response().Header().Set("HX-Push-Url", fmt.Sprintf("/docs/%s/%s", existing.UID, existing.Slug))
		return render(c, http.StatusOK, web.ViewerPage(existing))
	}

	doc, err := labbit.Parse(bytes.NewReader(body))
	if err != nil {
		slog.Warn("lab parse failed", "filename", file.Filename, "error", err)
		return s.renderUploadError(c, http.StatusBadRequest, err.Error())
	}
	doc.Hash = hash
	doc.UID = shortHashUID(hash)
	if err := s.labs.SaveDocument(c.Request().Context(), doc); err != nil {
		slog.Error("lab save failed", "uid", doc.UID, "slug", doc.Slug, "error", err)
		if existing, lookupErr := s.labs.GetDocumentByHash(c.Request().Context(), hash); lookupErr == nil {
			c.Response().Header().Set("HX-Push-Url", fmt.Sprintf("/docs/%s/%s", existing.UID, existing.Slug))
			return render(c, http.StatusOK, web.ViewerPage(existing))
		}
		return s.renderUploadError(c, http.StatusInternalServerError, "The lab could not be saved.")
	}
	slog.Info("lab uploaded", "uid", doc.UID, "slug", doc.Slug, "title", doc.Title, "filename", file.Filename)
	c.Response().Header().Set("HX-Push-Url", fmt.Sprintf("/docs/%s/%s", doc.UID, doc.Slug))
	return render(c, http.StatusOK, web.ViewerPage(doc))
}

func (s *Server) renderUploadError(c echo.Context, status int, message string) error {
	if c.Request().Header.Get("HX-Request") == "true" {
		c.Response().Header().Set("HX-Retarget", "#upload-error")
		c.Response().Header().Set("HX-Reswap", "innerHTML")
		return render(c, http.StatusOK, web.UploadError(message))
	}
	return render(c, status, web.UploadPage(message))
}

func fileHash(body []byte) string {
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}

func shortHashUID(hash string) string {
	if len(hash) < 7 {
		return hash
	}
	return hash[:7]
}

func (s *Server) docUIDRedirectHandler(c echo.Context) error {
	doc, err := s.labs.GetDocumentByUID(c.Request().Context(), c.Param("uid"))
	if err != nil {
		slog.Warn("lab not found for uid redirect", "uid", c.Param("uid"))
		return echo.NewHTTPError(http.StatusNotFound, "lab not found")
	}
	return c.Redirect(http.StatusMovedPermanently, fmt.Sprintf("/docs/%s/%s", doc.UID, doc.Slug))
}

func (s *Server) viewerHandler(c echo.Context) error {
	doc, err := s.loadDocument(c)
	if err != nil {
		return err
	}
	return render(c, http.StatusOK, web.ViewerPage(doc))
}

func (s *Server) sectionHandler(c echo.Context) error {
	doc, err := s.loadDocument(c)
	if err != nil {
		return err
	}
	section := c.Param("section")
	if c.Request().Header.Get("HX-Request") != "true" {
		if !sectionExists(doc, section) {
			return echo.NewHTTPError(http.StatusNotFound, "section not found")
		}
		return render(c, http.StatusOK, web.ViewerSectionPage(doc, section, c.QueryParam("block")))
	}
	return renderSection(c, doc, section, c.QueryParam("block"))
}

func renderSection(c echo.Context, doc *labbit.Document, section, selectedBlock string) error {
	if section == "overview" {
		return render(c, http.StatusOK, web.OverviewSection(doc))
	}
	for _, topic := range doc.Topics {
		if topic.ID == section {
			return render(c, http.StatusOK, web.LabTopicSection(doc, topic, selectedBlock))
		}
	}
	for _, topic := range quizTopics(doc.Questions) {
		if topic.ID == section {
			return render(c, http.StatusOK, web.QuizTopicSection(doc, topic, selectedBlock))
		}
	}
	return echo.NewHTTPError(http.StatusNotFound, "section not found")
}

func (s *Server) hintHandler(c echo.Context) error {
	doc, err := s.loadDocument(c)
	if err != nil {
		return err
	}
	task, err := s.labs.GetSolution(c.Request().Context(), doc.ID, c.Param("task"))
	if err != nil {
		slog.Warn("solution not found", "uid", doc.UID, "task", c.Param("task"))
		return echo.NewHTTPError(http.StatusNotFound, "solution not found")
	}
	slog.Info("solution served", "uid", doc.UID, "task", task.ID, "count", len(task.Hints))
	return render(c, http.StatusOK, web.SolutionFragment(task))
}

func (s *Server) inlineHintHandler(c echo.Context) error {
	doc, err := s.loadDocument(c)
	if err != nil {
		return err
	}
	hint, err := s.labs.GetHint(c.Request().Context(), doc.ID, c.Param("task"), c.Param("hint"))
	if err != nil {
		slog.Warn("inline answer not found", "uid", doc.UID, "task", c.Param("task"), "hint", c.Param("hint"))
		return echo.NewHTTPError(http.StatusNotFound, "inline answer not found")
	}
	slog.Info("inline answer served", "uid", doc.UID, "task", c.Param("task"), "hint", hint.ID)
	return render(c, http.StatusOK, web.InlineHintFragment(hint))
}

func (s *Server) quizCheckHandler(c echo.Context) error {
	doc, err := s.loadDocument(c)
	if err != nil {
		return err
	}
	if err := c.Request().ParseForm(); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid quiz submission")
	}
	question, err := s.labs.GetQuestion(c.Request().Context(), doc.ID, c.Param("question"))
	if err != nil {
		slog.Warn("question not found", "uid", doc.UID, "question", c.Param("question"))
		return echo.NewHTTPError(http.StatusNotFound, "question not found")
	}
	selected := c.Request().Form["option"]
	correct, correctIDs := labbit.CheckQuestion(question, selected)
	question.Options = reorderOptions(question.Options, c.Request().Form["_order"])
	section := strings.TrimSpace(c.FormValue("_section"))
	if section == "" {
		section = question.TopicID
	}
	number := strings.TrimSpace(c.FormValue("_number"))
	if number == "" {
		number = "01"
	}
	slog.Info("quiz checked", "uid", doc.UID, "question", question.ID, "correct", correct)
	return render(c, http.StatusOK, web.QuizCard(doc, section, question, number, question.ID, selected, correctIDs, correct, true))
}

func (s *Server) searchHandler(c echo.Context) error {
	doc, err := s.loadDocument(c)
	if err != nil {
		return err
	}
	results, err := s.labs.Search(c.Request().Context(), doc.ID, c.QueryParam("q"))
	if err != nil {
		slog.Error("search failed", "uid", doc.UID, "query", c.QueryParam("q"), "error", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "search failed")
	}
	slog.Info("search completed", "uid", doc.UID, "query", c.QueryParam("q"), "results", len(results))
	return render(c, http.StatusOK, web.SearchResultsFragment(doc, results))
}

func (s *Server) loadDocument(c echo.Context) (*labbit.Document, error) {
	doc, err := s.labs.GetDocument(c.Request().Context(), c.Param("uid"), c.Param("slug"))
	if err != nil {
		slog.Warn("lab not found", "uid", c.Param("uid"), "slug", c.Param("slug"))
		return nil, echo.NewHTTPError(http.StatusNotFound, "lab not found")
	}
	return doc, nil
}

func render(c echo.Context, status int, component templ.Component) error {
	c.Response().WriteHeader(status)
	return component.Render(c.Request().Context(), c.Response().Writer)
}

func staticCache(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/assets/fonts/") || r.URL.Path == "/assets/js/htmx.min.js" {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			w.Header().Set("Cache-Control", "public, max-age=300, stale-while-revalidate=86400")
		}
		next.ServeHTTP(w, r)
	})
}

func quizTopics(questions []labbit.Question) []labbit.Topic {
	seen := map[string]bool{}
	var topics []labbit.Topic
	for _, q := range questions {
		if seen[q.TopicID] {
			continue
		}
		seen[q.TopicID] = true
		topics = append(topics, labbit.Topic{ID: q.TopicID, Kind: "quiz", Title: q.TopicTitle})
	}
	return topics
}

func sectionExists(doc *labbit.Document, section string) bool {
	if section == "overview" {
		return true
	}
	for _, topic := range doc.Topics {
		if topic.ID == section {
			return true
		}
	}
	for _, topic := range quizTopics(doc.Questions) {
		if topic.ID == section {
			return true
		}
	}
	return false
}

func reorderOptions(options []labbit.Option, order []string) []labbit.Option {
	if len(order) == 0 {
		return options
	}
	byID := map[string]labbit.Option{}
	for _, option := range options {
		byID[option.ID] = option
	}
	ordered := make([]labbit.Option, 0, len(options))
	seen := map[string]bool{}
	for _, id := range order {
		if option, ok := byID[id]; ok && !seen[id] {
			ordered = append(ordered, option)
			seen[id] = true
		}
	}
	for _, option := range options {
		if !seen[option.ID] {
			ordered = append(ordered, option)
		}
	}
	return ordered
}

func requestLogger(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		start := time.Now()
		err := next(c)
		if err != nil {
			c.Error(err)
		}
		req := c.Request()
		res := c.Response()
		slog.Info("http request",
			"method", req.Method,
			"path", req.URL.Path,
			"status", res.Status,
			"duration_ms", time.Since(start).Milliseconds(),
			"remote_addr", c.RealIP(),
		)
		return nil
	}
}

func (s *Server) healthHandler(c echo.Context) error {
	ctx, cancel := context.WithTimeout(c.Request().Context(), time.Second)
	defer cancel()
	if err := s.labs.Ping(ctx); err != nil {
		return c.JSON(http.StatusServiceUnavailable, map[string]string{
			"status": "down",
			"error":  err.Error(),
		})
	}
	return c.JSON(http.StatusOK, map[string]string{"status": "up"})
}

func (s *Server) websocketHandler(c echo.Context) error {
	w := c.Response().Writer
	r := c.Request()
	socket, err := websocket.Accept(w, r, nil)

	if err != nil {
		slog.Warn("websocket accept failed", "error", err)
		_, _ = w.Write([]byte("could not open websocket"))
		w.WriteHeader(http.StatusInternalServerError)
		return nil
	}

	defer socket.Close(websocket.StatusGoingAway, "server closing websocket")

	ctx := r.Context()
	socketCtx := socket.CloseRead(ctx)

	for {
		payload := fmt.Sprintf("server timestamp: %d", time.Now().UnixNano())
		err := socket.Write(socketCtx, websocket.MessageText, []byte(payload))
		if err != nil {
			slog.Info("websocket closed", "error", err)
			break
		}
		time.Sleep(time.Second * 2)
	}
	return nil
}

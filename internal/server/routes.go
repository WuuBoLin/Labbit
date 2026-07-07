// Copyright (C) 2026 WuuBoLin
// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/a-h/templ"
	"github.com/coder/websocket"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"labbit/cmd/web"
	"labbit/internal/labbit"
)

const docsPrefix = "/docs"
const docsPerPage = 20

func docRoute(parts ...string) string {
	return strings.Join(append([]string{docsPrefix}, parts...), "/")
}

func userDocRoute(parts ...string) string {
	return strings.Join(append([]string{"/@:username", "docs"}, parts...), "/")
}

func docPath(doc *labbit.Document) string {
	return fmt.Sprintf("/@%s/docs/%s/%s", doc.OwnerName, doc.UID, doc.Slug)
}

func (s *Server) RegisterRoutes() http.Handler {
	if s.disableAuth {
		s.ensureAuthDisabledUser()
	} else {
		s.ensureID()
	}
	e := echo.New()
	e.Use(middleware.Recover())
	e.Use(requestLogger)
	e.Use(zstdCompression())
	if s.disableAuth {
		e.Use(s.authDisabledMiddleware)
	} else {
		e.Use(s.idMiddleware)
	}

	cors := middleware.CORSConfig{
		AllowMethods: []string{"GET", "POST", "PUT", "DELETE", "OPTIONS", "PATCH"},
		AllowHeaders: []string{"Accept", "Authorization", "Content-Type", "X-CSRF-Token"},
		MaxAge:       300,
	}
	if s.disableAuth {
		cors.AllowOrigins = []string{"*"}
	} else {
		cors.AllowOrigins = []string{s.id.origin}
		cors.AllowCredentials = true
	}
	e.Use(middleware.CORSWithConfig(cors))

	fileServer := staticCache(http.FileServer(http.FS(web.Files)))
	e.GET("/assets/*", echo.WrapHandler(fileServer))

	e.GET("/", s.homeHandler)
	e.GET("/favicon.ico", func(c echo.Context) error {
		return c.Redirect(http.StatusMovedPermanently, "/assets/img/favicon.ico")
	})
	e.GET("/apple-touch-icon.png", func(c echo.Context) error {
		return c.Redirect(http.StatusMovedPermanently, "/assets/img/icon-180.png")
	})
	e.PATCH("/i/theme", s.themeHandler)
	e.POST("/i/upload", s.uploadHandler, s.requireActive)
	e.GET("/i/library", s.docsPageHandler, s.requireActive)
	e.DELETE("/i/library", s.docsBulkDeleteHandler, s.requireActive)
	e.PUT("/i/library/:uid/:slug/visibility", s.docsVisibilityHandler, s.requireActive)
	e.DELETE("/i/library/:uid/:slug", s.docsDeleteHandler, s.requireActive)
	if !s.disableAuth {
		e.GET("/i/onboarding", s.onboardingPageHandler, s.requireSession)
		e.POST("/i/onboarding", s.onboardingHandler, s.requireSession)
		e.GET("/id", s.idHandler)
		e.GET("/id/authenticate", s.authenticateHandler, nextHandler)
		e.POST("/id/authenticate", s.passkeyAuthenticateHandler, nextHandler)
		e.GET("/id/register", s.registerRedirectHandler)
		e.POST("/id/register", s.passkeyRegisterHandler, nextHandler)
		e.GET("/id/signout", s.signoutPageHandler, nextHandler)
		e.POST("/id/signout", s.signoutHandler, nextHandler)
		e.GET("/id/oidc/:provider/start", s.oidcStartHandler)
		e.GET("/id/oidc/:provider/callback", s.oidcCallbackHandler)
	}
	e.GET("/@:username", s.userPageHandler)
	e.GET(userDocRoute(":uid", ":slug"), s.viewerHandler)
	e.GET(userDocRoute(":uid", ":slug", "search"), s.searchHandler)
	e.GET(userDocRoute(":uid", ":slug", "keys", "labs", ":task", ":hint"), s.inlineHintHandler)
	e.GET(userDocRoute(":uid", ":slug", "keys", "labs", ":task"), s.solutionHandler)
	e.POST(userDocRoute(":uid", ":slug", "keys", "quiz", ":question", "check"), s.quizCheckHandler)
	e.GET(userDocRoute(":uid", ":slug", ":type", ":section"), s.sectionHandler)

	e.GET("/_/healthz", s.healthHandler)

	e.GET("/_/websocket", s.websocketHandler)

	return e
}

func (s *Server) homeHandler(c echo.Context) error {
	user := currentUser(c)
	if user != nil && user.Status != labbit.UserStatusActive {
		return s.redirectToOnboarding(c)
	}
	if user == nil {
		component := web.HomePage(nil, nil, "", requestTheme(c), c.QueryParam("next"), s.oidcButtons(), s.disableAuth)
		return render(c, http.StatusOK, s.withMeta(component, s.websiteMeta(c)))
	}
	page := 1
	recent, err := s.labs.GetRecentDocuments(c.Request().Context(), user.ID, page, 10)
	if err != nil {
		return err
	}
	component := web.HomePage(user, recent, "", requestTheme(c), "", s.oidcButtons(), s.disableAuth)
	return render(c, http.StatusOK, s.withMeta(component, s.websiteMeta(c)))
}

func (s *Server) docsPageHandler(c echo.Context) error {
	user := currentUser(c)
	page := parsePage(c.QueryParam("page"))
	query := requestLibraryQuery(c)
	docs, hasNext, err := s.loadDocsPage(c.Request().Context(), user.ID, page, query)
	if err != nil {
		return err
	}
	if page > 1 && len(docs) == 0 {
		page = 1
		docs, hasNext, err = s.loadDocsPage(c.Request().Context(), user.ID, page, query)
		if err != nil {
			return err
		}
		if !isHTMX(c) {
			return c.Redirect(http.StatusSeeOther, docsLibraryPath(page, query))
		}
		c.Response().Header().Set("HX-Push-Url", docsLibraryPath(page, query))
	}
	if isHTMX(c) {
		c.Response().Header().Set("HX-Push-Url", docsLibraryPath(page, query))
		return render(c, http.StatusOK, web.DocsListFragment(docs, page, hasNext, query, ""))
	}
	component := web.DocsPage(user, docs, page, hasNext, query, requestTheme(c), s.disableAuth)
	return render(c, http.StatusOK, s.withMeta(component, s.websiteMeta(c)))
}

func (s *Server) userPageHandler(c echo.Context) error {
	username := c.Param("username")
	profileUser, err := s.labs.GetUserByUsername(c.Request().Context(), username)
	if err != nil || profileUser == nil || profileUser.Status != labbit.UserStatusActive {
		return echo.NewHTTPError(http.StatusNotFound, "user not found")
	}
	visitor := currentUser(c)
	page := parsePage(c.QueryParam("page"))
	query := requestLibraryQuery(c)
	ownerView := visitor != nil && visitor.ID == profileUser.ID
	loadPage := s.loadPublicDocsPage
	if ownerView {
		loadPage = s.loadDocsPage
	}
	docs, hasNext, err := loadPage(c.Request().Context(), profileUser.ID, page, query)
	if err != nil {
		return err
	}
	if page > 1 && len(docs) == 0 {
		page = 1
		docs, hasNext, err = loadPage(c.Request().Context(), profileUser.ID, page, query)
		if err != nil {
			return err
		}
		if !isHTMX(c) {
			return c.Redirect(http.StatusSeeOther, userLibraryPath(profileUser.Username, page, query))
		}
		c.Response().Header().Set("HX-Push-Url", userLibraryPath(profileUser.Username, page, query))
	}
	if isHTMX(c) {
		c.Response().Header().Set("HX-Push-Url", userLibraryPath(profileUser.Username, page, query))
		return render(c, http.StatusOK, web.UserDocsListFragment(profileUser, docs, page, hasNext, query, ownerView))
	}
	component := web.UserPage(profileUser, visitor, docs, page, hasNext, query, requestTheme(c), s.disableAuth)
	return render(c, http.StatusOK, s.withMeta(component, s.userMeta(c, profileUser)))
}

func (s *Server) docsVisibilityHandler(c echo.Context) error {
	if err := s.labs.SetUserDocumentVisibility(c.Request().Context(), currentUser(c).ID, c.Param("uid"), c.Param("slug"), c.FormValue("visibility")); err != nil {
		if errors.Is(err, labbit.ErrNotFound) {
			return echo.NewHTTPError(http.StatusNotFound, "lab not found")
		}
		return err
	}
	return s.renderDocsListAfterAction(c, requestPage(c))
}

func (s *Server) docsDeleteHandler(c echo.Context) error {
	if err := s.labs.DeleteUserDocument(c.Request().Context(), currentUser(c).ID, c.Param("uid"), c.Param("slug")); err != nil {
		if errors.Is(err, labbit.ErrNotFound) {
			return echo.NewHTTPError(http.StatusNotFound, "lab not found")
		}
		return err
	}
	return s.renderDocsListAfterAction(c, requestPage(c))
}

func (s *Server) docsBulkDeleteHandler(c echo.Context) error {
	values, err := deleteFormValues(c)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid library action")
	}
	page := parsePage(values.Get("page"))
	query := strings.TrimSpace(values.Get("q"))
	selected := values["doc"]
	if len(selected) == 0 {
		return s.renderDocsListAfterActionWithNotice(c, page, query, "Select at least one doc to delete.")
	}
	if _, err := s.labs.DeleteUserDocuments(c.Request().Context(), currentUser(c).ID, selected); err != nil {
		return err
	}
	return s.renderDocsListAfterActionWithNotice(c, page, query, "")
}

func (s *Server) renderDocsListAfterAction(c echo.Context, page int) error {
	return s.renderDocsListAfterActionWithNotice(c, page, requestLibraryQuery(c), "")
}

func (s *Server) renderDocsListAfterActionWithNotice(c echo.Context, page int, query string, notice string) error {
	docs, hasNext, err := s.loadDocsPage(c.Request().Context(), currentUser(c).ID, page, query)
	if err != nil {
		return err
	}
	for page > 1 && len(docs) == 0 {
		page--
		docs, hasNext, err = s.loadDocsPage(c.Request().Context(), currentUser(c).ID, page, query)
		if err != nil {
			return err
		}
	}
	target := docsLibraryPath(page, query)
	if isHTMX(c) {
		c.Response().Header().Set("HX-Push-Url", target)
		return render(c, http.StatusOK, web.DocsListFragment(docs, page, hasNext, query, notice))
	}
	return c.Redirect(http.StatusSeeOther, target)
}

func (s *Server) loadDocsPage(ctx context.Context, userID string, page int, query string) ([]labbit.RecentDocument, bool, error) {
	docs, err := s.labs.ListUserDocumentsFiltered(ctx, userID, page, docsPerPage+1, query)
	if err != nil {
		return nil, false, err
	}
	hasNext := len(docs) > docsPerPage
	if hasNext {
		docs = docs[:docsPerPage]
	}
	return docs, hasNext, nil
}

func (s *Server) loadPublicDocsPage(ctx context.Context, userID string, page int, query string) ([]labbit.RecentDocument, bool, error) {
	docs, err := s.labs.ListPublicUserDocumentsFiltered(ctx, userID, page, docsPerPage+1, query)
	if err != nil {
		return nil, false, err
	}
	hasNext := len(docs) > docsPerPage
	if hasNext {
		docs = docs[:docsPerPage]
	}
	return docs, hasNext, nil
}

func requestLibraryQuery(c echo.Context) string {
	if query := strings.TrimSpace(c.FormValue("q")); query != "" {
		return query
	}
	return strings.TrimSpace(c.QueryParam("q"))
}

func requestPage(c echo.Context) int {
	if page := strings.TrimSpace(c.FormValue("page")); page != "" {
		return parsePage(page)
	}
	return parsePage(c.QueryParam("page"))
}

func parsePage(raw string) int {
	page, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || page < 1 {
		return 1
	}
	return page
}

func docsLibraryPath(page int, query string) string {
	values := url.Values{}
	if strings.TrimSpace(query) != "" {
		values.Set("q", strings.TrimSpace(query))
	}
	if page > 1 {
		values.Set("page", fmt.Sprintf("%d", page))
	}
	if len(values) == 0 {
		return "/i/library"
	}
	return "/i/library?" + values.Encode()
}

func userLibraryPath(username string, page int, query string) string {
	values := url.Values{}
	if strings.TrimSpace(query) != "" {
		values.Set("q", strings.TrimSpace(query))
	}
	if page > 1 {
		values.Set("page", fmt.Sprintf("%d", page))
	}
	base := "/@" + username
	if len(values) == 0 {
		return base
	}
	return base + "?" + values.Encode()
}

func isHTMX(c echo.Context) bool {
	return c.Request().Header.Get("HX-Request") == "true"
}

func deleteFormValues(c echo.Context) (url.Values, error) {
	values := url.Values{}
	for key, existing := range c.QueryParams() {
		values[key] = append(values[key], existing...)
	}
	if !strings.HasPrefix(c.Request().Header.Get("Content-Type"), "application/x-www-form-urlencoded") {
		return values, nil
	}
	body, err := io.ReadAll(c.Request().Body)
	if err != nil {
		return nil, err
	}
	parsed, err := url.ParseQuery(string(body))
	if err != nil {
		return nil, err
	}
	for key, existing := range parsed {
		values[key] = append(values[key], existing...)
	}
	return values, nil
}

func (s *Server) uploadHandler(c echo.Context) error {
	user := currentUser(c)
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
		if err := s.labs.SaveUserDocument(c.Request().Context(), user.ID, existing.ID, labbit.NormalizeVisibility(c.FormValue("visibility"))); err != nil {
			return err
		}
		doc, err := s.labs.GetUserDocument(c.Request().Context(), user.Username, existing.UID, existing.Slug)
		if err != nil {
			return err
		}
		slog.Info("duplicate lab upload reused", "uid", doc.UID, "slug", doc.Slug, "filename", file.Filename, "user", user.Username)
		c.Response().Header().Set("HX-Push-Url", docPath(doc))
		component := web.ViewerPage(doc, requestTheme(c), currentUser(c), s.disableAuth)
		return render(c, http.StatusOK, s.withMeta(component, s.documentMetaForPath(docPath(doc), doc, "labs", "overview")))
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
			if err := s.labs.SaveUserDocument(c.Request().Context(), user.ID, existing.ID, labbit.NormalizeVisibility(c.FormValue("visibility"))); err != nil {
				return err
			}
			doc, err := s.labs.GetUserDocument(c.Request().Context(), user.Username, existing.UID, existing.Slug)
			if err != nil {
				return err
			}
			c.Response().Header().Set("HX-Push-Url", docPath(doc))
			component := web.ViewerPage(doc, requestTheme(c), currentUser(c), s.disableAuth)
			return render(c, http.StatusOK, s.withMeta(component, s.documentMetaForPath(docPath(doc), doc, "labs", "overview")))
		}
		return s.renderUploadError(c, http.StatusInternalServerError, "The lab could not be saved.")
	}
	if err := s.labs.SaveUserDocument(c.Request().Context(), user.ID, doc.ID, labbit.NormalizeVisibility(c.FormValue("visibility"))); err != nil {
		return err
	}
	doc, err = s.labs.GetUserDocument(c.Request().Context(), user.Username, doc.UID, doc.Slug)
	if err != nil {
		return err
	}
	slog.Info("lab uploaded", "uid", doc.UID, "slug", doc.Slug, "title", doc.Title, "filename", file.Filename, "user", user.Username)
	c.Response().Header().Set("HX-Push-Url", docPath(doc))
	component := web.ViewerPage(doc, requestTheme(c), currentUser(c), s.disableAuth)
	return render(c, http.StatusOK, s.withMeta(component, s.documentMetaForPath(docPath(doc), doc, "labs", "overview")))
}

func (s *Server) renderUploadError(c echo.Context, status int, message string) error {
	if c.Request().Header.Get("HX-Request") == "true" {
		c.Response().Header().Set("HX-Retarget", "#upload-error")
		c.Response().Header().Set("HX-Reswap", "innerHTML")
		return render(c, http.StatusOK, web.UploadError(message))
	}
	user := currentUser(c)
	var recent []labbit.RecentDocument
	if user != nil && user.Status == labbit.UserStatusActive {
		recent, _ = s.labs.GetRecentDocuments(c.Request().Context(), user.ID, 1, 10)
	}
	component := web.HomePage(user, recent, message, requestTheme(c), "", s.oidcButtons(), s.disableAuth)
	return render(c, status, s.withMeta(component, s.websiteMeta(c)))
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
	return c.Redirect(http.StatusMovedPermanently, docPath(doc))
}

func (s *Server) viewerHandler(c echo.Context) error {
	doc, err := s.loadDocument(c)
	if err != nil {
		return err
	}
	if doc == nil {
		return nil
	}
	component := web.ViewerPage(doc, requestTheme(c), currentUser(c), s.disableAuth)
	return render(c, http.StatusOK, s.withMeta(component, s.documentMeta(c, doc, "labs", "overview")))
}

func (s *Server) sectionHandler(c echo.Context) error {
	doc, err := s.loadDocument(c)
	if err != nil {
		return err
	}
	if doc == nil {
		return nil
	}
	section := c.Param("section")
	sectionType := c.Param("type")
	if c.Request().Header.Get("HX-Request") != "true" {
		if !sectionExists(doc, sectionType, section) {
			return echo.NewHTTPError(http.StatusNotFound, "section not found")
		}
		component := web.ViewerSectionPage(doc, section, c.QueryParam("block"), requestTheme(c), currentUser(c), s.disableAuth)
		return render(c, http.StatusOK, s.withMeta(component, s.documentMeta(c, doc, sectionType, section)))
	}
	return renderSection(c, doc, sectionType, section, c.QueryParam("block"))
}

func (s *Server) themeHandler(c echo.Context) error {
	theme := normalizeTheme(c.FormValue("theme"))
	http.SetCookie(c.Response(), &http.Cookie{
		Name:     "labbit.theme",
		Value:    theme,
		Path:     "/",
		MaxAge:   60 * 60 * 24 * 365,
		SameSite: http.SameSiteLaxMode,
		HttpOnly: true,
	})
	c.Response().Header().Set("HX-Trigger", fmt.Sprintf(`{"labbitThemeChanged":{"theme":"%s"}}`, theme))
	return render(c, http.StatusOK, web.ThemeToggleResponse(theme, c.FormValue("slot")))
}

func renderSection(c echo.Context, doc *labbit.Document, sectionType, section, selectedBlock string) error {
	if sectionType == "labs" && section == "overview" {
		return render(c, http.StatusOK, web.SectionFragment(doc, "overview", selectedBlock))
	}
	switch sectionType {
	case "labs":
		for _, topic := range doc.Topics {
			if topic.ID == section {
				return render(c, http.StatusOK, web.SectionFragment(doc, section, selectedBlock))
			}
		}
	case "quiz":
		for _, topic := range quizTopics(doc.Questions) {
			if topic.ID == section {
				return render(c, http.StatusOK, web.SectionFragment(doc, section, selectedBlock))
			}
		}
	}
	return echo.NewHTTPError(http.StatusNotFound, "section not found")
}

func (s *Server) solutionHandler(c echo.Context) error {
	doc, err := s.loadDocument(c)
	if err != nil {
		return err
	}
	if doc == nil {
		return nil
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
	if doc == nil {
		return nil
	}
	hint, err := s.labs.GetHint(c.Request().Context(), doc.ID, c.Param("task"), c.Param("hint"))
	if err != nil {
		slog.Warn("inline hint not found", "uid", doc.UID, "task", c.Param("task"), "hint", c.Param("hint"))
		return echo.NewHTTPError(http.StatusNotFound, "hint not found")
	}
	slog.Info("inline hint served", "uid", doc.UID, "task", c.Param("task"), "hint", hint.ID)
	return render(c, http.StatusOK, web.InlineHintFragment(hint))
}

func (s *Server) quizCheckHandler(c echo.Context) error {
	doc, err := s.loadDocument(c)
	if err != nil {
		return err
	}
	if doc == nil {
		return nil
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
	if doc == nil {
		return nil
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
	doc, err := s.labs.GetUserDocument(c.Request().Context(), c.Param("username"), c.Param("uid"), c.Param("slug"))
	if err != nil {
		slog.Warn("lab not found", "username", c.Param("username"), "uid", c.Param("uid"), "slug", c.Param("slug"))
		return nil, echo.NewHTTPError(http.StatusNotFound, "lab not found")
	}
	user := currentUser(c)
	if !s.canReadDocument(user, doc) {
		if user == nil {
			return nil, s.redirectToSignin(c)
		}
		return nil, echo.NewHTTPError(http.StatusNotFound, "lab not found")
	}
	return doc, nil
}

func render(c echo.Context, status int, component templ.Component) error {
	c.Response().Header().Set(echo.HeaderContentType, echo.MIMETextHTMLCharsetUTF8)
	c.Response().WriteHeader(status)
	return component.Render(c.Request().Context(), c.Response().Writer)
}

func (s *Server) withMeta(component templ.Component, meta web.PageMeta) templ.Component {
	return web.ComponentWithPageMeta(component, meta)
}

func (s *Server) websiteMeta(c echo.Context) web.PageMeta {
	return web.WebsitePageMeta(s.metaPublicURL(), web.CanonicalPath(c.Request().URL.Path))
}

func (s *Server) identityMeta(c echo.Context) web.PageMeta {
	return web.IdentityPageMeta(s.metaPublicURL(), web.CanonicalPath(c.Request().URL.Path))
}

func (s *Server) signOutMeta(c echo.Context) web.PageMeta {
	return web.SignOutPageMeta(s.metaPublicURL(), web.CanonicalPath(c.Request().URL.Path))
}

func (s *Server) userMeta(c echo.Context, user *labbit.User) web.PageMeta {
	return web.UserPageMeta(s.metaPublicURL(), web.CanonicalPath(c.Request().URL.Path), user.Username)
}

func (s *Server) documentMeta(c echo.Context, doc *labbit.Document, sectionType, section string) web.PageMeta {
	return s.documentMetaForPath(web.CanonicalPath(c.Request().URL.Path), doc, sectionType, section)
}

func (s *Server) documentMetaForPath(path string, doc *labbit.Document, sectionType, section string) web.PageMeta {
	title, articleSection := documentMetaTitleAndSection(doc, sectionType, section)
	published := doc.UploadedAt
	if published.IsZero() {
		published = doc.CreatedAt
	}
	return web.ArticlePageMeta(
		s.metaPublicURL(),
		web.CanonicalPath(path),
		title,
		web.PlainTextSnippet(doc.Overview, 220),
		"/@"+url.PathEscape(doc.OwnerName),
		articleSection,
		published,
		published,
		documentMetaTags(doc),
	)
}

func (s *Server) metaPublicURL() string {
	if s.publicURL != "" {
		return s.publicURL
	}
	if s.id.publicURL != "" {
		return s.id.publicURL
	}
	return localPublicURL(s.port)
}

func documentMetaTitleAndSection(doc *labbit.Document, sectionType, section string) (string, string) {
	if section == "" || section == "overview" {
		return doc.Title, web.ArticleOverviewValue
	}
	switch sectionType {
	case "labs":
		for _, topic := range doc.Topics {
			if topic.ID == section {
				return topic.Title + " · " + doc.Title + " · Labbit", web.ArticleLabTag
			}
		}
	case "quiz":
		for _, topic := range quizTopics(doc.Questions) {
			if topic.ID == section {
				return topic.Title + " · " + doc.Title + " · Labbit", web.ArticleQuizTag
			}
		}
	}
	return doc.Title, web.ArticleOverviewValue
}

func documentMetaTags(doc *labbit.Document) []string {
	tags := []string{web.ArticleDefaultTag}
	if len(doc.Topics) > 0 {
		tags = append(tags, web.ArticleLabTag)
	}
	if len(doc.Questions) > 0 {
		tags = append(tags, web.ArticleQuizTag)
	}
	return tags
}

func requestTheme(c echo.Context) string {
	cookie, err := c.Cookie("labbit.theme")
	if err != nil {
		return "dark"
	}
	return normalizeTheme(cookie.Value)
}

func normalizeTheme(theme string) string {
	if strings.EqualFold(strings.TrimSpace(theme), "light") {
		return "light"
	}
	return "dark"
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

func sectionExists(doc *labbit.Document, sectionType, section string) bool {
	if sectionType == "labs" && section == "overview" {
		return true
	}
	switch sectionType {
	case "labs":
		for _, topic := range doc.Topics {
			if topic.ID == section {
				return true
			}
		}
	case "quiz":
		for _, topic := range quizTopics(doc.Questions) {
			if topic.ID == section {
				return true
			}
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

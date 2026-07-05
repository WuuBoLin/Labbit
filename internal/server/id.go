// Copyright (C) 2026 WuuBoLin
// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/labstack/echo/v4"
	"golang.org/x/oauth2"

	"labbit/cmd/web"
	"labbit/internal/labbit"
)

const (
	sessionCookieName = "labbit.session"
	sessionTTL        = 24 * time.Hour
	challengeTTL      = 10 * time.Minute
)

type idConfig struct {
	publicURL string
	origin    string
	rpID      string
	secure    bool
}

type oidcProvider struct {
	Name          string
	DisplayName   string
	IssuerURL     string
	ClientID      string
	ClientSecret  string
	UsernameClaim string
	RedirectURL   string

	mu       sync.Mutex
	provider *oidc.Provider
	oauth    *oauth2.Config
}

type oidcStatePayload struct {
	Provider string `json:"provider"`
	Next     string `json:"next"`
}

func newIDConfig(publicURL string, port int) (idConfig, error) {
	if strings.TrimSpace(publicURL) == "" {
		publicURL = localPublicURL(port)
	}
	publicURL = strings.TrimRight(publicURL, "/")
	origin, host, secure, err := originHost(publicURL)
	if err != nil {
		return idConfig{}, err
	}
	return idConfig{publicURL: publicURL, origin: origin, rpID: host, secure: secure}, nil
}

func localPublicURL(port int) string {
	if port <= 0 || port == 80 {
		return "http://localhost"
	}
	return fmt.Sprintf("http://localhost:%d", port)
}

func loadOIDCProviders(publicURL string) map[string]*oidcProvider {
	out := map[string]*oidcProvider{}
	for _, raw := range strings.Split(os.Getenv("OIDC_PROVIDERS"), ",") {
		name := strings.ToLower(strings.TrimSpace(raw))
		if name == "" {
			continue
		}
		prefix := "OIDC_" + strings.ToUpper(strings.ReplaceAll(name, "-", "_")) + "_"
		issuer := strings.TrimSpace(os.Getenv(prefix + "ISSUER_URL"))
		clientID := strings.TrimSpace(os.Getenv(prefix + "CLIENT_ID"))
		clientSecret := strings.TrimSpace(os.Getenv(prefix + "CLIENT_SECRET"))
		if issuer == "" || clientID == "" || clientSecret == "" {
			slog.Warn("oidc provider skipped because required env vars are missing", "provider", name)
			continue
		}
		displayName := strings.TrimSpace(os.Getenv(prefix + "DISPLAY_NAME"))
		if displayName == "" {
			displayName = name
		}
		usernameClaim := strings.TrimSpace(os.Getenv(prefix + "USERNAME_CLAIM"))
		if usernameClaim == "" {
			usernameClaim = "preferred_username"
		}
		out[name] = &oidcProvider{
			Name:          name,
			DisplayName:   displayName,
			IssuerURL:     issuer,
			ClientID:      clientID,
			ClientSecret:  clientSecret,
			UsernameClaim: usernameClaim,
			RedirectURL:   publicURL + "/id/oidc/" + url.PathEscape(name) + "/callback",
		}
	}
	return out
}

func (p *oidcProvider) client(ctx context.Context) (*oidc.Provider, *oauth2.Config, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.provider != nil && p.oauth != nil {
		return p.provider, p.oauth, nil
	}
	provider, err := oidc.NewProvider(ctx, p.IssuerURL)
	if err != nil {
		return nil, nil, err
	}
	oauthConfig := &oauth2.Config{
		ClientID:     p.ClientID,
		ClientSecret: p.ClientSecret,
		RedirectURL:  p.RedirectURL,
		Endpoint:     provider.Endpoint(),
		Scopes:       []string{oidc.ScopeOpenID, "profile", "email"},
	}
	p.provider = provider
	p.oauth = oauthConfig
	return provider, oauthConfig, nil
}

func (s *Server) ensureID() {
	if s.labs == nil {
		return
	}
	if s.id.publicURL == "" {
		id, err := newIDConfig(os.Getenv("PUBLIC_URL"), s.port)
		if err != nil {
			panic(fmt.Sprintf("id config: %v", err))
		}
		s.id = id
	}
	if s.webauthn == nil {
		w, err := webauthn.New(&webauthn.Config{
			RPID:          s.id.rpID,
			RPDisplayName: "Labbit",
			RPOrigins:     []string{s.id.origin},
		})
		if err != nil {
			panic(fmt.Sprintf("webauthn config: %v", err))
		}
		s.webauthn = w
	}
	if s.oidcProviders == nil {
		s.oidcProviders = loadOIDCProviders(s.id.publicURL)
	}
}

func (s *Server) idMiddleware(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		user := s.userFromRequest(c)
		if user != nil {
			c.Set("currentUser", user)
			if user.Status != labbit.UserStatusActive && !pendingAllowedPath(c.Request().URL.Path) {
				return s.redirectToOnboarding(c)
			}
		}
		return next(c)
	}
}

func (s *Server) authDisabledMiddleware(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		s.ensureAuthDisabledUser()
		if s.localUser != nil {
			c.Set("currentUser", s.localUser)
		}
		return next(c)
	}
}

func pendingAllowedPath(path string) bool {
	return path == "/i/onboarding" ||
		path == "/i/theme" ||
		path == "/_/healthz" ||
		strings.HasPrefix(path, "/id") ||
		strings.HasPrefix(path, "/assets/") ||
		path == "/favicon.ico" ||
		path == "/apple-touch-icon.png"
}

func (s *Server) userFromRequest(c echo.Context) *labbit.User {
	cookie, err := c.Cookie(sessionCookieName)
	if err != nil {
		return nil
	}
	user, err := s.labs.GetUserBySession(c.Request().Context(), cookie.Value)
	if err != nil {
		return nil
	}
	return user
}

func currentUser(c echo.Context) *labbit.User {
	user, _ := c.Get("currentUser").(*labbit.User)
	return user
}

func (s *Server) requireSession(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		if currentUser(c) == nil {
			return s.idRequired(c)
		}
		return next(c)
	}
}

func (s *Server) requireActive(next echo.HandlerFunc) echo.HandlerFunc {
	return s.requireSession(func(c echo.Context) error {
		if currentUser(c).Status != labbit.UserStatusActive {
			return s.redirectToOnboarding(c)
		}
		return next(c)
	})
}

func (s *Server) idRequired(c echo.Context) error {
	if wantsHTML(c) {
		return s.redirectToSignin(c)
	}
	return echo.NewHTTPError(http.StatusUnauthorized, "authentication required")
}

func wantsHTML(c echo.Context) bool {
	if c.Request().Header.Get("HX-Request") == "true" {
		return true
	}
	accept := c.Request().Header.Get("Accept")
	return accept == "" || strings.Contains(accept, "text/html")
}

func (s *Server) redirectToSignin(c echo.Context) error {
	next := c.Request().URL.RequestURI()
	target := "/id/authenticate?next=" + url.QueryEscape(next)
	if c.Request().Header.Get("HX-Request") == "true" {
		c.Response().Header().Set("HX-Redirect", target)
		return c.NoContent(http.StatusUnauthorized)
	}
	return c.Redirect(http.StatusSeeOther, target)
}

func (s *Server) redirectToOnboarding(c echo.Context) error {
	next := c.QueryParam("next")
	target := "/i/onboarding"
	if next != "" {
		target += "?next=" + url.QueryEscape(next)
	}
	if c.Request().Header.Get("HX-Request") == "true" {
		c.Response().Header().Set("HX-Redirect", target)
		return c.NoContent(http.StatusForbidden)
	}
	return c.Redirect(http.StatusSeeOther, target)
}

func (s *Server) setSessionCookie(c echo.Context, raw string, expires time.Time) {
	http.SetCookie(c.Response(), &http.Cookie{
		Name:     sessionCookieName,
		Value:    raw,
		Path:     "/",
		Expires:  expires,
		MaxAge:   int(time.Until(expires).Seconds()),
		HttpOnly: true,
		Secure:   s.id.secure,
		SameSite: http.SameSiteLaxMode,
	})
}

func (s *Server) clearSessionCookie(c echo.Context) {
	http.SetCookie(c.Response(), &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   s.id.secure,
		SameSite: http.SameSiteLaxMode,
	})
}

func (s *Server) createSession(c echo.Context, userID string) (*labbit.User, error) {
	raw, expires, err := s.labs.CreateSession(c.Request().Context(), userID, sessionTTL)
	if err != nil {
		return nil, err
	}
	s.setSessionCookie(c, raw, expires)
	return s.labs.GetUser(c.Request().Context(), userID)
}

func (s *Server) idHandler(c echo.Context) error {
	user := currentUser(c)
	if user == nil {
		return echo.NewHTTPError(http.StatusUnauthorized, "authentication required")
	}
	return c.JSON(http.StatusOK, map[string]any{
		"user-id":             user.ID,
		"user-name":           user.Username,
		"status":              user.Status,
		"onboarding-required": user.Status != labbit.UserStatusActive,
	})
}

func (s *Server) authenticateHandler(c echo.Context) error {
	user := currentUser(c)
	if user == nil {
		return render(c, http.StatusOK, web.SignInPage(requestTheme(c), nextTarget(c, "/"), s.oidcButtons()))
	}
	cookie, err := c.Cookie(sessionCookieName)
	if err == nil {
		raw, expires, renewedUser, err := s.labs.RenewSession(c.Request().Context(), cookie.Value, sessionTTL)
		if err == nil {
			s.setSessionCookie(c, raw, expires)
			user = renewedUser
		}
	}
	if user.Status != labbit.UserStatusActive {
		return s.redirectToOnboarding(c)
	}
	return c.Redirect(http.StatusSeeOther, nextTarget(c, "/"))
}

func (s *Server) registerRedirectHandler(c echo.Context) error {
	target := "/id/authenticate"
	if query := c.QueryString(); query != "" {
		target += "?" + query
	}
	return c.Redirect(http.StatusSeeOther, target)
}

func (s *Server) signoutPageHandler(c echo.Context) error {
	return render(c, http.StatusOK, web.SignOutPage(currentUser(c), requestTheme(c), signoutNext(c), refererTarget(c)))
}

func (s *Server) signoutHandler(c echo.Context) error {
	next := signoutNext(c)
	if cookie, err := c.Cookie(sessionCookieName); err == nil {
		if err := s.labs.RevokeSession(c.Request().Context(), cookie.Value); err != nil {
			return err
		}
	}
	s.clearSessionCookie(c)
	if c.Request().Header.Get("HX-Request") == "true" {
		c.Response().Header().Set("HX-Redirect", next)
		return c.NoContent(http.StatusOK)
	}
	return c.Redirect(http.StatusSeeOther, next)
}

func (s *Server) onboardingPageHandler(c echo.Context) error {
	user := currentUser(c)
	if user.Status == labbit.UserStatusActive {
		return c.Redirect(http.StatusSeeOther, safeNext(c.QueryParam("next"), "/"))
	}
	return render(c, http.StatusOK, web.OnboardingPage(user, requestTheme(c), "", c.QueryParam("next")))
}

func (s *Server) onboardingHandler(c echo.Context) error {
	user := currentUser(c)
	if user.Status == labbit.UserStatusActive {
		return c.Redirect(http.StatusSeeOther, safeNext(c.QueryParam("next"), "/"))
	}
	activated, err := s.labs.ActivateUser(c.Request().Context(), user.ID, c.FormValue("username"))
	if err != nil {
		message := err.Error()
		if err == labbit.ErrUsernameTaken {
			message = "That username has been taken."
		}
		return render(c, http.StatusBadRequest, web.OnboardingPage(user, requestTheme(c), message, c.QueryParam("next")))
	}
	c.Set("currentUser", activated)
	return c.Redirect(http.StatusSeeOther, safeNext(c.QueryParam("next"), "/"))
}

func (s *Server) oidcButtons() []web.OIDCButton {
	buttons := make([]web.OIDCButton, 0, len(s.oidcProviders))
	for _, provider := range s.oidcProviders {
		buttons = append(buttons, web.OIDCButton{Name: provider.Name, DisplayName: provider.DisplayName})
	}
	return buttons
}

func (s *Server) passkeyAuthenticateHandler(c echo.Context) error {
	step := c.QueryParam("step")
	switch {
	case step == "begin":
		return s.passkeySigninBegin(c)
	case step == "finish":
		return s.passkeySigninFinish(c)
	default:
		return echo.NewHTTPError(http.StatusBadRequest, "invalid authentication action")
	}
}

func (s *Server) passkeyRegisterHandler(c echo.Context) error {
	step := c.QueryParam("step")
	switch {
	case step == "begin":
		return s.passkeyRegisterBegin(c)
	case step == "finish":
		return s.passkeyRegisterFinish(c)
	default:
		return echo.NewHTTPError(http.StatusBadRequest, "invalid registration action")
	}
}

func (s *Server) passkeyRegisterBegin(c echo.Context) error {
	user, err := s.labs.CreatePendingUser(c.Request().Context())
	if err != nil {
		return err
	}
	webUser, err := s.labs.GetWebAuthnUser(c.Request().Context(), user.ID)
	if err != nil {
		return err
	}
	creation, session, err := s.webauthn.BeginRegistration(webUser, webauthn.WithResidentKeyRequirement(protocol.ResidentKeyRequirementRequired))
	if err != nil {
		return err
	}
	state, err := labbit.NewToken()
	if err != nil {
		return err
	}
	payload, err := json.Marshal(session)
	if err != nil {
		return err
	}
	if err := s.labs.SaveIDChallenge(c.Request().Context(), "passkey-register", state, user.ID, payload, challengeTTL); err != nil {
		return err
	}
	return c.JSON(http.StatusOK, map[string]any{"state": state, "options": creation})
}

func (s *Server) passkeyRegisterFinish(c echo.Context) error {
	state := c.QueryParam("state")
	userID, payload, err := s.labs.ConsumeIDChallenge(c.Request().Context(), "passkey-register", state)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid passkey registration")
	}
	var session webauthn.SessionData
	if err := json.Unmarshal(payload, &session); err != nil {
		return err
	}
	webUser, err := s.labs.GetWebAuthnUser(c.Request().Context(), userID)
	if err != nil {
		return err
	}
	credential, err := s.webauthn.FinishRegistration(webUser, session, c.Request())
	if err != nil {
		slog.Warn("passkey registration failed", "origin", c.Request().Header.Get("Origin"), "rp_id", s.id.rpID, "allowed_origin", s.id.origin, "error", err)
		return echo.NewHTTPError(http.StatusBadRequest, "passkey registration failed")
	}
	if err := s.labs.SaveWebAuthnCredential(c.Request().Context(), userID, *credential); err != nil {
		return err
	}
	user, err := s.createSession(c, userID)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, idFinishResponse(user, nextTarget(c, "/")))
}

func (s *Server) passkeySigninBegin(c echo.Context) error {
	assertion, session, err := s.webauthn.BeginDiscoverableLogin()
	if err != nil {
		return err
	}
	state, err := labbit.NewToken()
	if err != nil {
		return err
	}
	payload, err := json.Marshal(session)
	if err != nil {
		return err
	}
	if err := s.labs.SaveIDChallenge(c.Request().Context(), "passkey-login", state, "", payload, challengeTTL); err != nil {
		return err
	}
	return c.JSON(http.StatusOK, map[string]any{"state": state, "options": assertion})
}

func (s *Server) passkeySigninFinish(c echo.Context) error {
	state := c.QueryParam("state")
	_, payload, err := s.labs.ConsumeIDChallenge(c.Request().Context(), "passkey-login", state)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid passkey sign-in")
	}
	var session webauthn.SessionData
	if err := json.Unmarshal(payload, &session); err != nil {
		return err
	}
	handler := func(rawID, userHandle []byte) (webauthn.User, error) {
		return s.labs.GetWebAuthnUserByCredential(c.Request().Context(), rawID, userHandle)
	}
	webUser, credential, err := s.webauthn.FinishPasskeyLogin(handler, session, c.Request())
	if err != nil {
		return echo.NewHTTPError(http.StatusUnauthorized, "passkey sign-in failed")
	}
	userID := string(webUser.WebAuthnID())
	if err := s.labs.UpdateWebAuthnCredential(c.Request().Context(), *credential); err != nil {
		return err
	}
	user, err := s.createSession(c, userID)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, idFinishResponse(user, nextTarget(c, "/")))
}

func idFinishResponse(user *labbit.User, next string) map[string]any {
	if user.Status != labbit.UserStatusActive {
		next = "/i/onboarding?next=" + url.QueryEscape(next)
	}
	return map[string]any{
		"ok":                  true,
		"next":                next,
		"onboarding-required": user.Status != labbit.UserStatusActive,
	}
}

func (s *Server) oidcStartHandler(c echo.Context) error {
	name := strings.ToLower(c.Param("provider"))
	provider := s.oidcProviders[name]
	if provider == nil {
		return echo.NewHTTPError(http.StatusNotFound, "oidc provider not found")
	}
	_, oauthConfig, err := provider.client(c.Request().Context())
	if err != nil {
		return echo.NewHTTPError(http.StatusBadGateway, "oidc provider unavailable")
	}
	state, err := labbit.NewToken()
	if err != nil {
		return err
	}
	body, _ := json.Marshal(oidcStatePayload{Provider: name, Next: safeNext(c.QueryParam("next"), "/")})
	if err := s.labs.SaveIDChallenge(c.Request().Context(), "oidc", state, "", body, challengeTTL); err != nil {
		return err
	}
	return c.Redirect(http.StatusFound, oauthConfig.AuthCodeURL(state))
}

func (s *Server) oidcCallbackHandler(c echo.Context) error {
	name := strings.ToLower(c.Param("provider"))
	provider := s.oidcProviders[name]
	if provider == nil {
		return echo.NewHTTPError(http.StatusNotFound, "oidc provider not found")
	}
	_, payload, err := s.labs.ConsumeIDChallenge(c.Request().Context(), "oidc", c.QueryParam("state"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid oidc state")
	}
	var state oidcStatePayload
	if err := json.Unmarshal(payload, &state); err != nil || state.Provider != name {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid oidc state")
	}
	oidcProvider, oauthConfig, err := provider.client(c.Request().Context())
	if err != nil {
		return echo.NewHTTPError(http.StatusBadGateway, "oidc provider unavailable")
	}
	token, err := oauthConfig.Exchange(c.Request().Context(), c.QueryParam("code"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "oidc exchange failed")
	}
	rawIDToken, ok := token.Extra("id_token").(string)
	if !ok {
		return echo.NewHTTPError(http.StatusBadRequest, "missing oidc id token")
	}
	idToken, err := oidcProvider.Verifier(&oidc.Config{ClientID: provider.ClientID}).Verify(c.Request().Context(), rawIDToken)
	if err != nil {
		return echo.NewHTTPError(http.StatusUnauthorized, "oidc token verification failed")
	}
	if user, err := s.labs.GetOIDCIdentity(c.Request().Context(), name, idToken.Subject); err == nil {
		_, err = s.createSession(c, user.ID)
		if err != nil {
			return err
		}
		if user.Status != labbit.UserStatusActive {
			return c.Redirect(http.StatusSeeOther, "/i/onboarding?next="+url.QueryEscape(state.Next))
		}
		return c.Redirect(http.StatusSeeOther, state.Next)
	}
	claims := map[string]any{}
	_ = idToken.Claims(&claims)
	suggested := claimString(claims, provider.UsernameClaim)
	user, err := s.labs.CreatePendingUser(c.Request().Context())
	if err != nil {
		return err
	}
	if suggested != "" {
		if activated, activateErr := s.labs.ActivateUser(c.Request().Context(), user.ID, suggested); activateErr == nil {
			user = activated
		}
	}
	if err := s.labs.LinkOIDCIdentity(c.Request().Context(), user.ID, name, idToken.Subject, suggested); err != nil {
		return err
	}
	user, err = s.createSession(c, user.ID)
	if err != nil {
		return err
	}
	if user.Status != labbit.UserStatusActive {
		return c.Redirect(http.StatusSeeOther, "/i/onboarding?next="+url.QueryEscape(state.Next))
	}
	return c.Redirect(http.StatusSeeOther, state.Next)
}

func claimString(claims map[string]any, key string) string {
	value, _ := claims[key].(string)
	return labbit.CleanUsername(value)
}

func safeNext(next, fallback string) string {
	next = strings.TrimSpace(next)
	if next == "" || !strings.HasPrefix(next, "/") || strings.HasPrefix(next, "//") {
		return fallback
	}
	return next
}

func nextHandler(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		c.Set("next", nextParam(c))
		return next(c)
	}
}

func nextTarget(c echo.Context, fallback string) string {
	next, _ := c.Get("next").(string)
	if next == "" {
		next = nextParam(c)
	}
	return safeNext(next, fallback)
}

func nextParam(c echo.Context) string {
	return c.QueryParam("next")
}

func signoutNext(c echo.Context) string {
	if next := nextTarget(c, ""); next != "" {
		return next
	}
	if referer := refererTarget(c); referer != "" {
		return referer
	}
	return "/"
}

func refererTarget(c echo.Context) string {
	referer := strings.TrimSpace(c.Request().Header.Get("Referer"))
	if referer == "" {
		return ""
	}
	u, err := url.Parse(referer)
	if err != nil {
		return ""
	}
	if u.IsAbs() && u.Host != c.Request().Host {
		return ""
	}
	if u.Path == "" || u.Path == "/id/signout" {
		return ""
	}
	if u.RawQuery != "" {
		return u.Path + "?" + u.RawQuery
	}
	return u.Path
}

func (s *Server) canReadDocument(user *labbit.User, doc *labbit.Document) bool {
	if s.disableAuth {
		return true
	}
	if doc.Visibility == labbit.VisibilityPublic {
		return true
	}
	return user != nil && user.ID == doc.OwnerID
}

func loadSessionCookie(c echo.Context) (string, error) {
	cookie, err := c.Cookie(sessionCookieName)
	if err != nil {
		return "", sql.ErrNoRows
	}
	return cookie.Value, nil
}

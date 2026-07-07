// Copyright (C) 2026 WuuBoLin
// SPDX-License-Identifier: GPL-3.0-or-later

package web

import (
	"context"
	"io"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/a-h/templ"
	"golang.org/x/net/html"
)

const (
	SiteName              = "Labbit"
	GlobalOGTitle         = "Labbit · Lab and Quiz viewer"
	GlobalOGDescription   = "Web viewer for lab exam notes. Upload a Labbit XML file and Labbit turns it into a documentation-style workspace with LABs and QUIZ."
	IdentityOGTitle       = "Authenticate ID · Labbit"
	IdentityOGDescription = "Sign in to continue to Labbit."
	SignOutOGTitle        = "Sign out · Labbit"
	SignOutOGDescription  = "Sign out of your Labbit session."
	GlobalOGImageType     = "image/png"
	SocialCardImagePath   = "/assets/img/social-card.png"
	SocialCardImageAlt    = "Labbit social card"
	SocialCardImageWidth  = "1200"
	SocialCardImageHeight = "630"
	TwitterLargeCard      = "summary_large_image"
	TwitterSummaryCard    = "summary"
	ThumbnailImagePath    = "/assets/img/icon-512.png"
	ThumbnailImageAlt     = "Labbit icon"
	ThumbnailImageWidth   = "512"
	ThumbnailImageHeight  = "512"
	GlobalOGLocale        = "en_US"
	GlobalOGDeterminer    = "auto"
	DefaultCanonicalPath  = "/"
	ArticleDefaultTag     = "Labbit"
	ArticleLabTag         = "LAB"
	ArticleQuizTag        = "QUIZ"
	ArticleOverviewValue  = "Overview"
)

type pageMetaKey struct{}

type PageMeta struct {
	Title       string
	Description string
	URL         string
	Type        string
	ImageURL    string
	ImageType   string
	ImageWidth  string
	ImageHeight string
	ImageAlt    string
	Canonical   string
	TwitterCard string

	ProfileUsername string

	ArticleAuthor        string
	ArticlePublishedTime string
	ArticleModifiedTime  string
	ArticleSection       string
	ArticleTags          []string
}

func WithPageMeta(ctx context.Context, meta PageMeta) context.Context {
	return context.WithValue(ctx, pageMetaKey{}, meta)
}

func ComponentWithPageMeta(component templ.Component, meta PageMeta) templ.Component {
	return templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		return component.Render(WithPageMeta(ctx, meta), w)
	})
}

func PageMetaFromContext(ctx context.Context, fallbackTitle string) PageMeta {
	if meta, ok := ctx.Value(pageMetaKey{}).(PageMeta); ok {
		return normalizePageMeta(meta, fallbackTitle)
	}
	return normalizePageMeta(PageMeta{}, fallbackTitle)
}

func WebsitePageMeta(publicURL, path string) PageMeta {
	absoluteURL := AbsoluteURL(publicURL, path)
	return PageMeta{
		Title:       GlobalOGTitle,
		Description: GlobalOGDescription,
		URL:         absoluteURL,
		Type:        "website",
		ImageURL:    AbsoluteURL(publicURL, SocialCardImagePath),
		ImageType:   GlobalOGImageType,
		ImageWidth:  SocialCardImageWidth,
		ImageHeight: SocialCardImageHeight,
		ImageAlt:    SocialCardImageAlt,
		Canonical:   absoluteURL,
		TwitterCard: TwitterLargeCard,
	}
}

func UserPageMeta(publicURL, path, username string) PageMeta {
	absoluteURL := AbsoluteURL(publicURL, path)
	return PageMeta{
		Title:           username + "'s docs · Labbit",
		Description:     "Labbit documents by " + username + ".",
		URL:             absoluteURL,
		Type:            "profile",
		ImageURL:        AbsoluteURL(publicURL, ThumbnailImagePath),
		ImageType:       GlobalOGImageType,
		ImageWidth:      ThumbnailImageWidth,
		ImageHeight:     ThumbnailImageHeight,
		ImageAlt:        ThumbnailImageAlt,
		Canonical:       absoluteURL,
		TwitterCard:     TwitterSummaryCard,
		ProfileUsername: username,
	}
}

func IdentityPageMeta(publicURL, path string) PageMeta {
	return thumbnailWebsiteMeta(publicURL, path, IdentityOGTitle, IdentityOGDescription)
}

func SignOutPageMeta(publicURL, path string) PageMeta {
	return thumbnailWebsiteMeta(publicURL, path, SignOutOGTitle, SignOutOGDescription)
}

func thumbnailWebsiteMeta(publicURL, path, title, description string) PageMeta {
	absoluteURL := AbsoluteURL(publicURL, path)
	return PageMeta{
		Title:       title,
		Description: description,
		URL:         absoluteURL,
		Type:        "website",
		ImageURL:    AbsoluteURL(publicURL, ThumbnailImagePath),
		ImageType:   GlobalOGImageType,
		ImageWidth:  ThumbnailImageWidth,
		ImageHeight: ThumbnailImageHeight,
		ImageAlt:    ThumbnailImageAlt,
		Canonical:   absoluteURL,
		TwitterCard: TwitterSummaryCard,
	}
}

func ArticlePageMeta(publicURL, path, title, description, authorPath, section string, published, modified time.Time, tags []string) PageMeta {
	absoluteURL := AbsoluteURL(publicURL, path)
	return PageMeta{
		Title:                title,
		Description:          fallback(description, GlobalOGDescription),
		URL:                  absoluteURL,
		Type:                 "article",
		ImageURL:             AbsoluteURL(publicURL, SocialCardImagePath),
		ImageType:            GlobalOGImageType,
		ImageWidth:           SocialCardImageWidth,
		ImageHeight:          SocialCardImageHeight,
		ImageAlt:             SocialCardImageAlt,
		Canonical:            absoluteURL,
		TwitterCard:          TwitterLargeCard,
		ArticleAuthor:        AbsoluteURL(publicURL, authorPath),
		ArticlePublishedTime: formatMetaTime(published),
		ArticleModifiedTime:  formatMetaTime(modified),
		ArticleSection:       section,
		ArticleTags:          compactStrings(tags),
	}
}

func AbsoluteURL(publicURL, path string) string {
	publicURL = strings.TrimRight(strings.TrimSpace(publicURL), "/")
	if publicURL == "" {
		publicURL = "http://localhost"
	}
	if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
		return path
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return publicURL + path
}

func PlainTextSnippet(htmlText string, maxRunes int) string {
	text := plainText(htmlText)
	if maxRunes > 0 && utf8.RuneCountInString(text) > maxRunes {
		runes := []rune(text)
		text = strings.TrimSpace(string(runes[:maxRunes]))
		text = strings.TrimRight(text, ".,;:") + "..."
	}
	return text
}

func MetaDescription(ctx context.Context, fallbackTitle string) string {
	return PageMetaFromContext(ctx, fallbackTitle).Description
}

func MetaTitle(ctx context.Context, fallbackTitle string) string {
	return PageMetaFromContext(ctx, fallbackTitle).Title
}

func MetaType(ctx context.Context, fallbackTitle string) string {
	return PageMetaFromContext(ctx, fallbackTitle).Type
}

func MetaURL(ctx context.Context, fallbackTitle string) string {
	return PageMetaFromContext(ctx, fallbackTitle).URL
}

func MetaImageURL(ctx context.Context, fallbackTitle string) string {
	return PageMetaFromContext(ctx, fallbackTitle).ImageURL
}

func MetaImageType(ctx context.Context, fallbackTitle string) string {
	return PageMetaFromContext(ctx, fallbackTitle).ImageType
}

func MetaImageWidth(ctx context.Context, fallbackTitle string) string {
	return PageMetaFromContext(ctx, fallbackTitle).ImageWidth
}

func MetaImageHeight(ctx context.Context, fallbackTitle string) string {
	return PageMetaFromContext(ctx, fallbackTitle).ImageHeight
}

func MetaImageAlt(ctx context.Context, fallbackTitle string) string {
	return PageMetaFromContext(ctx, fallbackTitle).ImageAlt
}

func MetaCanonical(ctx context.Context, fallbackTitle string) string {
	return PageMetaFromContext(ctx, fallbackTitle).Canonical
}

func MetaTwitterCard(ctx context.Context, fallbackTitle string) string {
	return PageMetaFromContext(ctx, fallbackTitle).TwitterCard
}

func MetaProfileUsername(ctx context.Context, fallbackTitle string) string {
	return PageMetaFromContext(ctx, fallbackTitle).ProfileUsername
}

func MetaArticleAuthor(ctx context.Context, fallbackTitle string) string {
	return PageMetaFromContext(ctx, fallbackTitle).ArticleAuthor
}

func MetaArticlePublishedTime(ctx context.Context, fallbackTitle string) string {
	return PageMetaFromContext(ctx, fallbackTitle).ArticlePublishedTime
}

func MetaArticleModifiedTime(ctx context.Context, fallbackTitle string) string {
	return PageMetaFromContext(ctx, fallbackTitle).ArticleModifiedTime
}

func MetaArticleSection(ctx context.Context, fallbackTitle string) string {
	return PageMetaFromContext(ctx, fallbackTitle).ArticleSection
}

func MetaArticleTags(ctx context.Context, fallbackTitle string) []string {
	return PageMetaFromContext(ctx, fallbackTitle).ArticleTags
}

func normalizePageMeta(meta PageMeta, fallbackTitle string) PageMeta {
	meta.Title = fallback(meta.Title, fallback(fallbackTitle, GlobalOGTitle))
	meta.Description = fallback(meta.Description, GlobalOGDescription)
	meta.Type = fallback(meta.Type, "website")
	meta.URL = fallback(meta.URL, AbsoluteURL("", DefaultCanonicalPath))
	meta.Canonical = fallback(meta.Canonical, meta.URL)
	meta.ImageURL = fallback(meta.ImageURL, AbsoluteURL("", SocialCardImagePath))
	meta.ImageType = fallback(meta.ImageType, GlobalOGImageType)
	meta.ImageWidth = fallback(meta.ImageWidth, SocialCardImageWidth)
	meta.ImageHeight = fallback(meta.ImageHeight, SocialCardImageHeight)
	meta.ImageAlt = fallback(meta.ImageAlt, SocialCardImageAlt)
	meta.TwitterCard = fallback(meta.TwitterCard, TwitterLargeCard)
	return meta
}

func formatMetaTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339)
}

func compactStrings(values []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

func plainText(htmlText string) string {
	tokenizer := html.NewTokenizer(strings.NewReader(htmlText))
	var parts []string
	for {
		tokenType := tokenizer.Next()
		switch tokenType {
		case html.ErrorToken:
			if tokenizer.Err() == io.EOF {
				return strings.Join(strings.Fields(strings.Join(parts, " ")), " ")
			}
			return strings.Join(strings.Fields(htmlText), " ")
		case html.TextToken:
			text := strings.TrimSpace(html.UnescapeString(string(tokenizer.Text())))
			if text != "" {
				parts = append(parts, text)
			}
		}
	}
}

func fallback(value, fallbackValue string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallbackValue
	}
	return value
}

func CanonicalPath(rawPath string) string {
	if rawPath == "" {
		return DefaultCanonicalPath
	}
	if parsed, err := url.Parse(rawPath); err == nil && parsed.Path != "" {
		return parsed.Path
	}
	return rawPath
}

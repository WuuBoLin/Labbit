// Copyright (C) 2026 WuuBoLin
// SPDX-License-Identifier: GPL-3.0-or-later

package labbit

import "time"

type Document struct {
	ID         int64
	UID        string
	Slug       string
	Title      string
	Accent     string
	Hash       string
	OwnerID    string
	OwnerName  string
	Visibility string
	Overview   string
	Topics     []Topic
	Questions  []Question
	CreatedAt  time.Time
	UploadedAt time.Time
}

type User struct {
	ID        string
	Username  string
	Status    string
	CreatedAt time.Time
	UpdatedAt time.Time
}

type RecentDocument struct {
	Document   *Document
	Visibility string
	UploadedAt time.Time
}

type Topic struct {
	ID    string
	Kind  string
	Title string
	Items []Task
}

type Task struct {
	ID            string
	Title         string
	Prompt        string
	Hints         []Hint
	HintCount     int
	SolutionCount int
}

type Hint struct {
	ID    string
	Kind  string
	Title string
	Body  string
}

type Question struct {
	ID          string
	TopicID     string
	TopicTitle  string
	Prompt      string
	Kind        string
	Options     []Option
	Explanation string
}

type Option struct {
	ID      string
	Label   string
	Correct bool
}

type SearchResult struct {
	Kind      string
	TargetID  string
	Title     string
	Excerpt   string
	SectionID string
}

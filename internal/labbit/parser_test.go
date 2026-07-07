// Copyright (C) 2026 WuuBoLin
// SPDX-License-Identifier: GPL-3.0-or-later

package labbit

import (
	"strings"
	"testing"
)

const sampleLab = `<labbit title="Linux Services Exam" slug="linux-services" accent="#ff3366">
<overview>
# Linux Services
Prepare Samba and networking tasks.
</overview>
<lab>
  <topic id="samba" title="Samba">
    <task id="setup-samba" title="Setup Samba">
Install packages.
<hint title="Package">
Use the Samba package.
</hint>
<solution>
Commands:
` + "```sh" + `
dnf install samba
systemctl enable --now smb
` + "```" + `
Explanation:
This installs and starts Samba.
</solution>
    </task>
  </topic>
</lab>
<quiz>
  <topic id="basics" title="Basics">
    <question id="daemon" type="single">
      <prompt>Which service handles SMB file sharing?</prompt>
      <option id="a" correct="true">smb</option>
      <option id="b">sshd</option>
      <explanation>smb provides SMB file shares.</explanation>
    </question>
    <question id="ports" type="multiple">
      <prompt>Select SMB-related ports.</prompt>
      <option id="a" correct="true">445</option>
      <option id="b" correct="true">139</option>
      <option id="c">22</option>
      <explanation>SMB uses 445 and NetBIOS session traffic may use 139.</explanation>
    </question>
  </topic>
</quiz>
</labbit>`

func TestParseDocument(t *testing.T) {
	doc, err := Parse(strings.NewReader(sampleLab))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if doc.Title != "Linux Services Exam" {
		t.Fatalf("title = %q", doc.Title)
	}
	if doc.Slug != "linux-services" {
		t.Fatalf("slug = %q", doc.Slug)
	}
	if doc.Accent != "#ff3366" {
		t.Fatalf("accent = %q", doc.Accent)
	}
	if len(doc.Topics) != 1 || len(doc.Topics[0].Items) != 1 {
		t.Fatalf("topics = %#v", doc.Topics)
	}
	task := doc.Topics[0].Items[0]
	if strings.Contains(task.Prompt, "dnf install") {
		t.Fatal("prompt leaked solution")
	}
	if strings.Contains(task.Prompt, "Samba package") {
		t.Fatal("prompt leaked inline hint")
	}
	if !strings.Contains(task.Prompt, "data-inline-hint-toggle") {
		t.Fatalf("prompt did not include inline hint control: %s", task.Prompt)
	}
	if task.HintCount != 2 || len(task.Hints) != 2 {
		t.Fatalf("hints = %#v", task.Hints)
	}
	if task.Hints[0].Kind != "hint" || !strings.Contains(task.Hints[0].Body, "Samba package") {
		t.Fatalf("first hint = %#v", task.Hints[0])
	}
	if task.Hints[1].Kind != "solution" {
		t.Fatalf("solution hint kind = %#v", task.Hints[1])
	}
	if !strings.Contains(task.Hints[1].Body, "dnf install samba") {
		t.Fatalf("solution did not render code: %s", task.Hints[1].Body)
	}
	if !strings.Contains(task.Hints[1].Body, "highlighted-code") {
		t.Fatalf("solution did not render highlighted code: %s", task.Hints[1].Body)
	}
	if len(doc.Questions) != 2 {
		t.Fatalf("questions = %d", len(doc.Questions))
	}
	if doc.Questions[1].Kind != "multiple" {
		t.Fatalf("question kind = %q", doc.Questions[1].Kind)
	}
}

func TestParseDefaultAccent(t *testing.T) {
	doc, err := Parse(strings.NewReader(`<labbit title="Default" accent="DEFAULT"><overview>Overview</overview></labbit>`))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if doc.Accent != DefaultAccent {
		t.Fatalf("accent = %q", doc.Accent)
	}
}

func TestParseRejectsLegacyAnswerTag(t *testing.T) {
	_, err := Parse(strings.NewReader(`<labbit title="Legacy Answer">
<overview>Overview</overview>
<lab>
  <topic title="Topic">
    <task title="Task">
Visible prompt.
<answer>Legacy hidden content.</answer>
    </task>
  </topic>
</lab>
</labbit>`))
	if err == nil || !strings.Contains(err.Error(), "answer tag") {
		t.Fatalf("expected legacy answer tag error, got %v", err)
	}
}

func TestParseInvalidAccentFallsBack(t *testing.T) {
	doc, err := Parse(strings.NewReader(`<labbit title="Bad Accent" accent="blue"><overview>Overview</overview></labbit>`))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if doc.Accent != DefaultAccent {
		t.Fatalf("accent = %q", doc.Accent)
	}
}

func TestParseDedentsIndentedTaskMarkdown(t *testing.T) {
	doc, err := Parse(strings.NewReader(`<labbit title="Indented">
  <overview>
    Overview
  </overview>
  <lab>
    <topic title="Topic">
      <task title="Task">
	Create an encrypted variables file.

	The encrypted file must contain:

	- a deployment user password
	- an API token placeholder
      </task>
    </topic>
  </lab>
</labbit>`))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	prompt := doc.Topics[0].Items[0].Prompt
	if strings.Contains(prompt, "code-shell") {
		t.Fatalf("prompt rendered as code block: %s", prompt)
	}
	if !strings.Contains(prompt, "<li>a deployment user password</li>") {
		t.Fatalf("prompt did not render list: %s", prompt)
	}
}

func TestParseCollapseComponent(t *testing.T) {
	doc, err := Parse(strings.NewReader(`<labbit title="Collapse">
  <overview>
    <collapse title="Reference ports"><![CDATA[
| Service | Port |
| --- | ---: |
| SSH | ` + "`22`" + ` |
]]></collapse>
  </overview>
  <lab>
    <topic title="Topic">
      <task title="Task">
Read the reference.
<collapse title="Background">
- visible context
</collapse>
      </task>
    </topic>
  </lab>
</labbit>`))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if !strings.Contains(doc.Overview, `<details class="labbit-collapse">`) {
		t.Fatalf("overview collapse was not rendered: %s", doc.Overview)
	}
	if !strings.Contains(doc.Overview, `<summary>Reference ports</summary>`) {
		t.Fatalf("overview collapse title was not rendered: %s", doc.Overview)
	}
	if !strings.Contains(doc.Overview, `<table class="labbit-table">`) {
		t.Fatalf("collapse markdown body was not rendered: %s", doc.Overview)
	}
	prompt := doc.Topics[0].Items[0].Prompt
	if !strings.Contains(prompt, `<summary>Background</summary>`) {
		t.Fatalf("task collapse was not rendered: %s", prompt)
	}
	if strings.Contains(prompt, "data-hint") || strings.Contains(prompt, "inline-hint") {
		t.Fatalf("collapse was confused with hint controls: %s", prompt)
	}
}

func TestParseImageComponent(t *testing.T) {
	doc, err := Parse(strings.NewReader(`<labbit title="Images">
  <overview>
    <image type="svg" alt="Service graph"><![CDATA[
<svg viewBox="0 0 120 60" xmlns="http://www.w3.org/2000/svg">
  <rect x="10" y="10" width="100" height="40" fill="#18181b" onclick="alert(1)"/>
  <text x="60" y="35" text-anchor="middle" fill="#e4e4e7">API</text>
</svg>
]]></image>
  </overview>
  <lab>
    <topic title="Topic">
      <task title="Task">
Look at the screenshot.
<image type="png" alt="Terminal screenshot">
iVBORw0KGgo=
</image>
      </task>
    </topic>
  </lab>
  <quiz>
    <topic title="Quiz">
      <question type="single">
	<prompt><image type="svg" alt="Prompt diagram"><![CDATA[<svg viewBox="0 0 10 10"><circle cx="5" cy="5" r="4"/></svg>]]></image></prompt>
	<option correct="true">A</option>
	<explanation><image type="svg" alt="Explanation diagram"><![CDATA[<svg viewBox="0 0 10 10"><line x1="0" y1="0" x2="10" y2="10"/></svg>]]></image></explanation>
      </question>
    </topic>
  </quiz>
</labbit>`))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if !strings.Contains(doc.Overview, `<figure class="labbit-image labbit-svg"`) {
		t.Fatalf("overview svg was not rendered: %s", doc.Overview)
	}
	if strings.Contains(doc.Overview, "onclick") {
		t.Fatalf("unsafe svg attribute was not removed: %s", doc.Overview)
	}
	prompt := doc.Topics[0].Items[0].Prompt
	if !strings.Contains(prompt, `data:image/png;base64,iVBORw0KGgo=`) {
		t.Fatalf("task base64 image was not rendered: %s", prompt)
	}
	if !strings.Contains(doc.Questions[0].Prompt, `aria-label="Prompt diagram"`) {
		t.Fatalf("quiz prompt image was not rendered: %s", doc.Questions[0].Prompt)
	}
	if !strings.Contains(doc.Questions[0].Explanation, `aria-label="Explanation diagram"`) {
		t.Fatalf("quiz explanation image was not rendered: %s", doc.Questions[0].Explanation)
	}
}

func TestParseRequiresOverview(t *testing.T) {
	_, err := Parse(strings.NewReader(`<labbit title="Bad"></labbit>`))
	if err == nil || !strings.Contains(err.Error(), "overview") {
		t.Fatalf("expected overview error, got %v", err)
	}
}

func TestCheckQuestion(t *testing.T) {
	q := Question{
		Options: []Option{
			{ID: "a", Correct: true},
			{ID: "b", Correct: true},
			{ID: "c"},
		},
	}
	ok, _ := CheckQuestion(q, []string{"a", "b"})
	if !ok {
		t.Fatal("expected correct answer")
	}
	ok, _ = CheckQuestion(q, []string{"a"})
	if ok {
		t.Fatal("expected incomplete answer to be wrong")
	}
}

---
name: labbit-authoring
description: Generate valid Labbit XML lab, quiz, or combined lab-and-quiz workspaces for the Labbit web viewer. Use when creating or validating Labbit XML files, component structure, IDs, quiz questions, hints, solutions, images, or Markdown content inside Labbit documents.
---

# Labbit Authoring Guide

Labbit reads one XML file that defines a lab, quiz, or combined lab-and-quiz workspace. This guide is the canonical specification for generating valid Labbit XML.

## Core Model

A Labbit file is XML with Markdown content inside text-bearing elements. Structural Labbit tags are XML elements; prose, lists, tables, code blocks, diagrams, hints, solutions, collapses, and quiz prompt or explanation content can be composed freely wherever the component is allowed.

CDATA is not a Labbit component. CDATA is only an XML mechanism for placing Markdown, code, SVG, or base64 text inside an element without escaping every `<`, `>`, and `&`. Labbit components such as `<image>`, `<collapse>`, `<hint>`, and `<solution>` must remain real XML tags outside CDATA.

Text and visible components can be interleaved:

````xml
<task id="inspect-service" title="Inspect a service">
Read the unit status before changing anything.

<image type="svg" alt="Service state flow"><![CDATA[
<svg viewBox="0 0 260 80" xmlns="http://www.w3.org/2000/svg">
  <rect x="10" y="20" width="90" height="40" rx="6" fill="#18181b" stroke="#1d9bf0"/>
  <text x="55" y="45" text-anchor="middle" font-size="13">inactive</text>
  <line x1="105" y1="40" x2="155" y2="40" stroke="#1d9bf0" stroke-width="2"/>
  <rect x="160" y="20" width="90" height="40" rx="6" fill="#18181b" stroke="#22c55e"/>
  <text x="205" y="45" text-anchor="middle" font-size="13">active</text>
</svg>
]]></image>

Then capture the command output and explain the current state.

<collapse title="Useful status fields">
Focus on `Loaded`, `Active`, and recent log lines.
</collapse>

<hint title="Status command">
Use `systemctl status`.
</hint>
</task>
````

## File Structure

Use exactly one `<labbit>` root element. The root must contain one `<overview>`. It may also contain one `<lab>`, one `<quiz>`, or both.

````xml
<labbit title="Linux Services Exam" slug="linux-services" accent="DEFAULT">
  <overview>
# Linux Services Exam

Practice service setup, validation, and troubleshooting.
  </overview>

  <lab>
    <topic id="samba" title="Samba">
      <task id="setup-samba" title="Setup Samba">
Install Samba and make it start at boot.

Requirements:

- install the required packages
- start the service immediately
- enable the service for reboot

<hint title="Service package">
Use the `samba` package and the `smb` service.
</hint>

<solution><![CDATA[
Install and start Samba:

```sh
dnf install -y samba
systemctl enable --now smb
```

Verify the service:

```sh
systemctl status smb
```
]]></solution>
      </task>
    </topic>
  </lab>

  <quiz>
    <topic id="basics" title="Basics">
      <question id="smb-service" type="single">
        <prompt>Which daemon provides SMB file sharing?</prompt>
        <option id="a" correct="true">smb</option>
        <option id="b">sshd</option>
        <explanation>The `smb` service provides SMB file sharing.</explanation>
      </question>
    </topic>
  </quiz>
</labbit>
````

Required root content:

- `<labbit title="...">`
- `<overview>...</overview>`

Optional root attributes:

- `slug="..."`: stable URL slug; use lowercase kebab-case.
- `accent="#RRGGBB"`: custom accent color.
- `accent="DEFAULT"` or missing: use the default accent.

Optional root sections:

- `<lab>`: contains lab topics and tasks.
- `<quiz>`: contains quiz topics and questions.

IDs:

- Use stable lowercase kebab-case IDs for `topic`, `task`, `question`, and `option`.
- IDs should be unique within their natural scope.
- If an ID is omitted, Labbit derives one from the title or surrounding content, but generated files should provide explicit stable IDs.

## Components

=> `<labbit>`
> The root element for one Labbit document.

- Required.
- Must wrap the entire file.
- Requires a non-empty `title` attribute.
- May include `slug` and `accent` attributes.
- Must contain one `<overview>`.
- May contain `<lab>`, `<quiz>`, or both.

=> `<overview>`
> The introductory content shown before lab tasks and quiz questions.

- Required.
- May contain Markdown.
- May contain `<note>`, `<callout>`, `<collapse>`, and `<image>`.
- Should describe the document scope, prerequisites, context, or orientation material.

=> `<lab>`
> The optional section that groups hands-on tasks.

- Optional.
- Contains one or more lab `<topic>` elements when present.
- Does not directly contain tasks; tasks belong inside lab topics.

=> `<topic id="..." title="...">`
> A named group of lab tasks or quiz questions.

- Required inside `<lab>` and `<quiz>` when those sections are present.
- Requires a meaningful `title`.
- Should include a stable `id`.
- Inside `<lab>`, contains one or more `<task>` elements.
- Inside `<quiz>`, contains one or more `<question>` elements.

=> `<task id="..." title="...">`
> One hands-on lab task, including visible prompt content and optional revealable help.

- Required inside lab topics.
- Requires a meaningful `title`.
- Should include a stable `id`.
- The visible task body may contain Markdown, `<note>`, `<callout>`, `<collapse>`, `<image>`, and `<hint>`.
- May contain one `<solution>`.
- May contain any number of `<hint>` elements.
- `<hint>` elements appear in the visible task flow as reveal controls; their bodies are hidden until revealed.
- `<solution>` is hidden from the visible task body and is shown through the task solution control.
- Do not use the legacy `<answer>` tag; it is not supported.

=> `<hint id="..." title="...">`
> Hidden inline help that is anchored at its exact position in a task.

- Optional inside `<task>`.
- Requires a `title`; if omitted, Labbit falls back to a generic title.
- May include a stable `id`.
- May contain Markdown.
- May contain `<note>`, `<callout>`, `<collapse>`, and `<image>`.
- Use as an inline revealable component within the task sequence.

=> `<solution>`
> Hidden task-level solution content.

- Optional inside `<task>`.
- A task may have no solution or one solution.
- May contain Markdown.
- May contain `<note>`, `<callout>`, `<collapse>`, and `<image>`.
- Represents the task's complete revealable solution block.
- Is not part of the visible prompt flow.

=> `<quiz>`
> The optional section that groups questions.

- Optional.
- Contains one or more quiz `<topic>` elements when present.
- Does not directly contain questions; questions belong inside quiz topics.

=> `<question id="..." type="single|multiple">`
> One quiz question.

- Required inside quiz topics.
- Should include a stable `id`.
- Must contain one `<prompt>`.
- Must contain at least two `<option>` elements.
- Must contain one `<explanation>`.
- The prompt and explanation are full text-bearing content areas; they may use Markdown and visible formatting components.
- Use `type="single"` for radio-button questions.
- Use `type="multiple"` for checkbox questions.
- `type="checkbox"` and `type="multiple-choice"` are accepted as aliases for `multiple`, but generated files should use `multiple`.
- Missing or unrecognized `type` values are treated as `single`.

=> `<prompt>`
> The rendered question prompt.

- Required inside `<question>`.
- May contain Markdown.
- May contain `<note>`, `<callout>`, `<collapse>`, and `<image>`.
- Should contain the full question stem.

=> `<option id="..." correct="true">`
> One selectable answer label for a quiz question.

- Required inside `<question>`.
- Each question must have at least two options.
- Should include a stable `id`.
- The label is plain text after XML text extraction.
- Mark correct answers with `correct="true"`.
- Omit `correct` or use `correct="false"` for incorrect answers.
- For `type="single"`, exactly one option must be correct.
- For `type="multiple"`, one or more options may be correct.

=> `<explanation>`
> Feedback shown after a quiz answer is submitted.

- Required inside `<question>`.
- May contain Markdown.
- May contain `<note>`, `<callout>`, `<collapse>`, and `<image>`.
- Should explain the answer in a way that is useful whether the learner was right or wrong.

=> `<note>`
> A short note rendered as quoted supporting text.

- Optional inside text-bearing content.
- May contain Markdown text.
- May appear in overview, task bodies, hints, solutions, prompts, explanations, and collapses.
- Use only as a content wrapper; it is not hidden.

=> `<callout>`
> A general quoted callout rendered as supporting text.

- Optional inside text-bearing content.
- May contain Markdown text.
- May appear in overview, task bodies, hints, solutions, prompts, explanations, and collapses.
- Use only as a content wrapper; it is not hidden.

=> `<collapse title="...">`
> Visible collapsible content.

- Optional inside text-bearing content.
- Requires a `title`; if omitted, Labbit uses `Details`.
- May contain Markdown.
- May contain `<note>`, `<callout>`, and `<image>`.
- Is visible as an expandable details block.
- Is not hidden hint or solution content.

=> `<image type="..." alt="...">`
> A visible inline image component.

- Optional inside text-bearing content.
- Requires `type`.
- Requires useful `alt` text; if omitted, Labbit uses a generic fallback.
- The body must be inline SVG or base64 raster image data.
- Supported SVG types: `svg`, `image/svg+xml`.
- Supported raster types: `png`, `jpg`, `jpeg`, `webp`, `gif`, and their image MIME forms.
- Raster bodies may be raw base64 or a data URL; whitespace is ignored.
- Unsupported types or invalid image bodies render nothing.

## Markdown

Markdown is supported in text-bearing elements:

- `<overview>`
- `<task>` visible body
- `<hint>`
- `<solution>`
- `<prompt>`
- `<explanation>`
- `<note>`
- `<callout>`
- `<collapse>`

Supported Markdown includes:

- ATX and Setext headings
- paragraphs and line breaks
- bold, italic, strikethrough, and inline code
- links and autolinks
- blockquotes
- unordered lists
- ordered lists, including non-1 starts
- task-list checkboxes
- pipe tables with optional left, center, or right alignment
- definition lists
- footnotes
- thematic breaks
- indented and fenced code blocks with optional language tags

Markdown images are not used for images. Use the Labbit `<image>` component instead. Raw HTML in Markdown is escaped, not rendered as HTML.

Table example:

```markdown
| Service | Port |
| --- | ---: |
| SSH | `22` |
| HTTPS | `443` |
```

Fenced code example:

````markdown
```sh
systemctl enable --now smb
systemctl status smb
```
````

## XML and CDATA Rules

Labbit files must be valid XML.

- Use CDATA inside a content element when Markdown, code, SVG, or base64 contains XML-sensitive characters.
- Otherwise escape XML-sensitive characters as `&lt;`, `&gt;`, and `&amp;`.
- Keep Labbit structural tags outside CDATA.
- Do not wrap `<hint>`, `<solution>`, `<collapse>`, `<image>`, `<task>`, `<question>`, or other Labbit tags inside CDATA.
- Do not use an XML declaration or multiple root elements.

Correct:

````xml
<hint title="Vault command"><![CDATA[
```sh
ansible-vault create group_vars/all/vault.yml
```
]]></hint>
````

Incorrect:

````xml
<![CDATA[
<hint title="Vault command">
ansible-vault create group_vars/all/vault.yml
</hint>
]]>
````

## Image Rules

SVG images:

- Use `type="svg"` for diagrams, graphs, flowcharts, topology maps, and other explanatory visuals.
- Include a valid `<svg>` root in the image body.
- Prefer a `viewBox` so the image scales correctly.
- Keep SVGs transparent; do not add an overall background rectangle.
- Do not rely on browser-default black fills for meaningful shapes.
- Use explicit colors for non-text diagram shapes, lines, arrows, and highlights.
- Labbit controls SVG text contrast in the viewer, so avoid hard-coded text fill and stroke colors unless a specific label color is necessary.
- Use inline SVG presentation attributes, safe `style` attributes, or a local `<style>` block.
- Do not use scripts, event attributes, external CSS, imported fonts, remote URLs, external images, or non-local CSS `url(...)` references.
- Local paint-server references such as `url(#arrow)` are supported.

Raster images:

- Use raster images only when the content must be a picture, screenshot, or photo.
- Use base64 image data in the image body.
- Supported raster formats are PNG, JPEG, WEBP, and GIF.
- Include accurate `alt` text.

SVG example:

````xml
<image type="svg" alt="Client connects to web server"><![CDATA[
<svg viewBox="0 0 360 120" xmlns="http://www.w3.org/2000/svg">
  <defs>
    <marker id="arrow" viewBox="0 0 10 10" refX="9" refY="5" markerWidth="7" markerHeight="7" orient="auto">
      <path d="M 0 0 L 10 5 L 0 10 z" fill="#1d9bf0"/>
    </marker>
  </defs>
  <rect x="20" y="35" width="90" height="50" rx="6" fill="#18181b" stroke="#1d9bf0"/>
  <text x="65" y="65" text-anchor="middle" font-size="14">Client</text>
  <line x1="115" y1="60" x2="240" y2="60" stroke="#1d9bf0" stroke-width="2" marker-end="url(#arrow)"/>
  <rect x="245" y="35" width="95" height="50" rx="6" fill="#18181b" stroke="#22c55e"/>
  <text x="292" y="65" text-anchor="middle" font-size="14">Server</text>
</svg>
]]></image>
````

Raster example:

```xml
<image type="png" alt="Terminal screenshot">
iVBORw0KGgo...
</image>
```

## Lab Authoring Rules

- Write concrete, verifiable tasks.
- Use task titles that identify the action or outcome.
- Keep solution-only commands, final config, completed files, and final answers inside `<hint>` or `<solution>` unless they are intentional starter material.
- Place inline revealable help at the relevant location with `<hint>`.
- Place complete task-level revealable solutions in `<solution>`.
- Include exact commands, paths, file contents, verification steps, and explanations when those details are part of the expected answer.
- Avoid vague hidden content such as "configure it correctly."

Interleaved task example:

````xml
<task id="create-inventory" title="Create an inventory">
Create an inventory with a `web` group and two hosts.

<hint title="Inventory pattern"><![CDATA[
Use an INI group header:

```ini
[web]
web1
web2
```
]]></hint>

After writing the file, run the playbook against that inventory.

<collapse title="Command format">
`ansible-playbook -i INVENTORY PLAYBOOK`
</collapse>

<solution><![CDATA[
Create `inventory.ini`:

```ini
[web]
web1
web2
```

Run the playbook:

```sh
ansible-playbook -i inventory.ini site.yml
```
]]></solution>
</task>
````

## Quiz Authoring Rules

- Single-choice questions must have exactly one correct option.
- Multiple-choice questions must have at least one correct option.
- Each question must include an explanation.
- Avoid "all of the above" and "none of the above" unless they are materially part of the assessment.
- Keep option labels clear and distinct.
- Use Markdown, tables, code blocks, collapses, callouts, and images in `<prompt>` or `<explanation>` whenever the question needs formatted context.
- Put diagrams or extended context in `<prompt>` or `<explanation>`, not inside `<option>`.

Quiz example:

````xml
<question id="smb-ports" type="multiple">
  <prompt><![CDATA[
Review the service notes and select every SMB-related port.

| Service | Port |
| --- | ---: |
| SSH | `22` |
| SMB over TCP | `445` |
| NetBIOS session service | `139` |
  ]]></prompt>
  <option id="a" correct="true">445</option>
  <option id="b" correct="true">139</option>
  <option id="c">22</option>
  <explanation>
SMB commonly uses TCP 445. NetBIOS session traffic may use TCP 139. SSH uses TCP 22.

<collapse title="Why SSH is not selected">
SSH is a remote shell protocol. It is commonly used to administer servers, but it is not part of SMB file sharing.
</collapse>
  </explanation>
</question>
````

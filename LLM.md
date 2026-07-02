# Labbit Authoring Guide

Labbit reads one XML file containing Markdown lab and quiz content. Generate valid XML that follows this guide exactly.

## File Format

Use one `<labbit>` root element. Markdown is allowed inside content elements.

````xml
<labbit title="Linux Services Exam" slug="linux-services" accent="DEFAULT">
  <overview>
# Linux Services Exam
Practice Linux service setup and validation.
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

<hint title="Commands"><![CDATA[
```sh
dnf install -y samba
systemctl enable --now smb
```
]]></hint>

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

This installs Samba, starts the SMB daemon now, and enables it for future boots.
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

Required:

- `<labbit title="...">`
- `<overview>...</overview>`

Optional root attributes:

- `slug="..."`: stable URL slug; use lowercase kebab-case.
- `accent="#RRGGBB"`: custom accent color.
- `accent="DEFAULT"` or missing: use the default accent.

Optional sections:

- `<lab>` with lab topics and tasks
- `<quiz>` with quiz topics and questions

## Components

Use stable lowercase kebab-case IDs for `topic`, `task`, `question`, and `option`.

`<overview>`

- Required.
- Describe what the lab/exam covers.
- May contain Markdown headings, lists, inline code, and fenced code blocks.

`<lab>` and `<topic>`

- `<lab>` contains one or more `<topic id="..." title="...">`.
- A lab topic contains one or more `<task>`.

`<task id="..." title="...">`

- Contains the visible learner prompt plus optional hidden answer content.
- The visible prompt must describe the task, requirements, constraints, expected result, and starter material.
- Do not put solution-only commands, final config, or exact completed files directly in the visible prompt.

`<hint title="...">`

- Hidden inline answer content.
- Appears at the exact location of the tag when revealed.
- Use for answer chunks that naturally belong between visible requirements or steps.
- Content should read like normal prose/code when revealed, not like quiz feedback.
- May contain anything, including Markdown, lists, commands, configuration snippets, fenced code blocks, and explanations.

`<solution>`

- Hidden separated full solution.
- Use when a complete final procedure or final explanation should be shown as one block.
- A task may use `<hint>`, `<solution>`, both, or neither.
- If both are used, `<hint>` blocks should be short inline answer chunks and `<solution>` should be the complete final procedure.
- Legacy `<answer>` is accepted as an alias, but new files must use `<solution>`.

`<collapse title="...">`

- Visible collapsible reference content.
- Use for optional background, examples, command output, tables, or supporting details that are not hidden answers.
- This is not a hint: do not use it for solution-only material that should stay hidden until requested.
- The body may contain normal supported Markdown.

`<quiz>` and `<question>`

- `<quiz>` contains one or more `<topic id="..." title="...">`.
- A quiz topic contains one or more `<question>`.
- Use `<question type="single">` for radio questions.
- Use `<question type="multiple">` for checkbox questions.
- Each question must contain one `<prompt>`, at least two `<option>` elements, and one `<explanation>`.
- Mark correct options with `correct="true"`.

`<option id="..." correct="true">`

- Keep option labels short and unambiguous.
- For `type="single"`, exactly one option must be correct.
- For `type="multiple"`, one or more options may be correct.

`<explanation>`

- Explain why the correct answer is correct.
- Explain common wrong-answer misconceptions when useful.
- Must be useful even when the learner answered correctly.

## Lab Rules

- Write concrete, verifiable tasks.
- Prefer imperative task titles: “Configure a static IP address”, “Create a Samba share”.
- Keep visible prompts challenge-focused.
- Put hidden inline answer chunks in `<hint>`.
- Put complete final procedures in `<solution>`.
- Do not write vague hidden content such as “configure it correctly”.
- Include exact commands, paths, file contents, verification steps, and short explanations when they are needed.
- If a command or config is the expected answer, hide it unless it is intentionally starter material.

Good inline answer:

````xml
<hint title="Inventory pattern"><![CDATA[
Use a `web` group in the inventory:

```ini
[web]
web1
web2
```
]]></hint>
````

Good separated solution:

````xml
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
````

## Quiz Rules

- Single-choice questions must have exactly one correct option.
- Multiple-choice questions must have at least one correct option.
- Do not use trick wording unless the topic requires distinguishing similar concepts.
- Avoid “all of the above” and “none of the above” unless they are truly meaningful.
- Every quiz question must include an explanation.

## Markdown and XML Rules

Supported Markdown inside text bodies:

- `#`, `##`, `###` headings
- unordered lists with `-` or `*`
- ordered lists like `1. Step`
- pipe tables with a header separator row
- inline code with backticks
- fenced code blocks with language tags such as `sh`, `yaml`, `go`, `python`, `xml`, `ini`

Table example:

```markdown
| Service | Port |
| --- | ---: |
| SSH | `22` |
| HTTPS | `443` |
```

Collapsible example:

````xml
<collapse title="Reference ports"><![CDATA[
| Service | Port |
| --- | ---: |
| SSH | `22` |
| HTTPS | `443` |
]]></collapse>
````

XML structural tags must remain real XML tags. Do not put `<hint>`, `<solution>`, `<task>`, `<question>`, or other Labbit tags inside CDATA.

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

Use CDATA inside content elements when Markdown contains XML-sensitive characters such as `<`, `>`, or `&`. Otherwise, escape those characters as `&lt;`, `&gt;`, and `&amp;`.

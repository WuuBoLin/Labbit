# Labbit

![Labbit rabbit icon](cmd/web/assets/img/social-card.png)

Labbit is a lightweight, high-performance viewer for lab exam notes. Upload a Labbit XML file and turn it into a fast, documentation-style workspace.
Use [LLM.md](LLM.md) to generate lab exams in Labbit XML format with AI agents.

> [!IMPORTANT]
> This project was heavily assisted by LLM. Please review the code carefully and use it with caution.

## Build

Requirements:

- Go
- `templ`
- Tailwind CSS standalone binary, or let `make build` download it
- SQLite support for Go (`github.com/mattn/go-sqlite3` uses CGO)

Build and test:

```sh
make
```

Run from source:

```sh
make run
```

Build the binary:

```sh
make build
./labbit --bind 0.0.0.0 --port 80 --db ./db/labbit.db
```

Useful commands:

```sh
make test
make watch
./labbit --help
```

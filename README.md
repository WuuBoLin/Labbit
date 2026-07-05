# Labbit

![Labbit rabbit icon](cmd/web/assets/img/social-card.png)

Labbit is a web viewer for lab exam notes. Upload a Labbit XML file and Labbit turns it into a documentation-style workspace with LABs and QUIZ.

> [!IMPORTANT]
> This project was heavily assisted by LLM. Please review the code carefully and use it with caution.

## Generate Labbit XML

Use [SKILL.md](cmd/web/assets/SKILL.md) when generating Labbit XML with ChatGPT, Claude, Gemini, or any AI agent.

Typical flow:

1. Give `SKILL.md` to the agent.
2. Ask it to generate a Labbit XML file for your lab, quiz, or combined workspace.
3. Upload the generated XML in Labbit.

## Usage

Run from source:

```sh
PUBLIC_URL=http://localhost:8080 make run
```

Run the built binary:

```sh
./labbit --bind 0.0.0.0 --port 8080 --db ./db/labbit.db --public-url http://localhost:8080
```

Show **help** or **version**:

```sh
./labbit --help
./labbit --version
```

### CLI flags:

Defaults are read from the process environment, including a local `.env` file when present. Explicit CLI flags override those defaults.

| Flag | Environment default | Description |
| --- | --- | --- |
| `-h`, `--help` | none | Print help and exit. |
| `--version` | none | Print the binary version and exit. |
| `-b`, `--bind` | `BIND`, default `0.0.0.0` | Address for the HTTP server to bind. |
| `-p`, `--port` | `PORT`, default `80` | HTTP port. Must be between `1` and `65535`. |
| `-d`, `--db` | `DB_URL`, default `./db/labbit.db` | SQLite database path or DSN. Parent directories are created automatically for normal file paths. |
| `--public-url` | `PUBLIC_URL` | Browser-facing base URL. Required for correct passkey origins and OIDC callback URLs outside local defaults. |
| `--disable-auth` | none | Disable passkeys, OIDC, sessions, onboarding, and auth-only routes. A local user is used instead. |
| `--log-format` | `APP_ENV` default, see below | Log format: `text` or `json`. There is no `LOG_FORMAT` environment variable. |
| `--log-level` | `LOG_LEVEL`, default `info` | Log level: `debug`, `info`, `warn`, `warning`, or `error`. |

### Environment variables:

| Variable | Default | Description |
| --- | --- | --- |
| `BIND` | `0.0.0.0` | Default bind address for `--bind`. |
| `PORT` | `80` | Default port for `--port`. Invalid values fall back to `80`. |
| `DB_URL` | `./db/labbit.db` | Default SQLite path or DSN for `--db`. The Docker image sets this to `/data/labbit.db`. |
| `PUBLIC_URL` | `http://localhost` for port `80`, otherwise `http://localhost:<port>` | Public origin used for passkeys and OIDC callbacks. Must include scheme and host when set. Use HTTPS in production. |
| `APP_ENV` | unset | When set to `local`, the default log format is `text`; otherwise it is `json`. |
| `LOG_LEVEL` | `info` | Default level for `--log-level`. |
| `OIDC_PROVIDERS` | unset | Comma-separated OIDC provider names, such as `github,google`. |
| `OIDC_<NAME>_DISPLAY_NAME` | provider name | Label shown on the sign-in button. |
| `OIDC_<NAME>_ISSUER_URL` | required for provider | OIDC issuer URL. |
| `OIDC_<NAME>_CLIENT_ID` | required for provider | OIDC client ID. |
| `OIDC_<NAME>_CLIENT_SECRET` | required for provider | OIDC client secret. |
| `OIDC_<NAME>_USERNAME_CLAIM` | `preferred_username` | Claim used as the initial username suggestion. |
| `HOST_PORT` | `8080` in Docker Compose | Host port mapped to container port `80` by `docker-compose.yml`. |

For provider names in `OIDC_PROVIDERS`, hyphens are converted to underscores and the name is uppercased for environment variable prefixes. For example, `OIDC_PROVIDERS=my-idp` uses `OIDC_MY_IDP_ISSUER_URL`, `OIDC_MY_IDP_CLIENT_ID`, and `OIDC_MY_IDP_CLIENT_SECRET`.

OIDC example:

```sh
OIDC_PROVIDERS=github
OIDC_GITHUB_DISPLAY_NAME=GitHub
OIDC_GITHUB_ISSUER_URL=https://github.example/issuer
OIDC_GITHUB_CLIENT_ID=...
OIDC_GITHUB_CLIENT_SECRET=...
OIDC_GITHUB_USERNAME_CLAIM=preferred_username
PUBLIC_URL=https://labbit.example.com
```

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
```

Other development commands:

```sh
make test
make watch
```

`make build` runs `templ generate`, builds Tailwind CSS into `cmd/web/assets/css/labbit.css`, and compiles the Linux binary as `./labbit`. It may download the standalone Tailwind binary. `templ` must be installed or accepted when prompted by the Makefile.

## Deploy

Build and run the container with a persistent SQLite volume:

```sh
docker build -t labbit .
docker run --rm \
  -p 8080:80 \
  -v labbit-data:/data \
  -e PUBLIC_URL=http://localhost:8080 \
  labbit
```

Docker Compose:

```sh
HOST_PORT=8080 PUBLIC_URL=http://localhost:8080 make docker-run
make docker-down
```

The container listens on port `80`, stores SQLite data at `/data/labbit.db`, and uses the `/data` volume for persistence. In production, set `PUBLIC_URL` to the HTTPS origin users open in their browser.

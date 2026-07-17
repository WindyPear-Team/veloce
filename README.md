Veloce

Your better personal agent and ai site

English | [简体中文](README_zh.md)

Veloce is an AI API gateway and marketplace designed for building AI platforms and developer ecosystems, providing a production-ready foundation for AI API management, authentication, billing, and upstream provider management.

## Features

- OpenAI-compatible API gateway
- Multiple upstream provider management
- OIDC authentication
- Passkey (WebAuthn) authentication
- API Key authentication
- User balance management
- Token usage logging
- Basic billing system
- Image generation support
- Modern administration dashboard

## Repository Structure

internal/    Internal code
cmd/         Cli module

## Building

Requirements

- Go (version specified in "go.mod")
- Node.js
- Yarn

1. Build the Frontend
```
cd web
yarn install
yarn build
```

> Tips: You should put your frontend code in ../web
2. Build the Backend
```
cd ../veloce
go build
```
Or run it directly during development:
```
go run .
```
After the frontend has been built, the backend will serve the generated frontend assets.

## Configuration

Copy ".env.example" to ".env" and configure your environment.

```
APP_ENV=development
PORT=8080
DB_DRIVER=sqlite
DB_PATH=veloce.db
DB_DSN=
DB_MAX_OPEN_CONNS=25
DB_MAX_IDLE_CONNS=10
DB_CONN_MAX_LIFETIME_SECONDS=3600
JWT_SECRET=your-secure-jwt-secret-here
OIDC_ISSUER=https://your-oidc-provider.com
OIDC_CLIENT_ID=your-client-id
OIDC_CLIENT_SECRET=your-client-secret
OIDC_REDIRECT_URL=http://localhost:8080/auth/callback
BOOTSTRAP_ADMIN_OIDC_SUBS=
BOOTSTRAP_ADMIN_EMAILS=
```

Enterprise features are enabled manually by selecting Enterprise Mode in the admin runtime-mode setting; no environment flag is required. One deployment represents one enterprise. Switching back to operation or personal mode preserves enterprise data while disabling enterprise APIs.

`DB_DRIVER` accepts `sqlite` (default), `postgres`, and `mysql`. SQLite uses
`DB_PATH`. For PostgreSQL or MySQL, set `DB_DSN` (or `DATABASE_URL`) and the
application will create or migrate its schema at startup.

```dotenv
# PostgreSQL
DB_DRIVER=postgres
DB_DSN=host=127.0.0.1 user=flai password=change-me dbname=flai port=5432 sslmode=disable

# MySQL 8+
DB_DRIVER=mysql
DB_DSN=flai:change-me@tcp(127.0.0.1:3306)/flai?charset=utf8mb4&parseTime=True&loc=Local
```

### Migrate SQLite to PostgreSQL or MySQL

Keep `DB_DRIVER=sqlite` and point `DB_PATH` at the source file. Set `DB_DSN`
to an empty PostgreSQL or MySQL database, then run the one-way migration:

```bash
go run . --migrate
```

The source SQLite file is read only. The command refuses a target that already
contains application tables, copies data in batches, and exits when complete.
Afterward, switch `DB_DRIVER` to `postgres` or `mysql` for normal startup.

## License

We use AGPL. See the LICENSE file for licensing information.

## Special thanks

[Linuxdo](https://linux.do)

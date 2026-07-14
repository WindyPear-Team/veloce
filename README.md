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
ENTERPRISE_FEATURES_ENABLED=false
```

`ENTERPRISE_FEATURES_ENABLED` is the master switch for single-enterprise private-deployment organization, workspace, and governance features. One deployment represents one enterprise. It defaults to `false` so existing personal and community deployments keep their current behavior.

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

## License

We use AGPL. See the LICENSE file for licensing information.

## Special thanks

[Linuxdo](https://linux.do)

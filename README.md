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
DB_PATH=veloce.db
JWT_SECRET=your-secure-jwt-secret-here
OIDC_ISSUER=https://your-oidc-provider.com
OIDC_CLIENT_ID=your-client-id
OIDC_CLIENT_SECRET=your-client-secret
OIDC_REDIRECT_URL=http://localhost:8080/auth/callback
BOOTSTRAP_ADMIN_OIDC_SUBS=
BOOTSTRAP_ADMIN_EMAILS=
```

## License

We use AGPL. See the LICENSE file for licensing information.

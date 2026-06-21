WindyPear Token Market

Community Edition

English | [简体中文](README_szh.md)

WindyPear Token Market (Token Market) is an AI token marketplace and API gateway designed for building AI platforms and developer ecosystems.

This repository contains the Community Edition of WindyPear Token Market, providing a production-ready foundation for AI API management, authentication, billing, and upstream provider management.

Features

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

Repository Structure

community/    Community Edition backend
web/          Frontend

Frontend

The frontend source code is located in the "web" directory.

Community Edition

This repository contains the Community Edition of WindyPear Token Market.

For enterprise features, commercial support, and additional capabilities, you can purchase the Professional Edition at:

https://project.flweb.cn/tokenmarket

Building

Requirements

- Go (version specified in "go.mod")
- Node.js
- Yarn

1. Build the Frontend

cd web
yarn install
yarn build

2. Build the Community Edition Backend

cd ../community
go build

Or run it directly during development:

go run .

After the frontend has been built, the backend will serve the generated frontend assets.

Configuration

Copy ".env.example" to ".env" and configure your environment.

APP_ENV=development
PORT=8080
DB_PATH=token-market.db
JWT_SECRET=your-secure-jwt-secret-here
OIDC_ISSUER=https://your-oidc-provider.com
OIDC_CLIENT_ID=your-client-id
OIDC_CLIENT_SECRET=your-client-secret
OIDC_REDIRECT_URL=http://localhost:8080/auth/callback
BOOTSTRAP_ADMIN_OIDC_SUBS=
BOOTSTRAP_ADMIN_EMAILS=

License

See the LICENSE file for licensing information.
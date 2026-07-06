Veloce

社区版

[English](README.md) | 简体中文

Veloce 是一款面向 AI 平台与开发者生态打造的 AI API 网关与服务市场。

本仓库包含 Veloce Community Edition（社区版），提供完整的 AI API 管理基础能力，包括身份认证、上游渠道管理、API 网关、用户余额、计费、调用日志等功能。

功能特性

- OpenAI 兼容 API 网关
- 多上游渠道管理
- OIDC 登录认证
- Passkey（WebAuthn）认证
- API Key 鉴权
- 用户余额管理
- Token 用量统计
- 基础计费系统
- 图片生成支持
- 现代化 Web 管理后台

仓库结构

community/    社区版后端
web/          前端

前端

前端源码位于 "web" 目录。

社区版

本仓库提供 Veloce Community Edition（社区版）。

如果需要更多高级功能、商业支持或企业级能力，可以购买 Professional Edition（专业版）：

https://tk.flweb.cn/buy

构建

环境要求

- Go（版本以 "go.mod" 为准）
- Node.js
- Yarn

1. 构建前端

cd web
yarn install
yarn build

2. 构建社区版后端

cd ../community
go build

开发时可直接运行：

go run .

完成前端构建后，后端会自动提供构建好的前端静态资源。

配置

将 ".env.example" 复制为 ".env"，并根据实际环境修改配置：

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

许可证

本项目采用仓库中 "LICENSE" 文件所规定的许可证。

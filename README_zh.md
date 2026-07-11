Veloce

你的新一代个人助理与AI中转站点

[English](README.md) | 简体中文

Veloce 是一款面向 AI 平台与开发者生态打造的 AI API 网关与服务市场。提供完整的 AI API 管理基础能力，包括身份认证、上游渠道管理、API 网关、用户余额、计费、调用日志等功能。

## 功能特性

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

## 仓库结构

internal/    内部代码
cmd/         Cli组件

## 构建

环境要求

- Go（版本以 "go.mod" 为准）
- Node.js
- Yarn

1. 构建前端
```
cd web
yarn install
yarn build
```
> 提示：前端需要放在相对后端的../web位置
2. 构建后端
```
cd ../community
go build
```
开发时可直接运行：
```
go run .
```
完成前端构建后，后端会自动提供构建好的前端静态资源。

## 配置

将 ".env.example" 复制为 ".env"，并根据实际环境修改配置：
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
## 许可证

本项目采用AGPL许可证，详情请查看仓库中 "LICENSE" 文件。

## 特别鸣谢

[Linuxdo](https://linux.do)

[English](/README.md) | [فارسی](/README.fa_IR.md) | [العربية](/README.ar_EG.md) | [中文](/README.zh_CN.md) | [Español](/README.es_ES.md) | [Русский](/README.ru_RU.md) | [Türkçe](/README.tr_TR.md)

# 3X-UI — RBAC 分支 (admin6501)

[MHSanaei/3X-UI](https://github.com/MHSanaei/3x-ui) 的定制分支——一个用于管理 [Xray-core](https://github.com/XTLS/Xray-core) 服务器的先进开源 Web 面板——扩展了内置的**多管理员 RBAC 系统**、**分销商（Reseller）范围限制**，以及面向隔离网/受限服务器的**完全离线安装程序**。

> [!IMPORTANT]
> 本项目仅供个人使用。请勿用于非法用途或生产环境。

## 本分支新增内容

- **基于角色的访问控制（RBAC）**——在**管理员**页面管理多个面板管理员，每人拥有以下四种角色之一：
  - `super_admin`——对一切拥有完全访问权（首个/默认管理员）。
  - `manager`——管理入站与客户端；**无**面板设置、Xray 模板或管理员管理权限。
  - `reseller`（分销商）——仅限于分配给他的入站；管理自己的客户端，并以**只读**方式查看其入站（无法新增/编辑/启停/删除入站）。
  - `readonly`——可查看一切，但不能执行任何写操作。
- **审计日志（Audit Log）**——每个管理员操作（新增/修改/删除管理员、重置密码……）都会记录操作者、目标与时间。
- **离线安装包**——通过自包含的 tarball（面板二进制 + Xray-core + geo 数据）在无网络的服务器上安装。见 [`offline/`](offline/)。
- **面向分支的更新检查**——面板的"检查更新"读取本分支（`admin6501/3x-ui`）的发行版。
- **PostgreSQL 安全迁移**——SQLite → PostgreSQL 迁移会复制全部 RBAC 数据（管理员、角色、允许的入站、审计日志）。

## 核心功能（来自 3X-UI）

- **多协议入站**——VLESS、VMess、Trojan、Shadowsocks、WireGuard、Hysteria2、HTTP、SOCKS (Mixed)、Dokodemo-door / Tunnel 与 TUN。
- **现代传输与安全**——TCP (Raw)、mKCP、WebSocket、gRPC、HTTPUpgrade 与 XHTTP，配合 TLS、XTLS 与 REALITY。
- **按客户端管理**——流量配额、到期时间、IP 限制、实时在线状态、分享链接、二维码与订阅。
- **多节点支持**、出站与路由（WARP、自定义规则、负载均衡）。
- **内置订阅服务器**，支持[自定义页面模板](docs/custom-subscription-templates.md)。
- **Telegram 机器人**、带面板内 Swagger 的 **RESTful API**、**SQLite 或 PostgreSQL**、**13 种界面语言**、深/浅色主题，以及 **Fail2ban** IP 限制强制执行。

## 快速开始（在线）

```bash
bash <(curl -Ls https://raw.githubusercontent.com/admin6501/3x-ui/main/install.sh)
```

安装期间会生成随机的用户名、密码与 Web 路径。之后运行 `x-ui` 打开管理菜单（启动/停止、重置凭据、管理 SSL 等）。

## 离线安装（无网络）

适用于无法访问互联网——或 GitHub 下载被屏蔽——的服务器：

1. 将 `offline/x-ui-linux-amd64.tar.gz` 与 `install_offline.sh` 复制到服务器（任意目录）。
2. 运行：

```bash
chmod +x install_offline.sh
sudo ./install_offline.sh                 # 自动检测当前目录中的 .tar.gz
# 或显式传入：
sudo ./install_offline.sh /root/x-ui-linux-amd64.tar.gz
```

安装程序对该安装包**不进行任何网络请求**（二进制 + Xray + geo 全部来自 tarball），会打印生成的凭据**以及 API 令牌**，并在升级时**保留现有的 `/etc/x-ui/x-ui.db`** 同时执行迁移（含 RBAC 表）。详见 [`offline/README.md`](offline/README.md)。

> 预编译离线包**仅支持 amd64 / x86_64**。其他架构请从源码为该架构构建。

## 支持的平台

**操作系统：** Ubuntu、Debian、Armbian、Fedora、CentOS、RHEL、AlmaLinux、Rocky Linux、Oracle Linux、Amazon Linux、Arch、Manjaro、openSUSE、Alpine 与 Windows。

**架构：** `amd64` · `arm64` (aarch64) · `armv7` · `armv6` · `386` · `s390x`。

## 数据库

- **SQLite**（默认）——位于 `/etc/x-ui/x-ui.db` 的单个文件。无需配置，适合中小型部署。
- **PostgreSQL**——推荐用于大量客户端或多节点部署。

将现有 SQLite 安装迁移到 PostgreSQL（含全部 RBAC 数据）：

```bash
x-ui migrate-db --dsn "postgres://xui:password@127.0.0.1:5432/xui?sslmode=disable"
# 然后在 /etc/default/x-ui 中设置 XUI_DB_TYPE 与 XUI_DB_DSN 并重启：
systemctl restart x-ui
```

源 SQLite 文件保持不变；确认新后端无误后再手动删除它。

## 环境变量

| 变量 | 说明 | 默认值 |
| --- | --- | --- |
| `XUI_DB_TYPE` | 数据库后端：`sqlite` 或 `postgres` | `sqlite` |
| `XUI_DB_DSN` | PostgreSQL 连接串（当 `XUI_DB_TYPE=postgres` 时） | — |
| `XUI_DB_FOLDER` | SQLite 数据库文件目录 | `/etc/x-ui` |
| `XUI_INIT_WEB_BASE_PATH` | Web 面板初始 URI 路径 | `/` |
| `XUI_ENABLE_FAIL2BAN` | 启用基于 Fail2ban 的 IP 限制 | `true` |
| `XUI_LOG_LEVEL` | 日志级别（`debug`、`info`、`warning`、`error`） | `info` |

## 支持的语言

面板界面提供 13 种语言：

English · فارسی · العربية · 中文（简体） · 中文（繁體） · Español · Русский · Українська · Türkçe · Tiếng Việt · 日本語 · Bahasa Indonesia · Português (Brasil)

## 文档

- [获取真实客户端 IP](docs/real-client-ip.md)（位于 Cloudflare / L4 中继之后）。
- [自定义订阅模板](docs/custom-subscription-templates.md)。

## 致谢与许可

本项目是 **[MHSanaei/3X-UI](https://github.com/MHSanaei/3x-ui)** 的分支，并基于 [Xray-core](https://github.com/XTLS/Xray-core) 以及 [alireza0](https://github.com/alireza0/) 的原始 X-UI 构建。地理路由规则：[Iran v2ray rules](https://github.com/chocolate4u/Iran-v2ray-rules) 与 [Russia v2ray rules](https://github.com/runetfreedom/russia-v2ray-rules-dat)。

依据 **GPL-3.0** 许可，与上游项目相同。

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

## 分销商（reseller）流量分配

为每个**分销商**设置总流量额度，面板会自动执行：

- 在 **Admins** 页面为每个分销商设置**流量配额**（GB）——`0` 表示无限制。
- 用量按**该分销商所有已分配入站的上行 + 下行之和**计算。
- 当分销商达到配额时，流量任务会**自动禁用其所有已分配入站**，立即切断其客户端。
- 以此方式被禁用的入站会被记录，并在你**提高或重置**分销商配额后**自动重新启用**。
- **允许超售**——只按真实用量计算——每个分销商可在分销商面板中查看自己的用量与配额。

## 核心功能（来自 3X-UI）

- **多协议入站**——VLESS、VMess、Trojan、Shadowsocks、WireGuard、Hysteria2、HTTP、SOCKS (Mixed)、Dokodemo-door / Tunnel 与 TUN。
- **现代传输与安全**——TCP (Raw)、mKCP、WebSocket、gRPC、HTTPUpgrade 与 XHTTP，配合 TLS、XTLS 与 REALITY。
- **按客户端管理**——流量配额、到期时间、IP 限制、实时在线状态、分享链接、二维码与订阅。
- **多节点支持**、出站与路由（WARP、自定义规则、负载均衡）。
- **内置订阅服务器**，支持[自定义页面模板](docs/custom-subscription-templates.md)。
- **Telegram 机器人**、带面板内 Swagger 的 **RESTful API**、**SQLite 或 PostgreSQL**、**13 种界面语言**、深/浅色主题，以及 **Fail2ban** IP 限制强制执行。

## 面板功能与选项（本分支）

除上述内容外，面板还包括：

- **套餐 / 计划（Plans）** — 可复用的客户端模板：一键预填流量、到期时间和 IP 限制。
- **客户端分组（Groups）** — 将多个入站捆绑，使单个订阅可一次提供多个配置。
- **主机（Hosts）** — 为每个入站发布自定义订阅端点（地址、端口、备注、SNI）。
- **可修改入站协议** — 现有入站可切换到其他协议；其客户端会被保留（邮箱、流量额度、到期时间、订阅 ID），仅重新生成凭据，并在保存前提示此前分享的链接将失效。
- **订阅备注模板** — 使用 `{{VAR}}` 变量（`EMAIL`、`INBOUND`、`TRAFFIC_LEFT`、`DAYS_LEFT`、`HOST` 等）生成客户端链接名称；清空该字段会恢复默认模板。
- **额外的订阅格式** — 在 base64 订阅之外提供 JSON 和 Clash 输出（可选路由规则）。
- **两步验证（2FA / TOTP）** 用于管理员登录。
- **LDAP / Active Directory 登录** — 通过目录服务对面板管理员进行认证。
- **邮件（SMTP）通知** — 基于事件的提醒，支持 STARTTLS/TLS 和测试邮件按钮。
- **外部流量 Webhook** — 将客户端流量事件推送到外部 URL。
- **MTProto（Telegram）代理** 支持，通过内置的 `mtg` 组件。
- **波斯历（Jalali）/ 公历** 可为所有日期切换。
- **面板内教程页面**（仅 super_admin）— 覆盖各面板板块的多语言快速指南。
- **设备（HWID）限制** — 通过 Happ/Hiddify 类应用发送的 `x-hwid` 请求头，限制可获取某客户端订阅的设备数量。支持面板默认值与逐客户端覆盖、可清空的设备列表，以及拒绝不发送设备标识的严格模式。
- **删除到期客户端** — 从入站行菜单清理该入站下已结束的客户端，或让面板在到期若干天后自动清除（默认关闭；0 天表示永不删除）。
- **订阅页品牌外观** — 在面板内构建用户在浏览器打开的页面：品牌名、副标题、标志、公告、配色、背景、显示板块、客服/Telegram/官网按钮与自定义 CSS，并带实时预览。
- **分销销售机器人** — 第二个 Telegram 机器人（独立的令牌与管理员列表，在面板设置 → Telegram → 销售机器人中开启），以波斯语销售分销面板。套餐可在面板或机器人中定义——价格、流量配额、客户名额、授予的入站；买家浏览后上传付款凭证，你审核通过后面板即创建其分销账号并发送登录信息。再次购买会为同一账号充值而非另开账号，充值还会唤醒因配额耗尽而停用的账号。机器人同时具备管理端：待审订单、套餐管理、分销商列表（启停与流量重置）、销售统计与群发。面板的**分销销售**页面提供同样的能力，并可记录在 Telegram 之外完成的交易。
- **自定义角色** — 在**管理员**页面自建角色：起个名字，勾选它应有的权限（查看/管理入站、客户端、套餐、分组、主机、节点、面板设置、Xray 配置、管理员管理），然后像内置角色一样分配。勾选*限定于所分配的入站*即可让它像分销商一样工作，配额同样生效。仍被管理员使用的角色无法删除。
- **分销商仪表盘** — 分销商登录后直接进入自己的仪表盘：流量与客户端名额的配额进度条、按状态分类的客户端数量（活跃、在线、即将到期、已结束）、各入站的用量明细、未来一周内到期的客户端，以及最近创建的客户端。
- **停用管理员会一并关闭其入站** — 停用分销商账号时，分配给它的入站也会关闭，让其客户真正断开，而不只是禁止其本人登录。重新启用会精确恢复被关闭的那些，绝不触碰你手动停用的入站。
- **强化安全** — CSRF 防护、安全响应头和可配置的会话时长。

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

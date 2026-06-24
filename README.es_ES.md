[English](/README.md) | [فارسی](/README.fa_IR.md) | [العربية](/README.ar_EG.md) | [中文](/README.zh_CN.md) | [Español](/README.es_ES.md) | [Русский](/README.ru_RU.md) | [Türkçe](/README.tr_TR.md)

# 3X-UI — Fork con RBAC (admin6501)

Un fork personalizado de [MHSanaei/3X-UI](https://github.com/MHSanaei/3x-ui) — un panel web avanzado y de código abierto para gestionar servidores [Xray-core](https://github.com/XTLS/Xray-core) — ampliado con un **sistema multi-administrador basado en roles (RBAC)**, **alcance para revendedores (Reseller)** y un **instalador totalmente sin conexión** para servidores aislados o restringidos.

> [!IMPORTANT]
> Este proyecto es solo para uso personal. No lo utilices con fines ilegales ni en un entorno de producción.

## Qué añade este fork

- **Control de acceso basado en roles (RBAC)** — gestiona varios administradores del panel desde la página **Administradores**, cada uno con uno de cuatro roles:
  - `super_admin` — acceso total a todo (el primer administrador / predeterminado).
  - `manager` — gestiona entradas y clientes; **sin** ajustes del panel, plantilla de Xray ni gestión de administradores.
  - `reseller` (revendedor) — limitado solo a las entradas asignadas; gestiona sus propios clientes y **visualiza** sus entradas en solo lectura (no puede añadir/editar/activar-desactivar/eliminar entradas).
  - `readonly` — puede ver todo pero no realizar ninguna acción de escritura.
- **Registro de auditoría (Audit Log)** — cada acción de administrador (crear/editar/eliminar administrador, restablecer contraseña, …) se registra con actor, objetivo y marca de tiempo.
- **Paquete de instalación sin conexión** — instala en un servidor sin internet mediante un tarball autónomo (binario del panel + Xray-core + datos geo). Ver [`offline/`](offline/).
- **Comprobador de actualizaciones del fork** — la opción "buscar actualización" del panel lee las versiones de este fork (`admin6501/3x-ui`).
- **Migración segura a PostgreSQL** — la migración SQLite → PostgreSQL copia todos los datos RBAC (administradores, roles, entradas permitidas, registros de auditoría).

## Funciones principales (de 3X-UI)

- **Entradas multiprotocolo** — VLESS, VMess, Trojan, Shadowsocks, WireGuard, Hysteria2, HTTP, SOCKS (Mixed), Dokodemo-door / Tunnel y TUN.
- **Transportes y seguridad modernos** — TCP (Raw), mKCP, WebSocket, gRPC, HTTPUpgrade y XHTTP, protegidos con TLS, XTLS y REALITY.
- **Gestión por cliente** — cuotas de tráfico, fechas de expiración, límites de IP, estado en línea, enlaces de compartición, códigos QR y suscripciones.
- **Soporte multinodo**, salidas y enrutamiento (WARP, reglas personalizadas, balanceadores de carga).
- **Servidor de suscripción integrado** con [plantillas de página personalizadas](docs/custom-subscription-templates.md).
- **Bot de Telegram**, **API RESTful** con Swagger integrado, **SQLite o PostgreSQL**, **13 idiomas de interfaz**, temas claro/oscuro y aplicación de límites de IP con **Fail2ban**.

## Inicio rápido (en línea)

```bash
bash <(curl -Ls https://raw.githubusercontent.com/admin6501/3x-ui/main/install.sh)
```

Durante la instalación se generan un usuario, una contraseña y una ruta web aleatorios. Luego ejecuta `x-ui` para abrir el menú de administración (iniciar/detener, restablecer credenciales, gestionar SSL, etc.).

## Instalación sin conexión (sin internet)

Para servidores sin acceso a internet — o donde la descarga de GitHub está bloqueada:

1. Copia `offline/x-ui-linux-amd64.tar.gz` e `install_offline.sh` al servidor (cualquier carpeta).
2. Ejecuta:

```bash
chmod +x install_offline.sh
sudo ./install_offline.sh                 # detecta automáticamente el .tar.gz del directorio actual
# o pásalo explícitamente:
sudo ./install_offline.sh /root/x-ui-linux-amd64.tar.gz
```

El instalador **no realiza ninguna llamada de red** para el paquete (binario + Xray + geo vienen del tarball), imprime las credenciales generadas **y el token de la API**, y al actualizar **conserva tu `/etc/x-ui/x-ui.db` existente** mientras ejecuta las migraciones (incluidas las tablas RBAC). Ver [`offline/README.md`](offline/README.md) para más detalles.

> El paquete sin conexión precompilado es **solo amd64 / x86_64**. Para otras arquitecturas, compila un paquete para esa arquitectura desde el código fuente.

## Plataformas compatibles

**Sistemas operativos:** Ubuntu, Debian, Armbian, Fedora, CentOS, RHEL, AlmaLinux, Rocky Linux, Oracle Linux, Amazon Linux, Arch, Manjaro, openSUSE, Alpine y Windows.

**Arquitecturas:** `amd64` · `arm64` (aarch64) · `armv7` · `armv6` · `386` · `s390x`.

## Base de datos

- **SQLite** (predeterminado) — un único archivo en `/etc/x-ui/x-ui.db`. Sin configuración, ideal para despliegues pequeños/medianos.
- **PostgreSQL** — recomendado para gran número de clientes o configuraciones multinodo.

Migrar una instalación SQLite existente a PostgreSQL (con todos los datos RBAC):

```bash
x-ui migrate-db --dsn "postgres://xui:password@127.0.0.1:5432/xui?sslmode=disable"
# luego define XUI_DB_TYPE y XUI_DB_DSN en /etc/default/x-ui y reinicia:
systemctl restart x-ui
```

El archivo SQLite original permanece intacto; elimínalo una vez verificado el nuevo backend.

## Variables de entorno

| Variable | Descripción | Predeterminado |
| --- | --- | --- |
| `XUI_DB_TYPE` | Backend de base de datos: `sqlite` o `postgres` | `sqlite` |
| `XUI_DB_DSN` | Cadena de conexión PostgreSQL (cuando `XUI_DB_TYPE=postgres`) | — |
| `XUI_DB_FOLDER` | Directorio del archivo de base de datos SQLite | `/etc/x-ui` |
| `XUI_INIT_WEB_BASE_PATH` | Ruta URI inicial del panel web | `/` |
| `XUI_ENABLE_FAIL2BAN` | Activar límites de IP mediante Fail2ban | `true` |
| `XUI_LOG_LEVEL` | Nivel de registro (`debug`, `info`, `warning`, `error`) | `info` |

## Idiomas compatibles

La interfaz del panel está disponible en 13 idiomas:

English · فارسی · العربية · 中文（简体） · 中文（繁體） · Español · Русский · Українська · Türkçe · Tiếng Việt · 日本語 · Bahasa Indonesia · Português (Brasil)

## Documentación

- [Capturar la IP real del cliente](docs/real-client-ip.md) (tras Cloudflare / relés L4).
- [Plantillas de suscripción personalizadas](docs/custom-subscription-templates.md).

## Créditos y licencia

Este proyecto es un fork de **[MHSanaei/3X-UI](https://github.com/MHSanaei/3x-ui)** y se basa en [Xray-core](https://github.com/XTLS/Xray-core) y el X-UI original de [alireza0](https://github.com/alireza0/). Reglas de enrutamiento geográfico: [Iran v2ray rules](https://github.com/chocolate4u/Iran-v2ray-rules) y [Russia v2ray rules](https://github.com/runetfreedom/russia-v2ray-rules-dat).

Licenciado bajo **GPL-3.0**, igual que el proyecto original.

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

## Asignación de tráfico por revendedor (reseller)

Da a cada **revendedor** un presupuesto total de tráfico y deja que el panel lo aplique automáticamente:

- Define una **cuota de tráfico** por revendedor (en GB) en la página **Admins** — `0` significa ilimitado.
- El consumo se mide como la **suma de subida + bajada de todas las entradas asignadas a ese revendedor**.
- Cuando un revendedor alcanza su cuota, la tarea de tráfico **desactiva automáticamente todas sus entradas asignadas**, cortando al instante a sus clientes.
- Las entradas desactivadas de este modo se registran y se **reactivan automáticamente** en cuanto subes o restableces la cuota del revendedor.
- Se **permite la sobreventa** — solo cuenta el consumo real — y cada revendedor ve su consumo frente a su cuota en el panel de revendedor.

## Funciones principales (de 3X-UI)

- **Entradas multiprotocolo** — VLESS, VMess, Trojan, Shadowsocks, WireGuard, Hysteria2, HTTP, SOCKS (Mixed), Dokodemo-door / Tunnel y TUN.
- **Transportes y seguridad modernos** — TCP (Raw), mKCP, WebSocket, gRPC, HTTPUpgrade y XHTTP, protegidos con TLS, XTLS y REALITY.
- **Gestión por cliente** — cuotas de tráfico, fechas de expiración, límites de IP, estado en línea, enlaces de compartición, códigos QR y suscripciones.
- **Soporte multinodo**, salidas y enrutamiento (WARP, reglas personalizadas, balanceadores de carga).
- **Servidor de suscripción integrado** con [plantillas de página personalizadas](docs/custom-subscription-templates.md).
- **Bot de Telegram**, **API RESTful** con Swagger integrado, **SQLite o PostgreSQL**, **13 idiomas de interfaz**, temas claro/oscuro y aplicación de límites de IP con **Fail2ban**.

## Funciones y opciones del panel (este fork)

Además de lo anterior, el panel también incluye:

- **Planes / Paquetes** — plantillas de cliente reutilizables: rellena tráfico, caducidad y límite de IP con un clic.
- **Grupos de clientes** — agrupa varias entradas para que una sola suscripción entregue varias configuraciones.
- **Hosts** — publica endpoints de suscripción personalizados (dirección, puerto, remark, SNI) por entrada.
- **Protocolo editable en la entrada** — cambia el protocolo de una entrada existente; sus clientes se conservan (correo, cuota, caducidad, id de suscripción) y solo se regeneran sus credenciales, tras avisar de que los enlaces ya compartidos dejarán de funcionar.
- **Plantilla de Remark de suscripción** — crea los nombres de los enlaces con tokens `{{VAR}}` (`EMAIL`, `INBOUND`, `TRAFFIC_LEFT`, `DAYS_LEFT`, `HOST`, …); vaciar el campo restaura la plantilla por defecto.
- **Formatos de suscripción adicionales** — salidas JSON y Clash (con reglas de enrutamiento opcionales) junto a la suscripción base64.
- **Autenticación de dos factores (2FA / TOTP)** para el inicio de sesión de administradores.
- **Inicio de sesión con LDAP / Active Directory** — autentica a los administradores contra un directorio.
- **Notificaciones por correo (SMTP)** — alertas basadas en eventos con STARTTLS/TLS y botón de correo de prueba.
- **Webhook de tráfico externo** — envía eventos de tráfico de clientes a una URL externa.
- **Soporte de proxy MTProto (Telegram)** mediante el sidecar `mtg` incluido.
- **Calendario Jalali (persa) / Gregoriano** conmutable para todas las fechas.
- **Página de Tutoriales en el panel** (solo super_admin) — una guía rápida multilingüe de cada sección del panel.
- **Límite de dispositivos (HWID)** — limita cuántos dispositivos pueden descargar la suscripción de un cliente mediante la cabecera `x-hwid` que envían apps tipo Happ/Hiddify. Valor por defecto del panel con anulación por cliente, lista de dispositivos que puedes vaciar y un modo estricto opcional que rechaza apps sin identificador.
- **Eliminar clientes caducados** — limpia los clientes terminados de una entrada desde el menú de su fila, o deja que el panel los borre automáticamente pasados X días desde su caducidad (desactivado por defecto; 0 días no borra nunca).
- **Marca de la página de suscripción** — construye desde el panel la página que tus usuarios abren en el navegador: nombre, lema, logotipo, aviso, colores, fondo, secciones visibles, botones de soporte/Telegram/web y CSS propio, con vista previa en vivo.
- **Seguridad reforzada** — protección CSRF, cabeceras de seguridad y duración de sesión configurable.

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

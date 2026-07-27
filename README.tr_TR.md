[English](/README.md) | [فارسی](/README.fa_IR.md) | [العربية](/README.ar_EG.md) | [中文](/README.zh_CN.md) | [Español](/README.es_ES.md) | [Русский](/README.ru_RU.md) | [Türkçe](/README.tr_TR.md)

# 3X-UI — RBAC Fork'u (admin6501)

[MHSanaei/3X-UI](https://github.com/MHSanaei/3x-ui) projesinin özelleştirilmiş bir fork'u — [Xray-core](https://github.com/XTLS/Xray-core) sunucularını yönetmek için gelişmiş, açık kaynaklı bir web paneli — yerleşik bir **çok yöneticili RBAC sistemi**, **bayi (Reseller) kapsam sınırlaması** ve internetsiz/kısıtlı sunucular için **tamamen çevrimdışı kurulum aracı** ile genişletilmiştir.

> [!IMPORTANT]
> Bu proje yalnızca kişisel kullanım içindir. Lütfen yasa dışı amaçlarla veya bir üretim ortamında kullanmayın.

## Bu fork'un eklediği özellikler

- **Rol Tabanlı Erişim Denetimi (RBAC)** — **Yöneticiler** sayfasından, her biri dört rolden birine sahip birden çok panel yöneticisini yönetin:
  - `super_admin` — her şeye tam erişim (ilk/varsayılan yönetici).
  - `manager` — gelenleri ve istemcileri yönetir; panel ayarları, Xray şablonu veya yönetici yönetimi **yoktur**.
  - `reseller` (bayi) — yalnızca atanan gelenlerle sınırlıdır; kendi istemcilerini yönetir ve gelenlerini salt okunur olarak **görüntüler** (gelen ekleyemez/düzenleyemez/etkin-pasif yapamaz/silemez).
  - `readonly` — her şeyi görüntüleyebilir ancak hiçbir yazma işlemi yapamaz.
- **Denetim günlüğü (Audit Log)** — her yönetici işlemi (yönetici oluşturma/düzenleme/silme, parola sıfırlama, …) işlemi yapan, hedef ve zaman damgasıyla kaydedilir.
- **Çevrimdışı kurulum paketi** — kendi kendine yeten bir tarball (panel ikilisi + Xray-core + geo verileri) ile internetsiz bir sunucuya kurulum. Bkz. [`offline/`](offline/).
- **Fork'a duyarlı güncelleyici** — panelin "güncellemeyi denetle" özelliği sürümleri bu fork'tan (`admin6501/3x-ui`) okur.
- **PostgreSQL güvenli geçiş** — SQLite → PostgreSQL geçişi tüm RBAC verilerini (yöneticiler, roller, izin verilen gelenler, denetim günlükleri) kopyalar.

## Bayi (reseller) trafik tahsisi

Her **bayiye** toplam bir trafik bütçesi verin ve panel bunu otomatik olarak uygulasın:

- **Admins** sayfasında bayi başına bir **trafik kotası** (GB) belirleyin — `0` sınırsız demektir.
- Tüketim, **o bayiye atanmış tüm gelenlerin yükleme + indirme toplamı** olarak ölçülür.
- Bir bayi kotasına ulaştığında, trafik görevi **atanmış tüm gelenlerini otomatik olarak devre dışı bırakır** ve müşterilerinin bağlantısını anında keser.
- Bu şekilde devre dışı bırakılan gelenler izlenir ve bayinin kotasını yükselttiğinizde veya sıfırladığınızda **otomatik olarak yeniden etkinleştirilir**.
- **Fazla satış (over-selling) serbesttir** — yalnızca gerçek tüketim sayılır — ve her bayi kendi tüketimini ve kotasını Bayi panosunda görür.

## Temel özellikler (3X-UI'den)

- **Çok protokollü gelenler** — VLESS, VMess, Trojan, Shadowsocks, WireGuard, Hysteria2, HTTP, SOCKS (Mixed), Dokodemo-door / Tunnel ve TUN.
- **Modern taşıma ve güvenlik** — TCP (Raw), mKCP, WebSocket, gRPC, HTTPUpgrade ve XHTTP; TLS, XTLS ve REALITY ile güvenli.
- **İstemci bazında yönetim** — trafik kotaları, son kullanma tarihleri, IP limitleri, canlı çevrimiçi durumu, paylaşım bağlantıları, QR kodları ve abonelikler.
- **Çok düğümlü (Multi-node) destek**, giden ve yönlendirme (WARP, özel kurallar, yük dengeleyiciler).
- **Yerleşik abonelik sunucusu** ve [özel sayfa şablonları](docs/custom-subscription-templates.md).
- **Telegram botu**, panel içi Swagger ile **RESTful API**, **SQLite veya PostgreSQL**, **13 arayüz dili**, koyu/açık temalar ve **Fail2ban** ile IP limiti uygulaması.

## Panel özellikleri ve seçenekleri (bu fork)

Yukarıdakilere ek olarak panel şunları da içerir:

- **Planlar / Paketler** — yeniden kullanılabilir istemci şablonları: trafik, bitiş tarihi ve IP limitini tek tıkla doldurur.
- **İstemci Grupları** — tek bir aboneliğin birden çok yapılandırma sunması için birkaç geleni gruplar.
- **Ana Bilgisayarlar (Hosts)** — her gelen için özel abonelik uç noktaları (adres, port, remark, SNI) yayınlar.
- **Gelen protokolü değiştirilebilir** — mevcut bir gelen bağlantının protokolü değiştirilebilir; istemcileri korunur (e-posta, kota, bitiş tarihi, abonelik kimliği) ve yalnızca kimlik bilgileri yeniden oluşturulur; kaydetmeden önce daha önce paylaşılan bağlantıların çalışmayacağı uyarısı gösterilir.
- **Abonelik Remark Şablonu** — istemci bağlantı adlarını `{{VAR}}` belirteçlerinden (`EMAIL`, `INBOUND`, `TRAFFIC_LEFT`, `DAYS_LEFT`, `HOST`, …) oluşturur; alan boşaltıldığında varsayılan şablon geri gelir.
- **Ek abonelik biçimleri** — base64 aboneliğinin yanında JSON ve Clash çıktıları (isteğe bağlı yönlendirme kurallarıyla).
- **İki Adımlı Doğrulama (2FA / TOTP)** yönetici girişleri için.
- **LDAP / Active Directory ile giriş** — panel yöneticilerini bir dizine karşı doğrular.
- **E-posta (SMTP) bildirimleri** — STARTTLS/TLS ve test e-postası düğmesiyle olay tabanlı uyarılar.
- **Harici trafik webhook'u** — istemci trafik olaylarını harici bir URL'ye gönderir.
- **MTProto (Telegram) proxy** desteği, birlikte gelen `mtg` yardımcı bileşeniyle.
- **Celali (Farsça) / Miladi takvim** tüm tarihler için değiştirilebilir.
- **Panel içi Eğitimler sayfası** (yalnızca super_admin) — her panel bölümü için çok dilli hızlı başlangıç kılavuzu.
- **Cihaz (HWID) limiti** — Happ/Hiddify tarzı uygulamaların gönderdiği `x-hwid` başlığıyla, bir kullanıcının aboneliğini kaç cihazın indirebileceğini sınırlar. Panel geneli varsayılan ve kullanıcı bazında geçersiz kılma, temizlenebilir cihaz listesi ve kimlik göndermeyen uygulamaları reddeden isteğe bağlı katı mod.
- **Süresi dolan kullanıcıları silme** — bir gelen bağlantının biten kullanıcılarını satır menüsünden temizleyin ya da panelin bitişten belirli gün sonra otomatik silmesine izin verin (varsayılan kapalı; 0 gün asla silmez).
- **Abonelik sayfası markası** — kullanıcılarınızın tarayıcıda açtığı sayfayı panelden oluşturun: marka adı, slogan, logo, duyuru, renkler, arka plan, görünen bölümler, destek/Telegram/web sitesi düğmeleri ve özel CSS, canlı önizlemeyle.
- **Bayilik satış botu** — kendi token'ı ve kendi yönetici listesiyle çalışan ikinci bir Telegram botu (Panel Ayarları → Telegram → Satış Botu'ndan açılır), Farsça olarak bayilik paneli satar. Paketleri panelde ya da botun içinde tanımlarsınız — fiyat, trafik kotası, kullanıcı kontenjanı, verilen gelenler; alıcı bunlara bakar, ödeme dekontunu yollar ve siz onayladığınız anda panel bayilik hesabını oluşturup giriş bilgilerini gönderir. Tekrar satın alma ikinci bir hesap açmak yerine aynı hesabı yükler, ve yükleme kotası bittiği için kapanmış bir hesabı geri getirir. Botun yönetim tarafı da vardır: bekleyen siparişler, paket yönetimi, aç/kapat ve trafik sıfırlamalı bayi listesi, satış rakamları ve toplu mesaj. Paneldeki **Satış** sayfası bunların tamamını sunar ve Telegram dışında yapılan bir satışı da kaydedebilir.
- **Özel roller** — **Yöneticiler** sayfasında kendi rollerinizi oluşturun: bir ad verin, taşıyacağı izinleri işaretleyin (gelenleri, kullanıcıları, planları, grupları, host'ları, düğümleri görüntüleme/yönetme, panel ayarları, Xray yapılandırması, yönetici yönetimi) ve yerleşik roller gibi atayın. *Atanan gelenlerle sınırla* seçeneğini işaretlerseniz rol, kotalarıyla birlikte tıpkı bir bayi gibi davranır. Hâlâ bir yöneticide olan bir rol silinemez.
- **Bayi panosu** — bayi giriş yaptıktan sonra doğrudan kendi panosuna düşer: trafik ve kullanıcı kotası çubukları, duruma göre kullanıcı sayıları (etkin, çevrimiçi, süresi yaklaşan, biten), gelen bazında dağılım, önümüzdeki hafta içinde süresi dolacak kullanıcılar ve en son oluşturulanlar.
- **Yöneticiyi devre dışı bırakmak gelenlerini de kapatır** — bir bayi hesabı kapatıldığında ona atanan gelenler de kapanır; böylece yalnızca bayinin girişi değil, müşterilerinin bağlantısı da kesilir. Yeniden etkinleştirmek tam olarak kapatılanları geri getirir ve elle kapattığınız bir gelene asla dokunmaz.
- **Güçlendirilmiş güvenlik** — CSRF koruması, güvenlik başlıkları ve yapılandırılabilir oturum ömrü.

## Hızlı Başlangıç (çevrimiçi)

```bash
bash <(curl -Ls https://raw.githubusercontent.com/admin6501/3x-ui/main/install.sh)
```

Kurulum sırasında rastgele bir kullanıcı adı, parola ve web yolu oluşturulur. Ardından yönetim menüsünü açmak için `x-ui` komutunu çalıştırın (başlat/durdur, kimlik bilgilerini sıfırla, SSL yönet vb.).

## Çevrimdışı kurulum (internetsiz)

İnternet erişimi olmayan — veya GitHub indirmesinin engellendiği — sunucular için:

1. `offline/x-ui-linux-amd64.tar.gz` ve `install_offline.sh` dosyalarını sunucuya kopyalayın (herhangi bir klasöre).
2. Çalıştırın:

```bash
chmod +x install_offline.sh
sudo ./install_offline.sh                 # geçerli dizindeki .tar.gz dosyasını otomatik algılar
# veya açıkça belirtin:
sudo ./install_offline.sh /root/x-ui-linux-amd64.tar.gz
```

Kurulum aracı paket için **hiçbir ağ çağrısı yapmaz** (ikili + Xray + geo'nun tümü tarball'dan gelir), oluşturulan kimlik bilgilerini **ve API belirtecini** yazdırır ve yükseltmede mevcut `/etc/x-ui/x-ui.db` dosyanızı **korurken** geçişleri (RBAC tabloları dahil) çalıştırır. Ayrıntılar için [`offline/README.md`](offline/README.md) dosyasına bakın.

> Önceden derlenmiş çevrimdışı paket **yalnızca amd64 / x86_64**'tür. Diğer mimariler için o mimariye yönelik bir paketi kaynaktan derleyin.

## Desteklenen platformlar

**İşletim sistemleri:** Ubuntu, Debian, Armbian, Fedora, CentOS, RHEL, AlmaLinux, Rocky Linux, Oracle Linux, Amazon Linux, Arch, Manjaro, openSUSE, Alpine ve Windows.

**Mimariler:** `amd64` · `arm64` (aarch64) · `armv7` · `armv6` · `386` · `s390x`.

## Veritabanı

- **SQLite** (varsayılan) — `/etc/x-ui/x-ui.db` konumunda tek bir dosya. Kurulum gerektirmez, küçük/orta dağıtımlar için idealdir.
- **PostgreSQL** — yüksek istemci sayıları veya çok düğümlü kurulumlar için önerilir.

Mevcut bir SQLite kurulumunu PostgreSQL'e taşıma (tüm RBAC verileri dahil):

```bash
x-ui migrate-db --dsn "postgres://xui:password@127.0.0.1:5432/xui?sslmode=disable"
# ardından /etc/default/x-ui içinde XUI_DB_TYPE ve XUI_DB_DSN'i ayarlayıp yeniden başlatın:
systemctl restart x-ui
```

Kaynak SQLite dosyası dokunulmadan kalır; yeni arka ucu doğruladıktan sonra silin.

## Ortam değişkenleri

| Değişken | Açıklama | Varsayılan |
| --- | --- | --- |
| `XUI_DB_TYPE` | Veritabanı arka ucu: `sqlite` veya `postgres` | `sqlite` |
| `XUI_DB_DSN` | PostgreSQL bağlantı dizesi (`XUI_DB_TYPE=postgres` olduğunda) | — |
| `XUI_DB_FOLDER` | SQLite veritabanı dosyası dizini | `/etc/x-ui` |
| `XUI_INIT_WEB_BASE_PATH` | Web panelinin başlangıç URI yolu | `/` |
| `XUI_ENABLE_FAIL2BAN` | Fail2ban tabanlı IP limiti uygulamasını etkinleştir | `true` |
| `XUI_LOG_LEVEL` | Günlük ayrıntısı (`debug`, `info`, `warning`, `error`) | `info` |

## Desteklenen diller

Panel arayüzü 13 dilde mevcuttur:

English · فارسی · العربية · 中文（简体） · 中文（繁體） · Español · Русский · Українська · Türkçe · Tiếng Việt · 日本語 · Bahasa Indonesia · Português (Brasil)

## Belgeler

- [Gerçek istemci IP'sini yakalama](docs/real-client-ip.md) (Cloudflare / L4 rölelerin arkasında).
- [Özel abonelik şablonları](docs/custom-subscription-templates.md).

## Teşekkürler ve lisans

Bu proje **[MHSanaei/3X-UI](https://github.com/MHSanaei/3x-ui)** projesinin bir fork'udur ve [Xray-core](https://github.com/XTLS/Xray-core) ile [alireza0](https://github.com/alireza0/) tarafından yazılan orijinal X-UI üzerine kuruludur. Coğrafi yönlendirme kuralları: [Iran v2ray rules](https://github.com/chocolate4u/Iran-v2ray-rules) ve [Russia v2ray rules](https://github.com/runetfreedom/russia-v2ray-rules-dat).

Üst proje ile aynı şekilde **GPL-3.0** altında lisanslanmıştır.

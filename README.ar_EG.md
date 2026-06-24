[English](/README.md) | [فارسی](/README.fa_IR.md) | [العربية](/README.ar_EG.md) | [中文](/README.zh_CN.md) | [Español](/README.es_ES.md) | [Русский](/README.ru_RU.md) | [Türkçe](/README.tr_TR.md)

<div dir="rtl">

# 3X-UI — نسخة معدّلة مع RBAC (admin6501)

نسخة مخصّصة من [MHSanaei/3X-UI](https://github.com/MHSanaei/3x-ui) — وهي لوحة تحكّم ويب متقدّمة ومفتوحة المصدر لإدارة خوادم [Xray-core](https://github.com/XTLS/Xray-core) — موسّعة بنظام **تعدّد المشرفين القائم على الأدوار (RBAC)**، و**تقييد الموزّع (Reseller)**، و**مثبّت غير متّصل بالكامل** للخوادم المعزولة أو المحظورة.

> [!IMPORTANT]
> هذا المشروع مخصّص للاستخدام الشخصي فقط. الرجاء عدم استخدامه لأغراض غير قانونية أو في بيئة إنتاجية.

## ما الذي يضيفه هذا الـ Fork

- **التحكّم في الوصول حسب الدور (RBAC)** — أدر عدّة مشرفين للوحة من صفحة **المشرفون**، لكلٍّ منهم أحد الأدوار الأربعة:
  - `super_admin` — وصول كامل لكل شيء (المشرف الأول/الافتراضي).
  - `manager` — إدارة المداخل والعملاء؛ **بدون** إعدادات اللوحة أو قالب Xray أو إدارة المشرفين.
  - `reseller` (الموزّع) — مقيّد بالمداخل المخصّصة له فقط؛ يدير عملاءه و**يشاهد** مداخله للقراءة فقط (لا يمكنه إضافة/تعديل/تفعيل-تعطيل/حذف المداخل).
  - `readonly` — يمكنه مشاهدة كل شيء لكن لا يمكنه تنفيذ أي عملية كتابة.
- **سجل التدقيق (Audit Log)** — تُسجَّل كل عملية للمشرف (إنشاء/تعديل/حذف مشرف، إعادة تعيين كلمة المرور، …) مع المنفّذ والهدف والوقت.
- **حزمة تثبيت غير متّصلة** — التثبيت على خادم بلا إنترنت عبر أرشيف tarball مكتفٍ ذاتيًا (ثنائي اللوحة + Xray-core + بيانات geo). انظر [`offline/`](offline/).
- **مدقّق تحديثات خاص بالـ Fork** — يقرأ "التحقق من التحديث" في اللوحة الإصدارات من هذا الـ Fork (`admin6501/3x-ui`).
- **ترحيل آمن إلى PostgreSQL** — يَنسخ ترحيل SQLite → PostgreSQL كل بيانات RBAC (المشرفون، الأدوار، المداخل المسموح بها، سجلات التدقيق).

## الميزات الأساسية (من 3X-UI)

- **مداخل متعدّدة البروتوكولات** — VLESS، VMess، Trojan، Shadowsocks، WireGuard، Hysteria2، HTTP، SOCKS (Mixed)، Dokodemo-door / Tunnel، وTUN.
- **نقل وأمان حديث** — TCP (Raw)، mKCP، WebSocket، gRPC، HTTPUpgrade، وXHTTP، مؤمّنة بـ TLS وXTLS وREALITY.
- **إدارة لكل عميل** — حصص الترافيك، تواريخ الانتهاء، حدود الـ IP، الحالة المباشرة، روابط المشاركة، رموز QR والاشتراكات.
- **دعم متعدّد العقد (Multi-node)**، المخارج والتوجيه (WARP، قواعد مخصّصة، موازِنات الحِمل).
- **خادم اشتراك مدمج** مع [قوالب صفحة مخصّصة](docs/custom-subscription-templates.md).
- **بوت تيليجرام**، **واجهة REST API** مع Swagger داخل اللوحة، **SQLite أو PostgreSQL**، **13 لغة واجهة**، سمات داكنة/فاتحة، وفرض حدود IP عبر **Fail2ban**.

## البدء السريع (متّصل)

```bash
bash <(curl -Ls https://raw.githubusercontent.com/admin6501/3x-ui/main/install.sh)
```

يُنشأ اسم مستخدم وكلمة مرور ومسار ويب عشوائية أثناء التثبيت. شغّل `x-ui` بعد ذلك لفتح قائمة الإدارة (تشغيل/إيقاف، إعادة تعيين البيانات، إدارة SSL، …).

## التثبيت غير المتّصل (بلا إنترنت)

للخوادم بلا إنترنت — أو حيث يكون التنزيل من GitHub محظورًا:

1. انسخ `offline/x-ui-linux-amd64.tar.gz` و`install_offline.sh` إلى الخادم (أي مجلد).
2. نفّذ:

```bash
chmod +x install_offline.sh
sudo ./install_offline.sh                 # يكتشف ملف .tar.gz في المجلد الحالي تلقائيًا
# أو مرّره صراحةً:
sudo ./install_offline.sh /root/x-ui-linux-amd64.tar.gz
```

لا يُجري المثبّت **أي اتصال شبكي** للحزمة (الثنائي + Xray + geo كلها من الأرشيف)، ويطبع البيانات المُنشأة **ورمز API**، وعند الترقية **يحافظ على ملف `/etc/x-ui/x-ui.db` الحالي** مع تنفيذ الترحيلات (بما في ذلك جداول RBAC). انظر [`offline/README.md`](offline/README.md) للتفاصيل.

> الحزمة الجاهزة غير المتّصلة هي **amd64 / x86_64 فقط**. لبُنى أخرى، ابنِ حزمة لتلك البنية من المصدر.

## المنصّات المدعومة

**أنظمة التشغيل:** Ubuntu، Debian، Armbian، Fedora، CentOS، RHEL، AlmaLinux، Rocky Linux، Oracle Linux، Amazon Linux، Arch، Manjaro، openSUSE، Alpine، وWindows.

**البُنى:** `amd64` · `arm64` (aarch64) · `armv7` · `armv6` · `386` · `s390x`.

## قاعدة البيانات

- **SQLite** (افتراضي) — ملف واحد في `/etc/x-ui/x-ui.db`. بلا إعداد، مثالي للنشر الصغير/المتوسط.
- **PostgreSQL** — يُنصح به لأعداد العملاء الكبيرة أو الإعدادات متعدّدة العقد.

ترحيل تثبيت SQLite قائم إلى PostgreSQL (مع كل بيانات RBAC):

```bash
x-ui migrate-db --dsn "postgres://xui:password@127.0.0.1:5432/xui?sslmode=disable"
# ثم اضبط XUI_DB_TYPE و XUI_DB_DSN في /etc/default/x-ui وأعد التشغيل:
systemctl restart x-ui
```

يبقى ملف SQLite الأصلي دون مساس؛ احذفه بعد التحقّق من الواجهة الخلفية الجديدة.

## متغيّرات البيئة

| المتغيّر | الوصف | الافتراضي |
| --- | --- | --- |
| `XUI_DB_TYPE` | واجهة قاعدة البيانات: `sqlite` أو `postgres` | `sqlite` |
| `XUI_DB_DSN` | سلسلة اتصال PostgreSQL (عند `XUI_DB_TYPE=postgres`) | — |
| `XUI_DB_FOLDER` | مجلّد ملف قاعدة بيانات SQLite | `/etc/x-ui` |
| `XUI_INIT_WEB_BASE_PATH` | مسار URI الأولي للوحة الويب | `/` |
| `XUI_ENABLE_FAIL2BAN` | تفعيل فرض حدود IP عبر Fail2ban | `true` |
| `XUI_LOG_LEVEL` | مستوى السجل (`debug`، `info`، `warning`، `error`) | `info` |

## اللغات المدعومة

واجهة اللوحة متوفّرة بـ 13 لغة:

English · فارسی · العربية · 中文（简体） · 中文（繁體） · Español · Русский · Українська · Türkçe · Tiếng Việt · 日本語 · Bahasa Indonesia · Português (Brasil)

## التوثيق

- [التقاط الـ IP الحقيقي للعميل](docs/real-client-ip.md) (خلف Cloudflare / مرحّلات L4).
- [قوالب اشتراك مخصّصة](docs/custom-subscription-templates.md).

## الاعتمادات والترخيص

هذا المشروع Fork من **[MHSanaei/3X-UI](https://github.com/MHSanaei/3x-ui)** ويعتمد على [Xray-core](https://github.com/XTLS/Xray-core) ونسخة X-UI الأصلية من [alireza0](https://github.com/alireza0/). قواعد التوجيه الجغرافي: [Iran v2ray rules](https://github.com/chocolate4u/Iran-v2ray-rules) و[Russia v2ray rules](https://github.com/runetfreedom/russia-v2ray-rules-dat).

مرخّص بموجب **GPL-3.0**، مثل المشروع الأصلي.

</div>

# 即刻收网关

YIN 自建的支付宝「即刻收 / NowPay」**平台模式**网关脚手架。阶段一让管理员为创作者建项目，创作者按席位永久买断「可绑定的个人支付宝收款账号」，顾客再通过支付宝托管购买把钱付进该账号。支付宝 OpenAPI 目前全部走模拟适配器，方法名和字段对齐公开的 `alipay.aipay.nowpay.*`，合作平台批下来之后可以换成真实 SDK。

阶段一没有开放给创作者的商户 OpenAPI，也不做个人收款码轮换。

## 两笔钱

```text
创作者 ──席位买断──▶ YIN（平台）
顾客   ──即刻收托管购买──▶ 创作者的支付宝
                         └─ 平台按比例记账抽成，之后再结算
```

**第一笔，创作者付给 YIN。** 每一个可以绑定的支付宝账号是一个席位。席位按当时的目录价（或项目覆盖价）做 `ONE_TIME` 永久买断。再加一个支付宝账号，就再买一席，用下单那一刻的价格，不改已经买过的席位。时长包、次数包、积分包在价格目录里留了行，购买接口会拒绝。

**第二笔，顾客付给创作者。** 平台调用即刻收：`charge.query` / `charge.initialize` 把创作者送进托管配置页（扫码或授权、确认档位），顾客侧走 `purchase.consult` / `purchase.create`。本金进创作者的支付宝，不经过 YIN。

支付宝自己还有收单费（JSAPI，常见 0.6%–1%）和即刻收商品经营费（文档中的 0.4%）。这两项由支付宝从创作者侧扣除，本网关只按配置估算并记成备忘，不进平台现金。

平台抽成是另外一层，默认引擎 `mock_book_then_settle`：

1. 顾客付款成功时，按比例把抽成记成应收，同时确认收入（先记账）。
2. 管理员在分录里执行结算时，再记现金、冲掉应收。

新商户在支付宝侧可能要次日才结算到账，分账也不保证当时能切走，所以抽成设计成可替换的 `commission.Engine`。换成真实代扣或分账时，只替换这个引擎，账本科目不用改。

取整规则：金额用分（整数）。`金额 × 基点 / 10000`，四舍五入到分，0.5 分进 1。10000 基点 = 100%。

## 架构

```text
浏览器  管理端 / 创作者门户 / 模拟托管页
   │
   ▼
web (nginx)  /api  反代
   │
   ▼
api (Go, chi) ── Postgres
   ├── 会话、创作者、项目、席位买断
   ├── 账本与结算批次、审计、通知投递
   └── alipay.Client
         ├── mock   持久化在 Postgres，重启不丢绑定
         └── live   签名工具已备好，调用一律返回未接通，不访问外网
```

内部 HTTP API 见 `internal/docs/openapi.yaml`，进程起来后也可以打开 `/api/openapi.yaml`。那是管理端和创作者门户的接口，不是给创作者发 Key 的商户 API。

支付宝侧要接上的方法：

| 方法 | 用途 |
| --- | --- |
| `alipay.aipay.nowpay.charge.query` | 查收费能力、档位、管理入口 |
| `alipay.aipay.nowpay.charge.initialize` | 要托管配置页。不传价格 |
| `alipay.aipay.nowpay.charge.modify` | 启用或停用新增购买 |
| `alipay.aipay.nowpay.purchase.consult` | 买断访问决策：`ALLOW` / `PURCHASE_REQUIRED` / `DENY` |
| `alipay.aipay.nowpay.purchase.create` | 取购买链接，`out_request_no` 幂等 |
| `quota.query` / `quota.verify` / `quota.refresh` | 接口签名已留出。阶段一商品不是额度包，返回 `BILLING_MODE_NOT_SUPPORTED` |

`out_product_id` 用席位的稳定编号，`external_owner_id` 用创作者的稳定编号。这样同一个创作者每多一个支付宝账号，就是一条独立的收费配置。

页面回跳（`callback_url`）和本网关的异步通知不是一回事。异步通知是 `POST` 到项目的 `notify_url`，事件有 `purchase.paid` 和 `charge.enabled`，头 `X-Yin-Signature: sha256=` HMAC-SHA256（密钥 `WEBHOOK_SECRET`）。地址空着就记为跳过。投递失败不回滚入账。

## 本地运行

需要 Docker。

```bash
docker compose up --build
```

- 控制台：<http://127.0.0.1:43123>
- API：<http://127.0.0.1:18480>（一般走上面的反代即可）
- 初始管理员：`admin@yin.local` / `changeme-yin`

登录后在「审计」页改掉管理员密码。

建议走一遍：

1. 建创作者，填门户邮箱和密码。
2. 建项目，填写顾客买断价；席位价和抽成可以留空，分别用目录价和全局抽成。
3. 在项目里「新开席位并登记买断」。
4. 生成绑定二维码，或直接「模拟完成支付宝授权」。
5. 「模拟顾客付款」。分录里会出现抽成应收；创作者门户能看到这笔付款。

创作者用门户账号登录后，只能改回跳地址、买断新席位、完成授权，看不到别的创作者，也没有商户 API Key。

只跑计算测试，不需要数据库：

```bash
go test ./...
```

`scripts/smoke.sh` 对已经起来的 compose 做同一条管理端路径。

开发时可以单独起 API（指到本机 Postgres）和 `cd web && npm install && npm run dev`（默认 <http://127.0.0.1:43124>，把 `/api` 代理到 `18480`）。

## 环境变量

| 变量 | 作用 |
| --- | --- |
| `DATABASE_URL` | Postgres 连接串 |
| `HTTP_ADDR` | API 监听地址，默认 `:8080` |
| `ADMIN_EMAIL` / `ADMIN_PASSWORD` / `ADMIN_NAME` | 仅当库里还没有管理员时写入。密码至少 8 位 |
| `ALIPAY_MODE` | `mock`（默认）或 `live` |
| `ALIPAY_APP_ID` | 支付宝应用 APPID |
| `ALIPAY_PRIVATE_KEY` | 应用私钥 PEM。换行可写成 `\n` |
| `ALIPAY_PUBLIC_KEY` | 支付宝公钥 PEM |
| `ALIPAY_PRIVATE_KEY_FILE` / `ALIPAY_PUBLIC_KEY_FILE` | 从文件读密钥，优先于上面的内联值 |
| `ALIPAY_GATEWAY_URL` | 默认 `https://openapi.alipay.com/gateway.do` |
| `ALIPAY_APP_AUTH_TOKEN` | 第三方代调用时才需要 |
| `WEBHOOK_SECRET` | 异步通知 HMAC 密钥 |
| `PUBLIC_BASE_URL` | 模拟托管页的外部地址，compose 里是 `http://127.0.0.1:43123` |
| `ALLOWED_ORIGINS` | 浏览器来源，逗号分隔。空 Origin（curl）放行，对不上的 Origin 拒绝 |
| `AUTO_COMMISSION` | 默认 `mock_book_then_settle`。其它名字启动失败 |

示例见 `.env.example`。

## 模拟和正式

`ALIPAY_MODE=mock` 时，配置页和购买页是本仓库里的页面，标题写明「模拟」。二维码打开的是 `configuration_qr_code` / `purchase_qr_code`。`configuration_url` 和 `purchase_url` 保持 `alipays://` 形状，正式环境直接用支付宝返回值，不要自己拼。

`ALIPAY_MODE=live` 可以启动，但每个 OpenAPI 方法都返回「未接通」，**不会**请求 `openapi.alipay.com`。`internal/alipay/sign.go` 里的 `BuildGatewayParams`、`SignContent`、`SignRSA2`、`VerifyRSA2` 按 RSA2、UTF-8、JSON、版本 1.0 准备好了，给以后的 SDK 调用用。

要换成真网关，需要先成为支付宝即刻收合作平台。产品说明写明：智能收款套件目前只让创作者通过合作平台接入，平台方要单独申请。批下来之前，直播调用没有意义。

接上之后建议按这个顺序改 `internal/alipay/live.go`：

1. 用应用私钥对 `BuildGatewayParams` 的待签串做 RSA2，POST 到 `ALIPAY_GATEWAY_URL`。
2. 用支付宝公钥验响应签，再读 `alipay_aipay_nowpay_*_response`。
3. 只有 `code=10000` 才当成功。`NOW_PAY_ALREADY_CONFIGURED` 要回头 `charge.query`，不要把初始化成功当成已经可售。
4. 创作者从托管页回来之后必须再查一次。买断只有 `purchase.consult` 的 `decision=ALLOW` 才放行。
5. 托管页上的价格以查询结果为准。初始化请求不要带 `price`。
6. 真通知仍以支付宝规则为准。本仓库的 `notify_url` 是平台自己的回调，不是支付宝的异步通知。

支付宝文档入口：<https://aipay.alipay.com/docs/vibe-pay/nowpay/platform/platfrom-guide.html>  
合作平台申请：<https://render.alipay.com/p/yuyan/180022670000179729/index.html?type=platform>

## 许可

Apache License 2.0。见 `LICENSE`。

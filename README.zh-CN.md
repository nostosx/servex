# servex

[English](README.md) | 简体中文

servex 是面向 Go 微服务的工具库，核心提供协议无关的契约和每个契约的一个参考实现，
仅依赖标准库与 [kitx](https://github.com/nostosx/kitx)。依赖第三方 SDK 的实现，
如 gRPC、Redis 和 OpenTelemetry，位于
[servex-contrib](https://github.com/nostosx/servex-contrib)。

当前版本为 v1.3.0。次版本可能包含不兼容的 API 变更，每项变更及迁移方式都列在
[CHANGELOG.md](CHANGELOG.md) 中；补丁版本不改变 API。可运行的 HTTP 和 gRPC 程序位于独立的
[servex-examples](https://github.com/nostosx/servex-examples) 项目。

## 开始使用

需要 Go 1.27.3。在应用模块根目录安装：

```sh
go get github.com/nostosx/servex@v1.3.0
```

kitx v1.0.0 从 Go 模块代理解析，无需并排检出仓库或添加本地替换。
核心标签不带目录前缀，例如 `v1.3.0`。

## 设计原则

核心模块及测试只允许使用标准库和 kitx；CI 拒绝其他外部依赖，包括 `golang.org/x/...`。
代码归属遵循以下约束：

1. **核心提供契约，按能力保留参考实现。** 配置和日志仅定义契约。 简单实现位于契约包中，例如 `encoding.JSON`、
   `selector.Random` 和空观测实现；有状态的实现位于子目录，
   如 `*/memory` 和 `transport/httpx`。其他实现放入 servex-contrib。
2. **配置和日志只定义契约。** `config` 与 `logger` 不包含具体实现；
   Viper、slog、Zap 实现位于 servex-contrib，由应用显式注入。SQL 使用 `database/sql`。
   servex 补充观测、生命周期和健康检查，调用方继续使用原生类型。
3. **显式装配。** 不提供全局注册表、自动中间件排序、`init()` 注册或全局可变状态。
   构造函数显式接收依赖；可选的 `di` 契约由 contrib 实现，容器仅用于应用装配入口。

`di` 只定义契约和类型化辅助函数，核心不实现第二套 DI 引擎。该能力自 v1.3.0 起提供，
`servex-contrib/di/do` 提供 samber/do/v2 实现。
注册可集中在一个 `bootstrap.go`，业务包继续使用普通构造函数。

导入分层由 `internal/archtest` 检查。精确的包级导入表和 kitx 白名单见
[CONTRIBUTING.md](CONTRIBUTING.md#dependency-boundaries)。

| 层级 | 包 | 允许依赖 |
|---|---|---|
| 基础 | `app`、`logger`、`di`、`cqrs`、`endpoint`、`errorx`、`metadata`、`health`、`encoding`、`observability/*` | 按包级白名单；app 可依赖 logger |
| 契约 | `config`、`selector`、`discovery`、`storage/*`、`messaging/*` | 按包级白名单使用基础能力 |
| 传输 | `transport` | 基础、契约与 `kitx/backoff` |
| 实现 | `*/memory`、`transport/httpx` 等 | 自身契约及允许的上层包，不依赖其他实现 |

## 包概览

| 目录 | 职责 |
|---|---|
| `transport/sse` | SSE 事件、发送、握手、接收与客户端契约；contrib `transport/ssex` 提供实现，尚未发布 |
| `transport/websocket` | WebSocket 消息、连接、握手与关闭状态契约；contrib `transport/websocketx` 提供实现，尚未发布 |
| `transport/rpc` | 类型化 unary 请求/响应及流接口；contrib `transport/connectx` 提供 Connect/gRPC/gRPC-Web 适配，尚未发布 |
| `app` | Hook 顺序启动、逆序停止、启动失败回滚、就绪检查和运行失败处理；`SignalContext` 处理 SIGINT/SIGTERM |
| `di` | 可选的 `Container` / `Resolver` 契约和类型化 `Provide`、`Supply`、`Resolve`、`MustResolve`；contrib 提供实现，自 v1.3.0 起提供 |
| `cqrs` | 类型化应用 `Handler`、`Command`、`Query` 与显式中间件组合；自 v1.1.0 起提供 |
| `endpoint` | 协议无关的 `Endpoint`、`Middleware` 和 `Chain` |
| `errorx` | 结构化错误码、原因、元数据和重试标记，不绑定协议 |
| `metadata` | 上下文中的请求元数据与调用描述 |
| `health` | 健康检查契约和并发检查组 |
| `encoding` | Codec 契约和 JSON 实现 |
| `config` | `Source`、`Watchable`、`Reader`、`Config` 契约；加载、解码、快照发布由 contrib 实现 |
| `logger` | 后端无关的 `Logger`、`Level` 契约，无默认或全局后端 |
| `observability/metrics`、`observability/tracing` | 后端无关的指标、追踪契约和空实现 |
| `discovery`、`discovery/memory` | 注册、查询、订阅、注册 Hook 和进程内注册中心 |
| `selector` | 节点选择契约和加权随机实现 |
| `transport`、`transport/httpx` | 服务端与客户端契约、节点选择、HTTP 路由、绑定、编码协商和健康端点 |
| `storage/cache`、`storage/cache/memory` | 字节缓存契约、共享回源 Loader 和内存 TTL/LRU 适配器 |
| `storage/database` | 普通 `*sql.DB` 的驱动层观测、生命周期、健康检查和事务助手 |
| `messaging/broker`、`messaging/broker/memory` | 发布订阅契约、消费者 Hook 和进程内消息代理 |
| `messaging/jobqueue`、`messaging/jobqueue/memory` | 任务契约、延迟/重试/唯一性选项、工作者 Hook 和进程内队列 |
| `internal/...` | 私有的依赖检查、发现地址选择和消费者生命周期助手 |

## 类型化应用处理器

`cqrs` 自 v1.1.0 起提供。
业务请求与结果仍放在应用自己的 command/query 包中。
`Command[Request, Result]`、`Query[Request, Result]` 是嵌入 `Handler` 的接口；
`CommandFunc`、`QueryFunc` 将函数适配为这些契约，具有对应 `Handle` 方法的结构体同样可以实现。
命令可以返回业务结果，没有结果载荷时使用 `struct{}`。

例如，在应用构造函数中传入消费方定义的 writer：

```go
return cqrs.ChainCommand("catalog.create_product", cqrs.CommandFunc[Create, Product](
    func(ctx context.Context, cmd Create) (Product, error) {
        return products.Create(ctx, cmd)
    },
), middleware...)
```

`middleware` 类型为 `[]cqrs.CommandMiddleware[Create, Product]`，每个中间件在装配时
获得独立的 `cqrs.Metadata{Kind, Operation}` 值副本。查询使用 `ChainQuery`；操作名应为
`catalog.get_product` 这样的稳定常量，不能使用请求 ID 或用户输入，空白名称会在装配时 panic。
命令与查询中间件共享泛型实现，命名表达意图，不提供读写副作用的编译期隔离。装配时组合一次并复用，
第一项位于最外层，nil 项会跳过。中间件可以拒绝执行、派生 context 或转换结果与错误。
nil handler 属于编程错误。完整中间件实现和顺序验证见
[可执行示例与测试](cqrs/cqrs_test.go)。

处理器同步执行；除非中间件主动变换，否则保持调用方的 context、结果与错误。
共享状态需要自行保证并发安全。查询的只读边界由应用端口与测试保证，此包不提供副作用约束。
传输中间件继续使用 `endpoint`，业务中间件使用 `cqrs`，应用内部无需经过 `any` 转换。
事务和命令幂等仍需显式实现，没有业务幂等保证时不能自动重试写操作。

## 示例

在新目录中初始化应用模块：

```sh
mkdir servex-demo
cd servex-demo
go mod init example.com/servex-demo
go get github.com/nostosx/servex@v1.3.0
mkdir server client
```

将下面的代码保存为 `server/main.go`。HTTP 服务绑定路径参数，返回 JSON，并在 SIGINT/SIGTERM 时优雅关闭。
就绪检查跟随应用状态；监听地址限定为本机，并显式设置请求体限制和连接超时。

```go
package main

import (
	"context"
	"log"
	"time"

	"github.com/nostosx/servex/app"
	"github.com/nostosx/servex/errorx"
	"github.com/nostosx/servex/health"
	"github.com/nostosx/servex/transport/httpx"
)

type GetUser struct {
	ID int `json:"id"`
}

type User struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

func getUser(_ context.Context, req *GetUser) (*User, error) {
	if req.ID <= 0 {
		return nil, errorx.New(errorx.InvalidArgument, "INVALID_ID", "id must be positive")
	}
	return &User{ID: req.ID, Name: "alice"}, nil
}

func run() error {
	var a *app.App
	srv := httpx.NewServer(
		httpx.WithAddress("127.0.0.1:8000"),
		httpx.WithReadHeaderTimeout(5*time.Second),
		httpx.WithReadTimeout(10*time.Second),
		httpx.WithWriteTimeout(10*time.Second),
		httpx.WithIdleTimeout(time.Minute),
		httpx.WithMaxBodyBytes(64<<10),
		// GET /readyz answers 200 only while the App is running.
		httpx.WithHealth(health.CheckerFunc(func(ctx context.Context) error { return a.Check(ctx) })),
	)
	// {id} and query parameters bind to the JSON fields of GetUser.
	httpx.RouteFunc(srv, "GET /v1/users/{id}", getUser)

	a = app.New(app.WithHooks(srv))
	ctx, stop := app.SignalContext(context.Background())
	defer stop()
	return a.Run(ctx)
}

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}
```

保存文件后解析导入的依赖，再启动服务：

```sh
go mod tidy
go run ./server
```

在另一个终端执行 `curl http://127.0.0.1:8000/v1/users/7` 返回 `{"id":7,"name":"alice"}`；
`/v1/users/0` 返回 HTTP 400 和 errorx 错误体。对外提供服务前，请配置认证、授权和 TLS。
内存参考实现不提供持久化或跨进程协调。

将下面的客户端保存为同一示例模块中的 `client/main.go`：

```go
package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/nostosx/servex/transport/httpx"
)

type GetUser struct {
	ID int `json:"id"`
}

type User struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	// A "discovery:///user" target with httpx.WithDiscoverer resolves nodes
	// through service discovery instead.
	client, err := httpx.NewClient(ctx, "http://127.0.0.1:8000")
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	get := httpx.Call[User](client, "GET", "/v1/users/{id}")
	reply, err := get(ctx, &GetUser{ID: 7})
	if err != nil {
		log.Fatal(err) // an *errorx.Error for non-2xx replies
	}
	fmt.Println(reply.(*User).Name)
}
```

保持服务运行，在另一个终端从示例模块根目录执行 `go run ./client`，输出为 `alice`。
使用 Ctrl+C 停止服务。

需要服务注册时，将
`discovery.Registration(registrar, discovery.Instance{Name: "user"}, srv)`
放在 `app.WithHooks` 的服务端 Hook 之后，这样端口打开后才注册。`Instance.Weight`
设置实例在客户端选择时的相对权重。注册 Hook 实现可选的 `Drain(ctx) error` 方法：App
停止时先逆序排空各 Hook，即注销实例；然后等待 `app.WithStopDelay`，让服务端处理完进行中的
请求；最后才停止各 Hook。排空失败时，Hook 的 `Stop` 会再重试一次注销。

同一 App 中的每个服务端、消费者和 worker 应使用 `httpx.WithName`、`broker.WithName`
或 `jobqueue.WithName` 设置各自的 Hook 名称。消费者默认名为 `consumer:<topic>`，
有消费组时为 `consumer:<topic>:<group>`。实现方通过 `broker.ReadySubscriber` 或
`jobqueue.ReadyRunner` 选择加入（内存实现均已加入）时，消费者或 worker 在订阅建立或
开始处理后才完成启动，其 Hook 同时参与就绪检查。
`database.Hook(name, db, database.WithReadiness(false))` 保留启动时的 ping，但不让数据库
参与就绪检查，适用于数据库不可用时仍能处理部分请求的服务。

中间件使用 `endpoint.Middleware`，按传入顺序执行，首项为最外层。使用
servex-contrib 中间件时，建议顺序为：

```
server:   recovery -> tracing -> logging -> metrics -> auth -> ratelimit -> validate -> handler
client:   tracing -> logging -> metrics -> retry -> circuitbreaker -> timeout -> [select node -> invoke]
consumer: recovery -> tracing -> logging -> metrics -> handler
```

HTTP 端点中间件在路由匹配并绑定请求体之后执行。需要在读取请求体前拒绝请求的检查
（如认证和限流），以及需要同时观测 404、请求体超限 413 和绑定失败的 `net/http`
观测组件，应使用 `httpx.WithHTTPMiddleware(...)`。它包裹整个服务端 Handler，包括
`HandleHTTP` 路由和健康探针；首项为最外层。

HTTP 错误响应体携带 `retryable`，即提供给调用方的重试标记。客户端从其他服务解码的错误
标记为远端错误（`errorx.WithRemote`）：该客户端自身的重试中间件仍可重试，但返回这种错误
（包括用 `%w` 包装后）的服务端会将其报告为不可重试（`PropagatedRetryable`）。只有紧邻
故障的一跳重试，层层重试的调用链不会成倍放大到达故障服务的请求。处理函数对远端错误
调用 `WithRetryable` 即由本服务作出决定，标记按设置值发送。客户端收到不含该字段的响应体
时使用错误码默认值。
客户端传输失败仅在未取得连接，或请求按 `net/http` 规则可重放（GET、HEAD、OPTIONS、
TRACE，或带有 `Idempotency-Key`/`X-Idempotency-Key` 请求头）时才是可重试的
`Unavailable`；否则请求可能已经执行，`Unavailable` 错误被显式标记为不可重试。

## 类型化端点

`endpoint.Typed` 把具体请求、响应类型的函数适配为 `endpoint.Endpoint`；请求类型不符时
返回 `*endpoint.TypeError`，服务端将其报告为 500 `Unknown`；请求为 nil 时传入零值。
`httpx.RouteFunc` 注册这种函数，请求绑定到 `*Req`：

```go
httpx.RouteFunc(srv, "GET /v1/users/{id}", func(ctx context.Context, req *GetUser) (*User, error) {
	return users.Get(ctx, req.ID)
})
```

同一适配器可以把 `cqrs` 处理器接到任意传输、消费者或 worker，无需经由 `any` 转换：

```go
httpx.Route[CreateProduct](srv, "POST /v1/products", endpoint.Typed(createProduct.Handle))
```

其中 `createProduct` 为 `cqrs.Command[*CreateProduct, Product]`。

## 客户端节点

`transport.WithNodeObserver(fn)` 在客户端节点集合变化时回调，按 `Addr`、`Weight` 与
`Metadata` 比较。首次回调发生在 `NewClient` 返回之前；之后随服务发现更新逐次回调，
传入深拷贝，`Close` 返回后不再回调。可用于同步有状态的选择器或指标；回调中不得调用 `Close`。

指标记录方法接收被度量操作的 context（`Counter.Add(ctx, v)`、`Gauge.Set(ctx, v)`、
`Histogram.Observe(ctx, v)`），以便指标实现关联当前 span。

## HTTP 限制与自定义响应

服务端选项配置由 `Start` 使用的原生 `http.Server`：

```go
srv := httpx.NewServer(
	httpx.WithReadHeaderTimeout(5*time.Second),
	httpx.WithReadTimeout(30*time.Second),
	httpx.WithWriteTimeout(30*time.Second),
	httpx.WithIdleTimeout(60*time.Second),
	httpx.WithMaxHeaderBytes(1<<20),
	httpx.WithMaxBodyBytes(8<<20),
)
```

超时默认为零，遵循 `net/http` 语义：零值请求头和空闲超时继承 `ReadTimeout`，
负数禁用继承；零值请求头大小使用标准库默认值。由其他 `http.Server` 托管 Handler 时，
应在该服务器上配置传输选项。连接超时不等于处理函数的执行截止时间。

`WithMaxBodyBytes` 限制 `Handle` 和 `Route` 的完整请求体，包含 multipart 边界
及没有请求工厂的路由。默认值为 `httpx.DefaultMaxBodyBytes`（4 MiB，与 gRPC 默认消息
大小一致），上传更大内容时需调高。已知长度可提前拒绝，剩余内容在执行端点前以有界内存
读取。非正数禁用此限制。检测到超限时返回 HTTP 413 JSON，错误为 `InvalidArgument` 和
`BODY_TOO_LARGE`；畸形输入可能先返回 400。`HandleHTTP` 处理函数自行控制请求体限制。
Multipart 文件在端点执行和响应编码后清理。

通过路由响应编码器自定义成功状态码、响应头、Cookie 和响应体：

```go
httpx.RouteFunc(srv, "POST /v1/users", getUser,
	httpx.WithResponseEncoder(func(w http.ResponseWriter, r *http.Request, reply any) error {
		data, err := json.Marshal(reply)
		if err != nil {
			return err
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Location", "/v1/users/7")
		http.SetCookie(w, &http.Cookie{Name: "created", Value: "1", HttpOnly: true})
		w.WriteHeader(http.StatusCreated)
		_, err = w.Write(data)
		return err
	}),
)
```

编码器在中间件包装的端点成功后执行，包括返回 nil 的情况。请求包含传输元数据；
绑定和端点错误继续使用固定 JSON 响应。默认编码使用协商后的 Codec，非 nil 返回 200，
nil 返回 204。尽可能在写入响应前返回编码错误：响应提交前映射为 JSON 500
`ENCODE_REPLY`，提交后仅记录日志，不追加第二份响应。刷新等可选能力使用
`http.ResponseController`。响应编码发生在端点中间件返回之后。

## 配置与日志契约

核心 `config` 只定义 `Source`、`Watchable`、`Reader`、`Config`，
`logger` 只定义 `Logger` 和日志级别。配置加载、环境变量、Viper 解码、
校验、监听和快照发布位于 [config/viper](https://github.com/nostosx/servex-contrib/tree/main/config/viper)；
日志后端位于 [logger/slog](https://github.com/nostosx/servex-contrib/tree/main/logger/slog)
和 [logger/zap](https://github.com/nostosx/servex-contrib/tree/main/logger/zap)，由应用显式注入：

```go
log := slogadapter.New(slog.New(slog.NewJSONHandler(os.Stderr, nil)))
application := app.New(app.WithLogger(log))
server := httpx.NewServer(httpx.WithLogger(log))
```

其中 `slogadapter` 导入 `github.com/nostosx/servex-contrib/logger/slog`。
省略可选 Logger 时不记录日志；核心不选择全局或默认后端。
原来的 `*slog.Logger` 需要显式包装。这些变更尚未发布，
开发验证需使用匹配的核心与 contrib 源码；发布时先核心、后适配器。

## 缓存 Loader 生命周期

创建 Loader 时传入 owner 上下文和正数共享执行超时：

```go
loader, err := cache.NewLoader[User](owner, backend, 5*time.Second, cache.WithTTL(time.Minute))
if err != nil {
	return err
}
defer loader.Close()
```

owner 应属于服务或显式作用域，生命周期长于单次请求。等待者给 `Get` 传入请求上下文，
可以独立取消；回源继承 owner 的值和取消信号，不继承发起请求的上下文值。
键必须包含租户与授权范围，共享凭据显式传入。`Close` 取消并等待回调，但不关闭后端。
执行超时、owner 取消和关闭都要求回调及后端响应取消；应用仍需限制同时活跃的不同键数。

同键回调必须可互换。共享结果中的指针、切片和映射不可修改。禁止在回源回调中调用
`Close` 或重新进入同一个键，否则会死锁。缓存失败回退和写回语义见
[Loader.Get](storage/cache/loader.go)。

## servex-contrib

[servex-contrib](https://github.com/nostosx/servex-contrib) 按依赖集合拆分 Go 模块，
路径为 `github.com/nostosx/servex-contrib/<capability>/<impl>`；另外提供无需额外 SDK
的 `middleware` 和 `selector` 模块。Contrib 仅导入核心公开包，各模块互不导入。
实现通过普通构造函数创建，可手动装配，也可交给应用已有的 DI 工具。

## 开发

使用 Go 1.27.3，在仓库根目录执行：

```sh
task check
task lint
task vulncheck
```

`check` 拒绝格式、现代化和 tidy 差异，检查普通及 `integration` 配置的 vet 和构建，
校验标准库与 kitx 依赖边界，并运行随机顺序的 race 测试。`lint` 和 `vulncheck`
也覆盖两种构建配置。所有目标使用 `GOWORK=off`。`fmt`、`fix` 和 `tidy` 会修改文件。

公开入口只有 `check`、`test`、`build`、`lint`、`vulncheck`、`fmt`、`fix`、`tidy`；
裸 `task` 默认执行 `check`。用 `task test RACE=true` 开启 race，用 `task test COVER=true`
运行 race 并生成 `coverage.out`、打印函数覆盖率。细项检查在内部执行，两种任务列表都只显示公开入口。

工具安装、变更要求和精确导入表见 [CONTRIBUTING.md](CONTRIBUTING.md)，未发布变更见
[CHANGELOG.md](CHANGELOG.md)，发布门槛见 [RELEASING.md](RELEASING.md)，
敏感问题见 [SECURITY.md](SECURITY.md)。核心无需服务端集成测试环境；CI 使用相同的
Task 目标。本地通过不代表远程 CI 已通过或模块已发布。

## 许可证

[MIT](LICENSE)。

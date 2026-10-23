# servex

[English](README.md) | 简体中文

servex 是面向 Go 微服务的工具库，核心提供协议无关的契约和每个契约的一个参考实现，
仅依赖标准库与 [kitx](https://github.com/nostosx/kitx)。依赖第三方 SDK 的实现，
如 gRPC、Redis 和 OpenTelemetry，位于
[servex-contrib](https://github.com/nostosx/servex-contrib)。

当前版本为 v1.0.0，已发布的 v1 版本保持公开 API 兼容，具体变更见
[CHANGELOG.md](CHANGELOG.md)。可运行的 HTTP 和 gRPC 程序位于独立的
[servex-examples](https://github.com/nostosx/servex-examples) 项目。

## 开始使用

需要 Go 1.27.3。在应用模块根目录安装：

```sh
go get github.com/nostosx/servex@v1.0.0
```

kitx v1.0.0 从 Go 模块代理解析，无需并排检出仓库或添加本地替换。
核心标签不带目录前缀，例如 `v1.0.0`。

## 设计原则

核心模块及测试只允许使用标准库和 kitx；CI 拒绝其他外部依赖，包括 `golang.org/x/...`。
代码归属遵循以下约束：

1. **核心提供契约和一个参考实现。** 简单实现位于契约包中，例如 `encoding.JSON`、
   `config.Env`、`selector.Random` 和空观测实现；有状态的实现位于子目录，
   如 `*/memory`、`config/file` 和 `transport/httpx`。其他实现放入 servex-contrib。
2. **保留标准接口。** 日志使用 `*slog.Logger`，SQL 使用 `database/sql`。
   servex 补充观测、生命周期和健康检查，调用方继续使用原生类型。
3. **显式装配。** 不提供全局注册表、DI 容器、自动中间件排序、`init()` 注册或全局可变状态。
   直接调用构造函数，执行顺序由代码决定。

导入分层由 `internal/archtest` 检查。精确的包级导入表和 kitx 白名单见
[CONTRIBUTING.md](CONTRIBUTING.md#dependency-boundaries)。

| 层级 | 包 | 允许依赖 |
|---|---|---|
| 基础 | `app`、`endpoint`、`errorx`、`metadata`、`health`、`encoding`、`observability/*` | 仅标准库，彼此不依赖 |
| 契约 | `config`、`selector`、`discovery`、`storage/*`、`messaging/*` | 按包级白名单使用基础能力 |
| 传输 | `transport` | 基础、契约与 `kitx/backoff` |
| 实现 | `*/memory`、`config/file`、`transport/httpx` 等 | 自身契约及允许的上层包，不依赖其他实现 |

## 包概览

| 目录 | 职责 |
|---|---|
| `app` | Hook 顺序启动、逆序停止、启动失败回滚、就绪检查和运行失败处理；`SignalContext` 处理 SIGINT/SIGTERM |
| `endpoint` | 协议无关的 `Endpoint`、`Middleware` 和 `Chain` |
| `errorx` | 结构化错误码、原因、元数据和重试标记，不绑定协议 |
| `metadata` | 上下文中的请求元数据与调用描述 |
| `health` | 健康检查契约和并发检查组 |
| `encoding` | Codec 契约和 JSON 实现 |
| `config`、`config/file` | 分层配置、环境变量源、轮询文件源、类型读取、快照与重载通知 |
| `observability/metrics`、`observability/tracing` | 后端无关的指标、追踪契约和空实现 |
| `discovery`、`discovery/memory` | 注册、查询、订阅、注册 Hook 和进程内注册中心 |
| `selector` | 节点选择契约和加权随机实现 |
| `transport`、`transport/httpx` | 服务端与客户端契约、节点选择、HTTP 路由、绑定、编码协商和健康端点 |
| `storage/cache`、`storage/cache/memory` | 字节缓存契约、共享回源 Loader 和内存 TTL/LRU 适配器 |
| `storage/database` | 普通 `*sql.DB` 的驱动层观测、生命周期、健康检查和事务助手 |
| `messaging/broker`、`messaging/broker/memory` | 发布订阅契约、消费者 Hook 和进程内消息代理 |
| `messaging/jobqueue`、`messaging/jobqueue/memory` | 任务契约、延迟/重试/唯一性选项、工作者 Hook 和进程内队列 |
| `internal/...` | 私有的依赖检查、发现地址选择和消费者生命周期助手 |

## 示例

下面的 HTTP 服务绑定路径参数，返回 JSON，并在 SIGINT/SIGTERM 时优雅关闭。
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

func getUser(_ context.Context, req any) (any, error) {
	r := req.(*GetUser)
	if r.ID <= 0 {
		return nil, errorx.New(errorx.InvalidArgument, "INVALID_ID", "id must be positive")
	}
	return &User{ID: r.ID, Name: "alice"}, nil
}

func run() error {
	var a *app.App
	srv := httpx.NewServer(
		httpx.Address("127.0.0.1:8000"),
		httpx.ReadHeaderTimeout(5*time.Second),
		httpx.ReadTimeout(10*time.Second),
		httpx.WriteTimeout(10*time.Second),
		httpx.IdleTimeout(time.Minute),
		httpx.MaxBodyBytes(64<<10),
		// GET /readyz answers 200 only while the App is running.
		httpx.Health(health.CheckerFunc(func(ctx context.Context) error { return a.Check(ctx) })),
	)
	// {id} and query parameters bind to the JSON fields of GetUser.
	httpx.Route[GetUser](srv, "GET /v1/users/{id}", getUser)

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

`curl http://127.0.0.1:8000/v1/users/7` 返回 `{"id":7,"name":"alice"}`；
`/v1/users/0` 返回 HTTP 400 和 errorx 错误体。对外提供服务前，请配置认证、授权和 TLS。
内存参考实现不提供持久化或跨进程协调。

另一个服务可通过客户端调用该路由：

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

需要服务注册时，将
`discovery.Registration(registrar, discovery.Instance{Name: "user"}, srv)`
放在 `app.WithHooks` 的服务端 Hook 之后。这样端口打开后才注册，关闭前先注销。

中间件使用 `endpoint.Middleware`，按传入顺序执行，首项为最外层。使用
servex-contrib 中间件时，建议顺序为：

```
server:   recovery -> tracing -> logging -> metrics -> auth -> ratelimit -> validate -> handler
client:   tracing -> logging -> metrics -> retry -> circuitbreaker -> timeout -> [select node -> invoke]
consumer: recovery -> tracing -> logging -> metrics -> handler
```

## HTTP 限制与自定义响应

服务端选项配置由 `Start` 使用的原生 `http.Server`：

```go
srv := httpx.NewServer(
	httpx.ReadHeaderTimeout(5*time.Second),
	httpx.ReadTimeout(30*time.Second),
	httpx.WriteTimeout(30*time.Second),
	httpx.IdleTimeout(60*time.Second),
	httpx.MaxHeaderBytes(1<<20),
	httpx.MaxBodyBytes(8<<20),
)
```

超时默认为零，遵循 `net/http` 语义：零值请求头和空闲超时继承 `ReadTimeout`，
负数禁用继承；零值请求头大小使用标准库默认值。由其他 `http.Server` 托管 Handler 时，
应在该服务器上配置传输选项。连接超时不等于处理函数的执行截止时间。

正数 `MaxBodyBytes` 限制 `Handle` 和 `Route` 的完整请求体，包含 multipart 边界
及没有请求工厂的路由。已知长度可提前拒绝，剩余内容在执行端点前以有界内存读取。
非正数禁用此限制。检测到超限时返回 HTTP 413 JSON，错误为 `InvalidArgument` 和
`BODY_TOO_LARGE`；畸形输入可能先返回 400。`HandleHTTP` 处理函数自行控制请求体限制。
Multipart 文件在端点执行和响应编码后清理。

通过路由响应编码器自定义成功状态码、响应头、Cookie 和响应体：

```go
httpx.Route[GetUser](srv, "POST /v1/users", getUser,
	httpx.ResponseEncoder(func(w http.ResponseWriter, r *http.Request, reply any) error {
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

## 配置校验与更新

普通配置加载使用 `config.New(ctx, sources...)`；需要拒绝无效初始值和重载时，使用校验器：

```go
cfg, err := config.NewWithOptions(ctx, []config.Source{config.Env("APP_")},
	config.WithValidator(func(ctx context.Context, candidate *config.Snapshot) error {
		var settings struct {
			Port int `json:"port"`
		}
		if err := candidate.Unmarshal("http", &settings); err != nil {
			return err
		}
		if settings.Port < 1 || settings.Port > 65535 {
			return errors.New("http.port must be between 1 and 65535")
		}
		return nil
	}),
)
if err != nil {
	return err
}
```

校验器按注册顺序处理完整合并后的候选配置，包括首次构造。只有加载、校验和最终上下文
检查均成功才发布；失败重载保留上一份有效值。校验器应解码到局部变量，不得重新进入同一
配置的 `Reload`。跨字段一致性读取使用 `cfg.Snapshot()`；`Value` 和 `Unmarshal`
复制配置容器。源应提供嵌套映射、列表和标量，不接受任意可变 Go 对象图。

`Watch` 保留成功回调；观察失败重载使用：

```go
err = cfg.WatchWithOptions(ctx, func(current *config.Config) {
	apply(current.Snapshot())
}, config.WithReloadErrorHandler(func(err error) {
	report(err)
}))
```

回调在重载锁外同步执行，同一次 Watch 的回调不重叠，源通知可以合并。
被拒绝的重载跳过成功回调，等待下次通知或手动 `Reload`；手动重载通过返回值报告错误。
取消不产生失败通知。终止性的源订阅错误由 `Watch` 返回；没有触发重载的源内部失败不在
此回调范围。`Env` 等只读源需要手动重载，或由其他源触发通知。
将有效配置应用到运行中组件仍由应用负责。

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
make check
make lint
make vulncheck
```

`check` 拒绝格式、现代化和 tidy 差异，检查普通及 `integration` 配置的 vet 和构建，
校验标准库与 kitx 依赖边界，并运行随机顺序的 race 测试。`lint` 和 `vulncheck`
也覆盖两种构建配置。所有目标使用 `GOWORK=off`。`fmt`、`fix` 和 `tidy` 会修改文件。

工具安装、变更要求和精确导入表见 [CONTRIBUTING.md](CONTRIBUTING.md)，未发布变更见
[CHANGELOG.md](CHANGELOG.md)，发布门槛见 [RELEASING.md](RELEASING.md)，
敏感问题见 [SECURITY.md](SECURITY.md)。核心无需服务端集成测试环境；CI 使用相同的
Make 目标。本地通过不代表远程 CI 已通过或模块已发布。

## 许可证

[MIT](LICENSE)。

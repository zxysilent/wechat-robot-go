# wechat-robot-go

[![CI](https://img.shields.io/github/actions/workflow/status/SpellingDragon/wechat-robot-go/ci.yml?branch=main&logo=github)](https://github.com/SpellingDragon/wechat-robot-go/actions)
[![Latest Release](https://img.shields.io/github/v/release/SpellingDragon/wechat-robot-go?logo=github)](https://github.com/SpellingDragon/wechat-robot-go/releases/latest)
[![Go Report Card](https://goreportcard.com/badge/github.com/SpellingDragon/wechat-robot-go)](https://goreportcard.com/report/github.com/SpellingDragon/wechat-robot-go)
[![GoDoc](https://pkg.go.dev/badge/github.com/SpellingDragon/wechat-robot-go.svg)](https://pkg.go.dev/github.com/SpellingDragon/wechat-robot-go)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
[![Go Version](https://img.shields.io/badge/Go-1.21%2B-blue)](https://github.com/SpellingDragon/wechat-robot-go)

> 基于腾讯 iLink Bot API 的微信机器人 Go SDK，几行代码接入微信消息。

[English](#english) | [中文](#中文)

---

[iLink API 参考](https://github.com/SpellingDragon/wechat-robot-go/wiki/iLink-API-Reference)

## ✨ 特性

- 📱 **扫码登录** - 自动持久化凭证，一劳永逸
- 🔄 **长轮询** - 实时接收消息，自动重连
- 🔑 **Context Token** - 持久化存储，支持主动推送
- ⌨️ **Typing 状态** - 显示"对方正在输入"
- 📎 **富媒体** - 图片/语音/视频/文件，AES-128 加密
- 📝 **长文本分割** - 智能分片，自然边界切分
- 🔧 **中间件** - 可组合的日志、限流、认证链
- 🛡️ **容错设计** - Panic 恢复，优雅关闭
- 📦 **零依赖** - 仅使用 Go 标准库

---

## 🚀 快速开始

### 安装

```bash
go get github.com/SpellingDragon/wechat-robot-go
```

### 最简示例

```go
package main

import (
    "context"
    "fmt"
    "os"
    "os/signal"
    "syscall"

    "github.com/SpellingDragon/wechat-robot-go/wechat"
)

func main() {
    bot := wechat.NewBot()

    ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
    defer cancel()

    // 扫码登录（首次需扫码，之后自动复用凭证）
    err := bot.Login(ctx, func(qrCode string) {
        fmt.Println("请使用微信扫描二维码:")
        fmt.Println(qrCode)
    })
    if err != nil {
        fmt.Fprintf(os.Stderr, "登录失败: %v\n", err)
        os.Exit(1)
    }

    // 注册消息处理器
    bot.OnMessage(func(ctx context.Context, msg *wechat.Message) error {
        text := msg.Text()
        if text == "" {
            return nil
        }
        return bot.Reply(ctx, msg, "收到: "+text)
    })

    fmt.Println("Bot 已启动，按 Ctrl+C 停止")
    bot.Run(ctx)
}
```

**运行效果：**

```
请使用微信扫描二维码:
iVBORw0KGgoAAAANSUhEUgAAAYYAAAGGCAYAAAB/gCbl...
Bot 已启动，按 Ctrl+C 停止
[用户] 你好
[Bot] 收到: 你好
```

---

## 📖 使用指南

### 发送文本消息

```go
// 回复用户（推荐）
bot.Reply(ctx, msg, "这是回复")

// 主动发送（需要先存储 context_token）
bot.SendTextToUser(ctx, "user@im.wechat", "定时推送")
```

### 发送图片

```go
// 一站式发送（推荐）
err := bot.SendImageFromPath(ctx, "user@im.wechat", "/path/to/image.png")

// 或先上传再发送
result, _ := bot.UploadFile(ctx, imageData, "user@im.wechat", "image")
imageItem := bot.Media().BuildImageItemPtr(result, 800, 600)
bot.SendImageToUser(ctx, "user@im.wechat", imageItem)
```

### 发送文件/语音/视频

```go
// 文件
bot.SendFileFromPath(ctx, "user@im.wechat", "/path/to/doc.pdf")

// 语音（duration: 毫秒）
bot.SendVoiceFromPath(ctx, "user@im.wechat", "/path/to/voice.silk", 5000)

// 视频
bot.SendVideoFromPath(ctx, "user@im.wechat", "/path/to/video.mp4")
```

### 使用中间件

```go
bot := wechat.NewBot()

// 内置中间件
bot.Use(
    wechat.WithRecovery(slog.Default()),  // Panic 恢复
    wechat.WithLogging(slog.Default()),   // 请求日志
)

bot.OnMessage(handler)
bot.Run(ctx)
```

### 限流中间件

```go
func RateLimiter(limit int, window time.Duration) wechat.Middleware {
    var (
        mu       sync.Mutex
        requests = make(map[string][]time.Time)
    )
    return func(next wechat.MessageHandler) wechat.MessageHandler {
        return func(ctx context.Context, msg *wechat.Message) error {
            mu.Lock()
            defer mu.Unlock()
            userID := msg.FromUserID
            now := time.Now()

            // 清理过期请求
            valid := requests[userID][:0]
            for _, t := range requests[userID] {
                if now.Sub(t) < window {
                    valid = append(valid, t)
                }
            }
            if len(valid) >= limit {
                return errors.New("rate limit exceeded")
            }
            requests[userID] = append(valid, now)
            return next(ctx, msg)
        }
    }
}

// 使用：每分钟最多 10 条消息
bot.Use(RateLimiter(10, time.Minute))
```

---

## ⚙️ 配置选项

| Option | 说明 | 默认值 |
|--------|------|--------|
| `WithBaseURL(url)` | API 服务器地址 | `https://ilinkai.weixin.qq.com` |
| `WithTokenFile(path)` | 凭证文件路径 | `.weixin-token.json` |
| `WithContextTokenDir(dir)` | context_token 目录 | `.wechat-context-tokens` |
| `WithHTTPClient(client)` | 自定义 HTTP 客户端 | `&http.Client{}` |
| `WithLogger(logger)` | 自定义日志 | `slog.Default()` |

---

## 🔧 Bot API

| 方法 | 说明 |
|------|------|
| `NewBot(opts ...Option)` | 创建 Bot 实例 |
| `Login(ctx, onQRCode)` | 扫码登录 |
| `OnMessage(handler)` | 注册消息处理回调 |
| `Use(middlewares ...)` | 注册中间件 |
| `Run(ctx)` | 启动（阻塞） |
| `Stop()` | 停止轮询 |
| `Reply(ctx, msg, text)` | 回复消息 |
| `SendTextToUser(ctx, userID, text)` | 主动发送 |
| `SendTyping(ctx, userID)` | 显示"正在输入" |
| `SendImageFromPath(ctx, userID, path)` | 发送图片 |
| `SendFileFromPath(ctx, userID, path)` | 发送文件 |

---

## 📁 项目结构

```
wechat/
├── internal/
│   ├── crypto/       # AES-128-ECB 加解密
│   ├── model/        # 消息数据类型
│   ├── store/        # ContextTokenStore
│   ├── middleware/   # 中间件框架
│   ├── text/         # 智能文本分片
│   └── media/        # CDN 上传下载
├── auth.go           # 登录认证
├── bot.go            # 门面 API
├── client.go         # HTTP 客户端
├── poller.go         # 长轮询
├── message_send.go   # 消息发送
├── typing.go         # Typing 状态
└── types.go          # 类型别名
```

---

## 📚 文档

- [Wiki 首页](https://github.com/SpellingDragon/wechat-robot-go/wiki)
- [架构设计](https://github.com/SpellingDragon/wechat-robot-go/wiki/Architecture)
- [中间件指南](https://github.com/SpellingDragon/wechat-robot-go/wiki/Middleware-Guide)

---

## 📄 License

MIT License - 详见 [LICENSE](LICENSE) 文件

---

<a name="english"></a>

# English

Go SDK for WeChat Bot based on Tencent's official iLink Bot API.

[iLink API Reference](https://github.com/SpellingDragon/wechat-robot-go/wiki/iLink-API-Reference)

## Features

- QR code login with automatic credential persistence
- Long-polling message reception with auto-reconnect
- Context token persistence for proactive messaging
- Typing indicator support
- Rich media: images, voice, video, files with AES-128 encryption
- Smart text splitting at natural boundaries
- Composable middleware (logging, rate limiting, auth)
- Panic recovery and graceful shutdown
- Zero external dependencies (Go stdlib only)

## Quick Start

```bash
go get github.com/SpellingDragon/wechat-robot-go
```

```go
bot := wechat.NewBot()
bot.Login(ctx, onQRCode)
bot.OnMessage(func(ctx context.Context, msg *wechat.Message) error {
    return bot.Reply(ctx, msg, "Echo: "+msg.Text())
})
bot.Run(ctx)
```

## Documentation

- [Wiki Home](https://github.com/SpellingDragon/wechat-robot-go/wiki)
- [Architecture](https://github.com/SpellingDragon/wechat-robot-go/wiki/Architecture)
- [Middleware Guide](https://github.com/SpellingDragon/wechat-robot-go/wiki/Middleware-Guide)
- [iLink API Reference](https://github.com/SpellingDragon/wechat-robot-go/wiki/iLink-API-Reference)

---

<a name="中文"></a>

# 中文
[iLink API Reference](https://github.com/SpellingDragon/wechat-robot-go/wiki/iLink-API-Reference)

基于腾讯 iLink Bot API 的微信机器人 Go SDK，几行代码接入微信消息。

## 背景

2026 年，腾讯通过 OpenClaw 框架正式开放了微信个人账号的 Bot API（iLink 协议）。这是微信首次提供合法的个人 Bot 开发接口，使用标准 HTTP/JSON 协议，接入域名为 `ilinkai.weixin.qq.com`。

## 解绑与重新绑定

iLink Bot API 没有服务器端解绑接口。解绑的本质是清除本地 `bot_token`。

```go
// 清除凭证
store := wechat.NewFileTokenStore(".weixin-token.json")
store.Clear()

// 重新登录
bot.Login(ctx, onQRCode)
```

或直接删除文件：

```bash
rm .weixin-token.json
```

## 持久化机制

### Bot Token

```json
// .weixin-token.json
{
  "bot_token": "eyJ...",
  "base_url": "https://ilinkai.weixin.qq.com"
}
```

### Context Token

```
// .wechat-context-tokens/{userID}.json
{
  "token": "AARzJWAFAAABAAAAAAAp...",
  "updated_at": "2026-03-24T10:30:00Z"
}
```

## 通信机制

### 长轮询

```
Bot → POST /ilink/bot/getupdates (hold 35s)
     ← 消息列表 或 超时重连
Bot → POST /ilink/bot/sendmessage
     ← 发送结果
```

### context_token

每条消息带有 `context_token`，用于：
1. 回复消息必须携带对应 token
2. 持久化后可主动推送
3. 进程重启后自动恢复

## 富媒体发送

```go
// 图片
bot.SendImageFromPath(ctx, userID, "/path/to/image.png")

// 文件
bot.SendFileFromPath(ctx, userID, "/path/to/doc.pdf")

// 语音 (duration: ms)
bot.SendVoiceFromPath(ctx, userID, "/path/to/voice.silk", 5000)

// 视频
bot.SendVideoFromPath(ctx, userID, "/path/to/video.mp4")
```

## 流式媒体下载

`Download*FromItemTo` / `DownloadVoiceTo` 系列方法以**常数内存**下载并解密媒体文件：密文边下载边解密，直接写入调用方提供的 `io.Writer`，不再把整个文件读入内存。

```go
// 下载文件到磁盘，并限制密文最大 100 MB
f, err := os.Create("/path/to/output.pdf")
if err != nil {
    return err
}
defer f.Close()

n, err := bot.DownloadFileFromItemTo(ctx, fileItem, cdnBaseURL, f,
    wechat.DownloadOptions{MaxSize: 100 << 20})
if err != nil {
    if errors.Is(err, wechat.ErrMaxSizeExceeded) {
        // 文件超过大小限制
    }
    // 契约：错误返回时 Writer 已收到的输出是不完整的，由调用方负责清理
    f.Close()
    os.Remove("/path/to/output.pdf")
    return err
}
// n 为写入 Writer 的明文字节数
```

四个流式方法与既有 `[]byte` 方法一一镜像：

| 既有方法 | 流式变体 |
|---|---|
| `DownloadImageFromItem` | `DownloadImageFromItemTo(ctx, cdnBaseURL, img, w, opts)` |
| `DownloadVoice` | `DownloadVoiceTo(ctx, voice, cdnBaseURL, w, opts)` |
| `DownloadFileFromItem` | `DownloadFileFromItemTo(ctx, file, cdnBaseURL, w, opts)` |
| `DownloadVideoFromItem` | `DownloadVideoFromItemTo(ctx, video, cdnBaseURL, w, opts)` |

### MaxSize 语义

- `DownloadOptions.MaxSize` 限制的是**密文**字节数，`0` 表示不限制。
- 若响应携带 `Content-Length` 且超限，将在**读取 body 之前**立即返回 `ErrMaxSizeExceeded`。
- 对于 chunked 无长度或谎报长度的响应，下载过程中密文累计超限时同样返回 `ErrMaxSizeExceeded`（流中截断）。
- 用 `errors.Is(err, wechat.ErrMaxSizeExceeded)` 判定。

### 错误时的输出契约

**错误返回时（包括 `ErrMaxSizeExceeded`），Writer 可能已经收到部分明文，该输出是不完整的，由调用方负责清理**（如删除半成品文件）。返回值 `int64` 始终等于已写入 Writer 的明文字节数。

### 声明大小

各媒体 Item 提供 `DeclaredSize() int64` 归一访问器，返回消息中声明的明文大小（字节），`0` 表示未知/缺失，可用于预先设定 `MaxSize` 或跳过超大文件：

```go
if size := fileItem.DeclaredSize(); size > 0 && size > 100<<20 {
    // 声明大小超过 100 MB，直接跳过
}
```

- `FileItem.DeclaredSize()`：解析 `Length` 字符串，空/非法返回 0
- `VoiceItem.DeclaredSize()`：`FileSize`
- `VideoItem.DeclaredSize()`：`VideoSize`
- `ImageItem.DeclaredSize()`：`HDSize` 优先，为 0 时退回 `MidSize`

注意：声明大小来自消息元数据，仅供参考，不参与 `MaxSize` 强制（强制以实际密文字节数为准）。

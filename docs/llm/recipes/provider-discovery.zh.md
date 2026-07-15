# 模型与鉴权发现

## 本场景实现什么

诊断命令只列出当前进程已导入 adapter 能路由的模型，并在不发送请求的情况下报告 provider 凭证状态。

模型选择 UI 和启动检查应基于这条路径。只使用 `GetModels` 会包含没有内置 adapter 的目录协议。

## 完整程序

```go
package main

import (
	"fmt"

	"github.com/ktsoator/or/llm"
	_ "github.com/ktsoator/or/llm/all"
)

func main() {
	providers := llm.GetProviders()
	registry := llm.DefaultProviderRegistry()
	for _, providerID := range providers {
		models := llm.GetRunnableModels(providerID)
		if len(models) == 0 {
			continue
		}
		status, ok := registry.AuthStatus(providerID, nil)
		if !ok {
			fmt.Printf("%s: provider is not registered\n", providerID)
			continue
		}
		fmt.Printf("%s configured=%t source=%q missing=%v\n",
			providerID, status.Configured, status.Source, status.Missing)
		for _, model := range models {
			fmt.Printf("  %s protocol=%s context=%d image=%v reasoning=%t\n",
				model.ID, model.Protocol, model.ContextWindow,
				model.Input, model.Reasoning)
		}
	}
}
```

## 目录与运行时是两层状态

| API | 回答的问题 |
|---|---|
| `GetProviders` | 嵌入目录有哪些 provider ID？ |
| `GetModels(provider)` | 目录中有哪些模型，包括未实现协议？ |
| `GetRunnableModels(provider)` | 默认 adapter registry 当前能路由哪些模型？ |
| `SupportsProtocol(protocol)` | 是否导入了该协议 adapter？ |
| `AuthStatus(provider, env)` | Provider 能否解析凭证，凭证来自哪里？ |

`AuthStatus` 报告 `env:DEEPSEEK_API_KEY` 或 `override` 等来源，但不会发送请求。配置了凭证不代表凭证未过期、具有模型权限或一定会被 endpoint 接受。

诊断中不要暴露 `GetEnvAPIKey` 返回的 secret。应展示 `APIKeyEnvVars` 给出的变量名和 `AuthStatus` 的缺失项。

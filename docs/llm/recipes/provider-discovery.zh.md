# 查找模型与检查凭证

应用可以在启动时列出当前可调用的模型，并检查各提供方是否已配置凭证。整个过程不发送模型请求。

模型选择界面和启动检查应使用 `GetRunnableModels`，而不是只使用 `GetModels`。后者还会返回当前程序没有注册协议适配器的模型。

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

## 模型是否可调用

| API | 回答的问题 |
|---|---|
| `GetProviders` | 内置模型清单中有哪些提供方 ID？ |
| `GetModels(provider)` | 清单中收录哪些模型，包括当前不能调用的模型？ |
| `GetRunnableModels(provider)` | 默认协议适配器注册表当前能路由哪些模型？ |
| `SupportsProtocol(protocol)` | 当前程序是否已注册该协议的适配器？ |
| `AuthStatus(provider, env)` | 提供方能否解析凭证，凭证来自哪里？ |

模型被内置模型清单收录，并不表示当前程序可以调用它。调用模型还需要对应的协议适配器已注册；`llm/all` 会注册全部内置协议，按需导入单个协议包则只注册对应协议。

## 检查凭证

`AuthStatus` 会报告诸如 `env:DEEPSEEK_API_KEY` 或 `override` 的凭证来源，但不会发送请求。已配置凭证不代表它未过期、有权调用该模型，或一定会被模型服务接受。

诊断界面不要展示 `GetEnvAPIKey` 返回的密钥值。可展示 `APIKeyEnvVars` 给出的变量名，以及 `AuthStatus` 中缺失的变量名。

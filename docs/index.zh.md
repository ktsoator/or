# or

从意图到行动，自由选择路径——一个模块化的 Go 工具包，用于构建基于语言模型和上层
agent 的应用。

[快速开始](llm/getting-started.md){ .md-button .md-button--primary }
[在 GitHub 上查看](https://github.com/ktsoator/or){ .md-button }

<div class="grid cards" markdown>

- __不绑定厂商__

    一套对话模型即可覆盖 OpenAI 兼容和 Anthropic 兼容的多家提供方，并可在不同
    轮次之间切换模型，无需重建历史。

- __类型化流式__

    文本、推理、工具调用、用量和错误均通过类型化事件流式输出，每个事件都携带一份
    当前响应的部分快照。

- __结构化工具__

    用 Go 结构体定义工具，并校验模型生成的参数；对被截断的流也能尽力恢复。

- __感知推理__

    与厂商无关的推理强度会映射到各提供方的原生思考形式，并保留多轮连续性所需的元数据。

</div>

## 软件包

| 软件包 | 状态 | 说明 |
|---|---|---|
| [`or/llm`](llm/README.md) | 可用 | 统一的模型访问、流式、工具、推理、图像与对话历史 |
| [`or/agent`](agent/README.md) | 可用 | 带工具、流式事件、干预、追加消息和中止能力的有状态 agent 循环 |

除包使用指南外，[源码解析](internals/overview.md)章节深入讲解 `llm` 包的内部工作原理，
[笔记](notes/README.md)则收集了用本工具包构建过程中沉淀的模式与开放问题。

完整的导出类型和函数，参见
[pkg.go.dev](https://pkg.go.dev/github.com/ktsoator/or/llm) 上的包文档。

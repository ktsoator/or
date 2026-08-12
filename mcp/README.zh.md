# Or MCP 目录

此目录用于维护适配 Or 的精选 MCP 服务器配置。每个条目记录服务器的官方来源、
Or 可直接使用的连接模板、安装前置条件，以及用户在添加前需要确认的安全信息。

这里保存的是配置模板，不是已安装配置。Or 仍然只从自己的私有配置文件加载已启用
的 MCP 服务器：

- 默认位置：`~/.or/coding/mcp.json`
- 自定义数据目录：`$OR_DATA_DIR/mcp.json`

目录中不得保存凭据。模板使用 `${env:NAME}` 引用已有的进程环境变量。将来实现安装
功能时，应先使用模板打开现有 MCP 编辑弹窗，让用户检查并明确确认后再保存和连接。

## 已收录服务器

| 服务器 | 连接方式 | 身份验证 | 上游版本 |
|---|---|---|---|
| [GitHub](servers/github/README.md) | Streamable HTTP | Personal access token | [`github/github-mcp-server@eff4c3c`](https://github.com/github/github-mcp-server/tree/eff4c3c041742426f417f7c2247b96bbf6d60b69) |

## 目录结构

```text
mcp/
├── README.md
├── README.zh.md
└── servers/
    └── <name>/
        ├── manifest.json
        └── README.md
```

`manifest.json` 是未来目录界面的机器可读数据，其中 `server.config` 与 Or MCP 编辑器
和 `mcp.json` 使用相同字段。相邻的 README 用于说明不应放入可执行配置的安装决策。

## 收录要求

- 目录名和 `id` 使用稳定的小写标识。
- 链接可信的官方上游仓库，并固定实际审查过的 revision。
- 记录上游许可证；只有确实需要分发上游代码且完整保留许可证和声明时才复制源码。
- 只收录 Or 支持的 stdio 或 Streamable HTTP 配置。
- 密钥统一使用 `${env:NAME}`，不得提交 token、密码、Cookie 或私有服务地址。
- 如果服务器支持，应优先使用最小权限和只读默认配置。
- 如实说明网络、进程、文件系统和写入能力。
- 收录前应基于固定的上游 revision 审查命令、容器镜像、包名、URL 和参数。
- 安装必须是用户确认过的操作。仅浏览目录不得建立连接、执行命令或修改
  `mcp.json`。


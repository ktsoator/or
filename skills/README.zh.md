# Or Skills

此目录用于维护和分发 Or 项目精选的 Agent Skills。每个一级子目录都是一个完整的
Skill 包，并遵循开放的
[Agent Skills 规范](https://agentskills.io/specification)。

这里是 Skill 源码集合，不是运行时发现目录。Or 只会从以下标准位置加载已安装的
Skill：

- 用户级 Skill：`~/.agents/skills/<name>/SKILL.md`
- 工作区 Skill：`<workspace>/.agents/skills/<name>/SKILL.md`

在 Or 提供安装器之前，需要将选中的 Skill 目录复制到上述任一位置。

## 目录结构

```text
skills/
└── <name>/
    ├── SKILL.md
    ├── scripts/       # 可选
    ├── references/    # 可选
    └── assets/        # 可选
```

## 收录要求

- 每个 Skill 必须完整放在一个一级子目录中。
- 目录名必须与 `SKILL.md` frontmatter 中的 `name` 完全一致。
- 只使用 Agent Skills 规范定义的字段。版本信息写入 `metadata.version`，不要使用
  顶层 `version` 字段。
- 不添加 Or 私有兼容字段、Prompt Template 参数替换或额外运行时路径。
- 从第三方引入 Skill 时保留来源和许可证信息，只收录项目有权再分发的内容。
- 收录前审查附带脚本。读取 Skill 不代表其脚本获得执行权限。
- 不提交凭据、生成产物、依赖缓存或本地配置。

如果某个 Skill 只用于指导本仓库的开发工作，应放入 `.agents/skills/`，而不是这个
分发集合。

# Whale

[English](README.md)

面向终端的 DeepSeek 原生编程代理。

Whale 是一个本地 CLI/TUI 编程代理，围绕 DeepSeek 的前缀缓存行为、追加式 turn，以及终端优先工作流来设计。

## 安装

**安装最新发布版本：**

```bash
curl -fsSL https://raw.githubusercontent.com/usewhale/whale/main/scripts/install.sh | sh
whale --version
```

安装脚本会从 GitHub Releases 下载匹配的发布二进制，并根据 `checksums.txt` 进行校验。

**使用 Go 从源码安装：**

```bash
go install github.com/usewhale/whale/cmd/whale@latest
whale --version
```

你需要 Go `1.26.2` 或更新版本。

## 快速开始

Whale 当前使用 DeepSeek API。

在运行 Whale 之前，请先在
[DeepSeek Platform](https://platform.deepseek.com/)
创建一个 DeepSeek API key。
API 细节请查看 [DeepSeek API docs](https://api-docs.deepseek.com/)。

**保存 key 供后续会话使用：**

```bash
whale setup
```

**或者按进程传入：**

```bash
DEEPSEEK_API_KEY=... whale
```

**运行健康检查：**

```bash
whale doctor
```

**启动交互式 TUI：**

```bash
whale
```

**以非交互方式运行单条 prompt：**

```bash
whale exec "Explain what this repository does"
whale exec --json "Say exactly: whale exec ok"
printf 'Summarize the current directory\n' | whale exec
```

## Whale 是什么

Whale 优化的是 **DeepSeek 特定行为**，而不是做一个通用 provider 抽象层。

- **前缀缓存** 在循环保持追加式、字节稳定时更有价值。
- DeepSeek 有时会生成格式错误或被转义的 tool-call payload；Whale 为此内置了 **repair 和 scavenge 路径**。
- DeepSeek 会丢失某些嵌套较深的 tool schema；Whale 会把工具参数扁平化，以降低这种失败模式。
- 推理深度通过 `reasoning_effort` 暴露，所以 Whale 把这项控制保留在运行时。

## 主要特性

- **交互式终端工作流**，带本地 TUI 和会话恢复。
- `setup`、`doctor`、`exec` 入口，覆盖首次配置、诊断和无头执行。
- 面向 DeepSeek 行为优化的工具循环，内置 **shell、文件、patch、搜索和 web 工具**。
- 从 `AGENTS.md` 等常见仓库说明文件加载 project memory。
- 通过 hook 支持策略与工作流定制。
- 仓库内置离线 eval 脚手架和针对 TUI 的测试覆盖。

## 项目状态

⚠️ **早期开发提示：** 该项目仍处于早期开发阶段，尚未准备好用于生产环境。功能可能变化、损坏或不完整。请自行承担使用风险。

## 常用命令

- `whale` — 启动交互式 TUI
- `whale setup` — 保存 DeepSeek API key
- `whale doctor` — 运行健康检查
- `whale exec "prompt"` — 以非交互方式运行单条 prompt
- `whale resume [id]` — 恢复已保存会话

## 配置与 hooks

Whale 会把本地状态存放在 `~/.whale/` 下，并支持可选的 project 级和 global 级 hook 文件。

**更多细节见 [docs/configuration.md](docs/configuration.md)：**

- API key 与凭证行为
- project 和 global hook 文件
- hook 事件名
- 运行时配置说明

## 参与贡献

关于克隆、开发、测试、issues 和 pull requests，请查看 [CONTRIBUTING.md](CONTRIBUTING.md)。

## 安全

如果是安全相关问题，请查看 [SECURITY.md](SECURITY.md)。

# Agent hooks

[Codex hooks](codex.json) and [Claude hooks](claude.json) check for new client
responses after tool calls and before ending a turn. The hook also reminds the
agent when its last status update is at least five minutes old, at most once
every five minutes. It does not consume responses; the agent receives them
through `responses_wait`.

Install `opeco` using the [CLI installation instructions](../../README.md#install)
and configure it as described in [MCP server setup](../../README.md#mcp-server),
using the server name `opeco-link` and `args = ["mcp"]`.

Merge the `hooks` object from your agent's JSON file into its hook configuration,
preserving existing hooks. For Codex, follow [Codex hooks](https://learn.chatgpt.com/docs/hooks) and
review the configuration with `/hooks`. For Claude, merge it into
`.claude/settings.json` as described in [Claude hooks](https://code.claude.com/docs/en/hooks).

Pair a client normally and send feedback while the agent is working. Hooks run
at lifecycle boundaries; they cannot immediately interrupt a running operation
or wake an idle agent. A Stop hook asks the agent to continue at most once.

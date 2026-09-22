# Engineering Handbook

How gokvx is built. The [specification](../requirements.md) says *what* the system must do; this handbook says *how* we write, test, and deliver it. The binding summary of both is [AGENTS.md](../../AGENTS.md).

| Document | Covers |
|---|---|
| [product-context.md](product-context.md) | Mission, positioning, design goals, non-goals, components, and phases |
| [system-invariants.md](system-invariants.md) | The technical rules the system must never violate, with requirement references |
| [go-standards.md](go-standards.md) | Go code standards: layout, comments, size limits, constructors, context, concurrency, observability, security |
| [error-handling.md](error-handling.md) | The unified error model and how errors cross gRPC and HTTP boundaries |
| [testing.md](testing.md) | Test strategy, conventions, and requirement traceability |
| [dependencies.md](dependencies.md) | The dependency allowlist and the technology decisions behind it |
| [workflow.md](workflow.md) | Branches, commits, pull requests, definition of done, and tooling |

Changes to this handbook go through pull requests like code. A rule that no longer serves the project is changed here, explicitly — never ignored quietly.

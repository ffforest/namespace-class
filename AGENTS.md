# 项目须知

## 文档索引

- 项目目标与快速入口：`README.md`
- 稳定领域语言与边界：`CONTEXT.md`
- 当前设计方案：`docs/design/namespaceclass-design.md`
- 实施计划目录：`docs/plans/`
- ADR 目录：`docs/adr/`
- 进度、TODO、完成记录和经验：`docs/progress/progress.md`、`docs/progress/todos.md`、`docs/progress/done.md`、`docs/progress/learnings.md`

## 工作流程

- 新增功能前，先判断是否影响 `docs/design/namespaceclass-design.md` 中的 API、controller reconciliation、inventory、RBAC 或测试 harness；影响这些边界时先更新设计或写 ADR。
- 较大改动进入实现前，先在 `docs/plans/{日期}-{主题}.md` 写实施计划，明确验收命令。
- 代码改动默认遵循 TDD：先写能失败的测试或集群 smoke，再实现最小代码让它通过，最后重构并运行最小充分 `make` 目标。
- 纯文档、注释或机械格式改动不需要强行补测试；涉及 controller 行为、CRD/schema、status、inventory、RBAC、模板渲染或 Helm manifest 时，必须先有对应单元测试、envtest 或 smoke 验证。
- 不要绕过 Makefile 约定写临时验证脚本；确实需要新增脚本时放入 `scripts/` 并接入 Makefile。
- 不要把本地 minikube 状态、kubeconfig、下载的工具、构建产物或日志提交进仓库。

## 验证规则

- 纯文档改动：运行 `make docs-check`。
- Go 代码改动：至少运行 `make test` 和 `make vet`；涉及 Kubernetes API、CRD、status 或 reconciler 行为时补充 `make envtest`。
- manifest、CRD、Helm chart 改动：运行 `make manifests-check` 和 `make helm-template`。
- controller 行为或集群交互改动：在本地 minikube 上运行 `make cluster-check`、`make deploy-crds` 和 `make smoke`。
- 收尾优先运行 `make check`；它包含普通 Go 单测、envtest 集成测试、vet、manifest check 和 Helm render。如果因为本机缺少 Go、kubectl、helm、envtest assets 或 minikube 无法运行，在最终说明里写清楚缺失项和已完成的替代验证。

## 状态同步

- 新增待办写入 `docs/progress/todos.md`。
- 完成待办后移动到 `docs/progress/done.md`，保留日期和简短说明。
- 重要设计取舍写入 `docs/adr/`。
- 可复用的调试经验写入 `docs/progress/learnings.md`。

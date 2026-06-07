# NamespaceClass 设计文档

## 1. 背景

Kubernetes 管理员希望为 namespace 定义一组可复用的“class”。每个 `NamespaceClass` 描述一组附加资源、策略或配置。当某个 `Namespace` 绑定到某个 class 时，controller 自动在该 namespace 中创建并维护这些资源。

示例：

- `public-network`：创建允许公网访问的 `NetworkPolicy`
- `internal-network`：创建仅允许公司 VPN 访问的 `NetworkPolicy`

该设计的核心不是 hard-code 某几类资源，而是提供一个通用机制：管理员可以在 `NamespaceClass` 中声明任意 Kubernetes 资源模板，controller 根据 namespace 的 class label 维护实际资源。

## 2. 目标

本设计需要满足以下目标：

1. 提供 `NamespaceClass` CRD，用于声明 namespace class 及其附属资源模板。
2. 监听 `Namespace` 创建和更新，根据 namespace label 创建对应 class 的资源。
3. 支持 namespace 切换 class，并清理旧 class 产生但新 class 不再需要的资源。
4. 支持 `NamespaceClass` 更新后，同步所有使用该 class 的 namespace。
5. 支持任意 Kubernetes 资源类型，包括 namespaced resources 和 cluster-scoped resources，而不限于 `NetworkPolicy`、`ServiceAccount` 等固定 kind。
6. 尽量遵循 Kubernetes controller 的 declarative reconciliation 模型。

## 3. 非目标

第一版不解决以下问题：

1. 不提供复杂模板语言，只支持最小变量替换，例如 namespace name。
2. 不做多层 class 继承。
3. 不做跨集群分发。
4. 不提供图形化管理界面。
5. 不试图接管已经存在但不是 controller 管理的资源。
6. 不保证所有集群级资源都能无冲突地自动命名，模板作者需要为 cluster-scoped resources 负责。

## 4. API 设计

### 4.1 NamespaceClass

`NamespaceClass` 是 cluster-scoped CRD。

建议 API group：

```yaml
apiVersion: namespaceclass.akuity.io/v1alpha1
kind: NamespaceClass
metadata:
  name: public-network
spec:
  resources:
    - apiVersion: networking.k8s.io/v1
      kind: NetworkPolicy
      metadata:
        name: allow-public-ingress
      spec:
        podSelector: {}
        policyTypes:
          - Ingress
        ingress:
          - {}
```

Go 类型可以表达为：

```go
type NamespaceClassSpec struct {
    Resources []runtime.RawExtension `json:"resources,omitempty"`
}
```

CRD schema 对 `spec.resources` 使用 preserve unknown fields，允许保存任意 Kubernetes object：

```yaml
spec:
  type: object
  properties:
    resources:
      type: array
      items:
        type: object
        x-kubernetes-preserve-unknown-fields: true
```

### 4.2 Namespace 绑定方式

namespace 通过 label 绑定 class：

```yaml
apiVersion: v1
kind: Namespace
metadata:
  name: web-portal
  labels:
    namespaceclass.akuity.io/name: public-network
```

label key：

```text
namespaceclass.akuity.io/name
```

选择 label 而不是 annotation 的原因是：controller 需要高效判断 namespace 是否绑定了 class，并且后续可以通过 label selector 或 informer index 做批量查询。第一版实际 fan-out 使用 `NamespaceClassBinding.spec.className` 索引，因为 binding 已经记录了 controller 观察到的当前绑定状态和 inventory。

### 4.3 NamespaceClassBinding

controller 为每个被 `NamespaceClass` 管理过的 namespace 创建一个 cluster-scoped `NamespaceClassBinding`。该对象不是用户主要操作入口，而是 controller 的持久状态对象，用来保存绑定状态、同步状态和 inventory。

示例：

```yaml
apiVersion: namespaceclass.akuity.io/v1alpha1
kind: NamespaceClassBinding
metadata:
  name: web-portal
spec:
  namespaceName: web-portal
  className: public-network
status:
  observedNamespaceUID: "..."
  observedClassGeneration: 3
  inventory:
    - apiVersion: networking.k8s.io/v1
      kind: NetworkPolicy
      namespace: web-portal
      name: allow-public-ingress
  conditions:
    - type: Ready
      status: "True"
      reason: ReconcileSucceeded
```

`NamespaceClassBinding` 选择 cluster-scoped 的原因是：

1. namespace 删除时，namespace 内对象会被清理；如果 inventory 存在 namespace 内，controller 可能丢失清理 cluster-scoped resources 所需的信息。
2. `NamespaceClass` 本身是 cluster-scoped，对应 binding 也更适合表达集群级状态。
3. 管理员可以直接查询每个 namespace 的 class 绑定、同步 generation、inventory 和错误 conditions。

binding 的 name 可以直接使用 namespace name，因为 namespace name 在集群内唯一。

## 5. 资源模板规则

`NamespaceClass.spec.resources` 中的每个对象都必须包含：

```yaml
apiVersion: ...
kind: ...
metadata:
  name: ...
```

controller 对模板进行如下处理：

1. 如果资源是 namespaced resource，强制设置 `metadata.namespace` 为目标 namespace。
2. 如果资源是 cluster-scoped resource，不设置 namespace。
3. 如果模板中已经为 namespaced resource 写了别的 namespace，controller 拒绝该模板或覆盖为目标 namespace。推荐拒绝并记录错误，因为这通常代表管理员配置有误。
4. controller 为创建的资源添加统一的 labels 和 annotations。
5. controller 不允许模板覆盖内部管理用的 labels 和 annotations。

controller 添加的元数据示例：

```yaml
metadata:
  labels:
    namespaceclass.akuity.io/managed: "true"
    namespaceclass.akuity.io/class: public-network
    namespaceclass.akuity.io/namespace: web-portal
  annotations:
    namespaceclass.akuity.io/owner-namespace-uid: "<namespace-uid>"
```

### 5.1 模板变量

第一版只支持非常小的变量集合，避免演变成复杂模板系统。

支持变量：

```text
{{ .Namespace.Name }}
{{ .Namespace.UID }}
{{ .Namespace.Labels.<key> }}
{{ .Namespace.Annotations.<key> }}
{{ .Class.Name }}
```

约束：

1. 变量只能用于字符串字段，例如 `metadata.name`、labels、annotations 和 spec 内的字符串值。
2. 不支持条件、循环、函数、外部数据引用或跨资源引用。
3. 渲染失败时整个 `NamespaceClass` 被认为无效，controller 不应部分执行该 class。

该变量集合主要用于 cluster-scoped resources 的唯一命名。例如：

```yaml
metadata:
  name: "{{ .Namespace.Name }}-public-access"
```

### 5.2 Validating Admission

由于 `spec.resources` 使用 raw Kubernetes objects，CRD schema 本身无法完成足够强的校验。建议提供 validating admission webhook，在资源进入集群前尽早发现错误。

webhook 至少校验：

1. 每个 resource 都包含 `apiVersion`、`kind`、`metadata.name`。
2. 同一个 `NamespaceClass` 中不能出现重复的 resource identity。
3. 模板不能设置 controller 保留的 labels 和 annotations。
4. namespaced resource 不能写入其他 namespace。
5. 模板变量必须可解析，且只能出现在允许的字符串字段中。
6. GVK 必须符合 controller 配置的 allowlist/denylist 策略。

webhook 无法替代 reconcile 阶段的运行时校验，因为 API discovery、RBAC、quota、admission webhook 和目标对象冲突仍然可能在 apply 时失败。

## 6. Controller 架构

### 6.1 Watch 对象

controller 监听：

1. `Namespace`
2. `NamespaceClass`
3. `NamespaceClassBinding`

核心 reconcile key 是 namespace name。即使事件来自 `NamespaceClass`，也会被转换成一个或多个 namespace reconcile 请求。

对于 `NamespaceClass.spec.resources` 中的任意 GVK，第一版不动态 watch 每一种子资源，只依赖 `Namespace`、`NamespaceClass`、`NamespaceClassBinding` 事件和周期性 resync 修复 drift。原因是 controller-runtime 对编译期已知类型支持很好，例如 `Namespace`、`Deployment`、`NetworkPolicy`；但 `NamespaceClass` 允许管理员在运行时引用任意 GVK，包括新安装的 CRD。要实时 watch 这些资源，需要使用 dynamic informer 和 dynamic client 针对 `unstructured.Unstructured` 在运行时创建 watch，这会引入 RESTMapper/discovery cache、GVK 增删、informer 生命周期、RBAC 失败和内存占用等复杂度。

因此第一版选择更简单的策略：

1. 创建、class 切换、class 更新通过主要对象事件立即触发。
2. 用户手动删除或修改 managed resource 后，通过周期性 resync 最终修复。
3. dynamic informer 作为后续增强，用于缩短 drift 修复延迟。

### 6.2 Namespace Reconciler

Namespace reconcile 是唯一负责实际 create/update/delete 的路径。

流程：

```text
1. 读取 Namespace
2. 如果 Namespace 正在删除，执行清理逻辑，然后返回
3. 读取 label namespaceclass.akuity.io/name
4. 如果 label 不存在：
   4.1 读取 NamespaceClassBinding.status.inventory
   4.2 删除此前由 NamespaceClass 管理的资源
   4.3 清空 inventory 并删除 NamespaceClassBinding
   4.4 返回
5. 如果 label 存在：
   5.1 读取对应 NamespaceClass
   5.2 确保 NamespaceClassBinding 存在，并读取旧 status.inventory
   5.3 将 spec.resources 转换为 desired resources
   5.4 对 desired resources 做 server-side apply
   5.5 删除旧 inventory 中存在但 desired set 中不存在的资源
   5.6 写回 NamespaceClassBinding.status.inventory 和 conditions
```

如果 namespace 仍然存在但 `NamespaceClass` 不存在，controller 将 desired set 视为空，根据 binding 中的 inventory 清理已管理资源。清理成功后删除 `NamespaceClassBinding`；清理失败时保留 binding，并写入 `Ready=False` / `CleanupFailed` condition 以便重试和排查。

### 6.3 NamespaceClass 事件处理

当某个 `NamespaceClass` 被创建、spec 更新或删除时：

```text
1. 通过 NamespaceClassBinding.spec.className=<class-name> 索引找出所有 binding
2. 从 binding.spec.namespaceName 得到目标 Namespace
3. 将这些 Namespace enqueue
4. 由 Namespace Reconciler 完成实际同步
```

这样可以让 namespace 创建、class 切换、class 更新和 class 删除复用同一套 reconciliation 逻辑。class 删除事件触发 fan-out 后，Namespace Reconciler 会发现 class 不存在，并按空 desired set 清理 binding inventory 中的资源。

使用 binding 索引而不是每次扫描所有 namespace 的原因是：

```text
1. binding 已经是当前绑定状态和 inventory 的 source of truth
2. class 更新时只需要触达已经被 controller 管理过的 namespace
3. 大规模 namespace 场景下避免全量 namespace list/filter
4. namespace label 切换仍然由 Namespace watch 负责创建或更新 binding
```

如果将来需要处理“NamespaceClass 创建时，namespace 已经带 label 但 binding 尚不存在”的特殊恢复场景，可以再增加 namespace label index fan-out 或周期性全量扫描。

## 7. Inventory 设计

controller 需要知道自己上一次为某个 namespace 创建过哪些资源。否则当 class 更新或 namespace 切换 class 时，旧资源可能已经不在新 class spec 中，controller 无法仅凭新 spec 找到需要删除的对象。

第一版使用 cluster-scoped `NamespaceClassBinding.status.inventory` 保存 inventory：

```yaml
apiVersion: namespaceclass.akuity.io/v1alpha1
kind: NamespaceClassBinding
metadata:
  name: web-portal
spec:
  namespaceName: web-portal
  className: public-network
status:
  inventory:
    - apiVersion: networking.k8s.io/v1
      kind: NetworkPolicy
      namespace: web-portal
      name: allow-public-ingress
```

资源 identity 使用：

```text
apiVersion + kind + namespace + name
```

不能只使用 `name`，因为不同 kind 可以同名。

对于 cluster-scoped resources，`namespace` 字段为空。binding 的 inventory 只保存 identity，不保存完整对象，以降低对象大小和状态漂移风险。

## 8. Apply 与 Delete 策略

### 8.1 Server-Side Apply

创建和更新资源推荐使用 server-side apply，field manager 使用固定名称：

```text
namespaceclass-controller
```

好处：

1. 避免简单 update 覆盖用户不相关字段。
2. 可以通过 field ownership 发现字段冲突。
3. 符合 Kubernetes controller 管理声明式资源的常见方式。

### 8.2 已存在资源处理

如果目标资源已经存在：

1. 如果带有 controller 的 managed marker，controller 可以 apply 更新。
2. 如果不带 managed marker，controller 不应接管，应该记录 conflict。

这避免 controller 意外覆盖用户手动创建的资源。

### 8.3 删除策略

删除只基于 inventory 中记录的资源，并且删除前再次检查资源是否仍然带有 controller 的 managed marker。

如果对象存在但 marker 已被移除，controller 不删除它，并记录 warning。

当 `NamespaceClass` 被删除时，本设计的默认行为是清理所有仍引用该 class 的 namespace 对应的 managed resources。具体做法是将 desired set 视为空，根据 `NamespaceClassBinding.status.inventory` 删除已管理资源。该行为风险较高，尤其当 class 中包含 cluster-scoped resources 时，管理员误删 class 可能触发大规模删除。保守替代方案是保留资源并把 binding 标记为 `ClassNotFound`，要求管理员显式设置 cleanup policy 后再删除。本设计选择默认删除，但将其列为明确风险项。

## 9. Switching Classes

namespace class 从 `public-network` 切换到 `internal-network` 时：

```text
旧 inventory:
  NetworkPolicy/allow-public-ingress

新 desired set:
  NetworkPolicy/allow-vpn-only

controller:
  apply NetworkPolicy/allow-vpn-only
  delete NetworkPolicy/allow-public-ingress
  update inventory
```

不需要单独保存旧 class 名。只要 inventory 准确，diff 就足够处理 class 切换。

## 10. Updating Classes

`NamespaceClass` 更新时：

```text
1. NamespaceClass event handler 收到更新事件
2. 使用 NamespaceClassBinding.spec.className 索引找到所有引用该 class 的 binding
3. enqueue 每个 binding.spec.namespaceName 对应的 Namespace
4. Namespace Reconciler 对每个 namespace 重新计算 desired set
5. 创建新增资源，更新已有资源，删除被移除资源
```

例如：

```text
old public-network:
  NetworkPolicy/allow-public-ingress

new public-network:
  NetworkPolicy/allow-public-ingress
  ServiceAccount/public-app

结果:
  已有 NetworkPolicy 被 apply 更新
  新 ServiceAccount 被创建
  inventory 被更新
```

## 11. 状态与可观测性

第一版以 `NamespaceClassBinding` 作为主要状态对象，并辅以 Kubernetes Events、controller logs 和 metrics。

`NamespaceClassBinding.status` 至少包含：

```yaml
status:
  observedNamespaceUID: "..."
  observedClassGeneration: 3
  inventory:
    - apiVersion: networking.k8s.io/v1
      kind: NetworkPolicy
      namespace: web-portal
      name: allow-public-ingress
  conditions:
    - type: Ready
      status: "False"
      reason: ApplyConflict
      message: "NetworkPolicy/allow-public-ingress has field ownership conflict"
```

binding 解决的是 durable per-namespace status：管理员可以查询某个 namespace 当前绑定哪个 class、同步到哪个 class generation、有哪些 managed resources、最近一次失败原因是什么。

建议记录的事件：

1. `NamespaceClassNotFound`
2. `ResourceApplyFailed`
3. `ResourceDeleteFailed`
4. `ResourceConflict`
5. `InventoryCorrupted`
6. `ReconcileSucceeded`

metrics 用于表达全局运行状态，例如 queue depth、reconcile latency、apply error count、delete error count、conflict count 和 class fan-out count。

`NamespaceClass.status` 可以作为后续增强，用于汇总引用该 class 的 namespace 数量、ready 数量和 degraded 数量，但第一版不依赖它完成核心功能。

## 12. 安全与权限

由于 `NamespaceClass` 支持任意资源，controller 的 RBAC 权限是重要风险点。

需要考虑：

1. controller 需要管理多种 namespaced resources 和 cluster-scoped resources。
2. controller 可能需要广泛的 cluster-level write 权限。
3. 管理员可以通过 `NamespaceClass` 间接让 controller 创建高权限资源，例如 `RoleBinding` 或 `ClusterRoleBinding`。
4. cluster-scoped resources 存在全局命名冲突和大范围误删风险。

推荐策略：

1. `NamespaceClass` 的写权限只授予集群管理员。
2. controller 支持可配置的 GVK allowlist/denylist。产品能力支持任意资源，但具体部署可以通过策略限制可执行的 GVK。
3. 对高风险资源，例如 `ClusterRoleBinding`，需要显式 allow。
4. 所有 managed resources 必须带 ownership marker 和 namespace UID annotation。
5. 所有 apply/delete 失败都要记录在 `NamespaceClassBinding.status.conditions` 和 metrics 中。

## 13. 极端边界情况

### 13.1 Namespace 数量非常大

如果集群中有几万甚至更多 namespace，`NamespaceClass` 更新会产生 fan-out：所有引用该 class 的 namespace 都需要重新 reconcile。

风险：

1. 瞬间 enqueue 大量 namespace。
2. API server 被大量 get/apply/delete 请求打满。
3. controller 内存占用增加。
4. reconcile 延迟变高，最终一致时间变长。

应对：

1. 使用 informer cache 和 label index，避免每次 class 更新都全量扫描所有 namespace。
2. 使用 rate-limited workqueue，限制重试和突发流量。
3. 设置合理的 `MaxConcurrentReconciles`，例如从 5 到 20 开始，根据集群规模调优。
4. 对 class 更新触发的 fan-out 分批 enqueue，而不是一次性对 API server 施压。
5. controller 指标暴露 queue depth、reconcile latency、error count、apply count。
6. 接受 eventual consistency，不承诺所有 namespace 立即完成同步。
7. 具体并发、QPS 和同步延迟目标必须根据目标集群规模压测确定；在缺少目标环境数据时，设计只能给出限流和观测原则，不能承诺固定完成时间。

### 13.2 单个 NamespaceClass 包含大量资源

如果一个 class 包含几百甚至上千个资源模板：

风险：

1. 单次 reconcile 时间过长。
2. `NamespaceClassBinding.status.inventory` 可能接近 Kubernetes object size limit。
3. apply 中途失败会导致部分资源已更新、部分资源未更新。

应对：

1. 对单个 class 的资源数量设置合理上限。
2. inventory 只记录资源 identity，不保存完整对象。
3. reconcile 保持幂等，失败后允许重试继续推进。
4. 对超大 class 返回 validation error，要求管理员拆分。

### 13.3 Class 更新风暴

管理员或自动化系统频繁更新 `NamespaceClass` 时，会导致大量 namespace 反复入队。

应对：

1. workqueue 天然按 key 去重，同一个 namespace 不需要重复排队多次。
2. 使用 generation/observedGeneration，避免处理过期事件。
3. 对错误重试使用指数退避。
4. 指标中暴露 class fan-out 次数和队列积压。

### 13.4 Namespace 快速切换 Class

namespace label 可能从 A 切到 B，又快速切到 C。

应对：

1. reconcile 总是读取当前 namespace 状态，而不是依赖事件中的旧值。
2. 每次根据当前 label 重新计算 desired set。
3. 通过 inventory diff 清理不再需要的资源。
4. 操作必须幂等，重复 apply/delete 不应造成错误状态。

### 13.5 Namespace 正在删除

如果 namespace 正在删除，namespaced resources 会由 Kubernetes namespace deletion 流程清理。

应对：

1. controller 不强行逐个删除 namespaced resources，交给 Kubernetes namespace garbage collection。
2. 如果 inventory 中包含 cluster-scoped resources，则需要在 namespace 删除前清理它们。
3. controller 在成功解析到 `NamespaceClass` 且创建 managed resources 前，为 namespace 添加 `namespaceclass.akuity.io/finalizer`。
4. namespace 删除时，controller 只清理 inventory 中 `namespace` 为空的 cluster-scoped resources，清理成功后删除 `NamespaceClassBinding` 并移除 finalizer。
5. 如果某个部署通过策略禁用了 cluster-scoped resources，可以避免 namespace finalizer，降低卡死风险。

### 13.6 NamespaceClass 被删除

如果某个 class 被删除，但仍有 namespace 引用它：

默认行为：

1. 将引用该 class 的 namespace 的 desired set 视为空。
2. 根据 `NamespaceClassBinding.status.inventory` 删除此前由该 class 管理的资源。
3. 删除完成后清理对应 binding，或将 binding 标记为 cleanup completed。

风险：

1. 管理员误删 class 会触发大规模资源删除。
2. 如果 class 中包含 cluster-scoped resources，影响范围可能超过单个 namespace。

保守替代方案是保留已有资源，把 binding 标记为 `ClassNotFound`，等待管理员显式设置 cleanup policy 后再删除。本设计选择默认删除，因为它更符合“class 不存在时 desired set 为空”的声明式模型，但该行为必须在文档和运维手册中明确。

### 13.7 API Discovery 失败

controller 需要判断资源是 namespaced 还是 cluster-scoped。这个判断依赖 API discovery。

风险：

1. API server 短暂不可用。
2. CRD 刚安装，discovery cache 尚未刷新。
3. 某个 kind 已被删除。

应对：

1. discovery 失败时不要猜测 scope，直接 requeue。
2. 对 unknown kind 记录错误。
3. 定期刷新 RESTMapper/discovery cache。
4. 如果 CRD 后续恢复，reconcile 应该能自动成功。

### 13.8 目标资源已存在

如果 class 模板要创建 `ServiceAccount/app`，但 namespace 中已经存在同名 `ServiceAccount/app`，且不是 controller 管理：

风险：

1. 覆盖用户资源会破坏现有工作负载。
2. 接管已有资源会造成 ownership 不清晰。

应对：

1. 不覆盖。
2. 记录 conflict event。
3. 保持该 namespace reconcile 为失败或 degraded。
4. 管理员需要重命名模板资源，或手动删除/迁移已有资源。

### 13.9 Inventory 丢失或损坏

如果 `NamespaceClassBinding` 被用户删除，或 `status.inventory` 内容损坏：

风险：

1. controller 无法知道旧资源有哪些。
2. class 切换或删除 label 时可能无法清理旧资源。

应对：

1. 优先从 labels 反查 managed resources 进行 best-effort rebuild。
2. 只删除同时满足 managed marker 和 namespace UID annotation 的资源。
3. 如果无法安全重建，记录 `InventoryCorrupted`，停止危险删除。
4. 后续成功 apply 后重写 `NamespaceClassBinding.status.inventory`。

### 13.10 部分 Apply 成功

一次 reconcile 中可能前几个资源 apply 成功，后一个资源失败。

应对：

1. 不做事务性回滚，Kubernetes controller 通常依赖幂等重试。
2. 成功 apply 的资源保留。
3. 返回错误并 requeue。
4. 下次 reconcile 继续推进，直到 desired state 达成。

### 13.11 Server-Side Apply 冲突

如果用户手动修改了 controller 管理字段，server-side apply 可能出现 field conflict。

应对：

1. 默认不使用 force apply。
2. 对每个 desired resource 独立 apply；某个资源 conflict 不应阻塞其他资源处理。
3. 将冲突记录到 `NamespaceClassBinding.status.conditions`，reason 使用 `ApplyConflict`。
4. 对冲突资源不强制覆盖，不把从未成功 apply 的新增冲突资源写入 inventory。
5. 已经在旧 inventory 中的资源继续保留 inventory 记录，避免后续失去 ownership 线索。
6. stale resource 删除仍然执行，但只删除 inventory 中确认由 controller 管理、且不在 desired set 里的对象。
7. 可以提供可选配置 `forceConflicts: true`，但默认关闭；force 模式应标记为高风险，因为它会覆盖其他 field manager 的字段。

### 13.12 RBAC 权限不足

controller 可能没有权限创建某种资源。

应对：

1. apply/delete 返回 forbidden 时记录 event。
2. 不重试过快，避免无意义打 API server。
3. 暴露 metrics，方便管理员发现 RBAC 配置缺失。
4. 文档明确 controller 需要哪些权限，或要求为允许的 GVK 配置对应 RBAC。

### 13.13 ResourceQuota 或 Admission Webhook 拒绝

namespace 中的 quota、validating webhook、mutating webhook 可能拒绝资源创建。

应对：

1. 将错误记录到 event。
2. reconcile requeue，但使用退避。
3. 不绕过 admission。
4. 接受 namespace 处于 degraded 状态，直到管理员修复配置。

### 13.14 Cluster-Scoped Resource 命名冲突

如果多个 namespace 使用同一个 class，而 class 中定义了同名 cluster-scoped resource，会发生冲突。

应对：

1. 推荐模板支持变量，例如 `{{ .Namespace.Name }}`。
2. 对 cluster-scoped resources 要求名称全局唯一。
3. validating admission webhook 应尽可能提前发现同一个 class 内的重复 identity。

考虑题目要求支持任意资源，本设计保留 cluster-scoped resource 支持，但文档中明确命名责任和风险。

### 13.15 多副本 Controller 并发

controller 通常会运行多个副本以保证高可用。

应对：

1. 开启 leader election，确保同一时间只有一个 active reconciler。
2. 即使没有 leader election，apply/delete 逻辑也应尽量幂等。
3. inventory 更新需要处理 resourceVersion conflict，失败后重新读取并重试。

## 14. 设计取舍和考量

### 14.1 Raw Object API vs 强类型 API

选择 raw Kubernetes object list：

```yaml
spec:
  resources:
    - apiVersion: ...
      kind: ...
      metadata:
        name: ...
      spec: ...
```

原因：

1. 题目要求支持任意 Kubernetes 资源。
2. controller 不需要为每种资源设计专门字段。
3. 新 CRD 安装后也能被 `NamespaceClass` 使用。

代价：

1. CRD schema 校验能力弱。
2. 错误更多在 reconcile 阶段暴露。
3. 管理员需要理解原生 Kubernetes 对象结构。

这个取舍是合理的，因为题目的核心需求是灵活性，而不是强约束的专用 API。

### 14.2 ConfigMap Inventory vs NamespaceClassBinding CRD

第一版选择 cluster-scoped `NamespaceClassBinding` 保存 inventory。

优点：

1. inventory 不会因为 namespace 内 ConfigMap 被 namespace deletion 清理而丢失。
2. 可以可靠记录 cluster-scoped resources，支持 namespace 删除前清理。
3. 可以表达 per-namespace conditions、observed generation 和错误状态。
4. 管理员有一个稳定查询入口，知道某个 namespace 的 class 同步状态。

缺点：

1. 增加一个额外 CRD 和 controller 维护逻辑。
2. binding 生命周期需要和 namespace、class 删除语义配合。
3. inventory 损坏时仍然需要 best-effort rebuild。
4. namespace finalizer 会把 cleanup 失败转化为 namespace 删除阻塞，需要清晰的状态、日志和运维排障手段。

ConfigMap inventory 实现更简单，但不适合本设计对任意资源和 cluster-scoped resources 的支持目标。

### 14.3 Server-Side Apply vs Create/Update

选择 server-side apply。

优点：

1. 更符合声明式 controller 模型。
2. 可以保留其他 field manager 管理的字段。
3. 更容易处理 drift。

代价：

1. 需要处理 field conflict。
2. 实现比简单 create/update 稍复杂。
3. 管理员需要理解哪些字段由 controller 管理。

### 14.4 保守 Ownership vs 自动接管

选择保守策略：不接管已有但未标记为 managed 的资源。

原因：

1. 避免破坏用户手动创建的对象。
2. ownership 边界清晰。
3. 出错时更容易解释和恢复。

代价：

1. 管理员可能需要手动处理同名资源冲突。
2. 初次迁移已有资源时不够自动化。

这个取舍偏安全，适合作为 controller 的默认行为。

### 14.5 支持任意资源 vs 安全风险

题目要求支持任意资源，因此设计上支持 namespaced resources 和 cluster-scoped resources。

但这带来额外风险：

1. 命名是全局的，容易冲突。
2. 删除需要更谨慎，不能依赖 namespace deletion；必须依赖 binding inventory 和 namespace finalizer 清理 cluster-scoped resources。
3. RBAC 风险更高。
4. 高权限资源可能被 `NamespaceClass` 间接创建。

控制方式：

1. `NamespaceClass` 写权限只授予集群管理员。
2. controller 支持 GVK allowlist/denylist；产品能力支持任意资源，但部署策略可以限制实际允许的 GVK。
3. 高风险 GVK 需要显式 allow。
4. cluster-scoped resources 推荐使用模板变量生成唯一名称。
5. 删除行为必须基于 binding inventory 和 ownership marker。

### 14.6 Watch Managed Resources vs 只靠主资源事件

只监听 `Namespace`、`NamespaceClass` 和 `NamespaceClassBinding` 可以满足核心需求，但无法及时发现用户删除或修改了已管理资源。

推荐监听 managed resources：

1. 如果已管理资源被删除，controller 可以重新创建。
2. 如果已管理资源发生 drift，controller 可以重新 apply。

controller-runtime 对编译期已知类型支持很好，例如在代码中直接 watch `Namespace`、`Deployment` 或 `NetworkPolicy`。但 `NamespaceClass.spec.resources` 允许管理员在运行时引用任意 GVK，controller 编译时不知道这些类型。如果要实时 watch 它们，需要使用 dynamic informer 针对 `unstructured.Unstructured` 动态创建 informer，并处理 discovery、RESTMapper 缓存刷新、informer 生命周期、RBAC 失败和内存占用。

第一版不做 dynamic informer，只 watch `Namespace`、`NamespaceClass` 和 `NamespaceClassBinding`，并通过周期性 resync 修复 drift。这牺牲了 drift 修复速度，但明显降低实现复杂度。dynamic informer 作为后续增强。

## 15. 测试计划

核心测试：

1. 创建带 class label 的 namespace 后，class 中的资源被创建。
2. namespace 从 class A 切到 class B 后，A 独有资源被删除，B 资源被创建。
3. `NamespaceClass` 增加资源后，已有 namespace 自动获得新资源。
4. `NamespaceClass` 删除资源后，已有 namespace 中对应 managed resource 被删除。
5. 移除 namespace 的 class label 后，已管理资源被清理。
6. 目标资源已存在但不是 controller 管理时，controller 不覆盖并记录 conflict。
7. `NamespaceClassBinding.status.inventory` 损坏时，controller 不执行危险删除。
8. unknown GVK 或 discovery 失败时，controller 记录错误并 requeue。
9. server-side apply conflict 时，controller 不强制覆盖。
10. namespace 删除时，cluster-scoped resources 能被清理。
11. validating admission webhook 拒绝缺少 `apiVersion`、`kind`、`metadata.name` 或使用保留 label 的 class。
12. 模板变量渲染失败时，controller 不部分执行 class。

规模测试：

1. 1 万个 namespace 同时引用同一个 class，更新 class 后 controller 不应压垮 API server。
2. 单个 class 包含大量资源时，controller 能稳定重试并暴露错误。
3. 高频 class 更新时，workqueue 能去重并保持 eventual consistency。

## 16. 成功标准

该设计成功的标准：

1. Namespace 创建后，controller 能根据 class 创建 desired resources。
2. Namespace 切换 class 后，旧资源被清理，新资源被创建。
3. NamespaceClass 更新后，所有引用它的 namespace 最终同步到新 desired state。
4. controller 不覆盖不属于自己的资源。
5. controller 在错误和大规模场景下保持幂等、可重试、可观测。
6. 管理员可以解释每个资源为什么存在、由哪个 class 创建、属于哪个 namespace。

## 17. 总结

这个方案把 `NamespaceClass` 设计成“namespace 级资源模板集合”，把实际正确性交给 controller 的 reconciliation、inventory diff、server-side apply 和 ownership 边界。

第一版保持 API 简洁：

```text
NamespaceClass.spec.resources = raw Kubernetes objects
Namespace label = class binding
Inventory = cluster-scoped NamespaceClassBinding.status.inventory
Apply = server-side apply
Delete = inventory diff
```

这套设计能覆盖题目要求，同时保留清晰的演进路径：如果需要更严格安全控制，可以强化 GVK allowlist/denylist 和 admission webhook；如果需要更快 drift 修复，可以加入 dynamic informer watch managed resources。

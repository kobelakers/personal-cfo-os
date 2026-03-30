# Deployment Profiles

Phase 7B 把已有运行方式整理成正式产品化档位。  
这些 profile 是对同一内核的不同包装，不是重新定义 runtime。

## Profile Matrix

| Profile | 组成 | 适用场景 | 不包含 |
| --- | --- | --- | --- |
| `local-lite` | SQLite + LocalFS + CLI/API | 单机开发、快速验证、本地测试 | promoted Postgres runtime、MinIO、multi-worker proof |
| `runtime-promotion` | Postgres + MinIO + API + 2 workers + served UI | 7A/7B 强运行时档位、operator 演示、runtime backbone 证明 | remote agents、broker、Temporal cluster |
| `interview-demo` | local-lite backend + served UI + deterministic seed + checked-in samples | 启动快、面试展示、可读 replay/benchmark/operator 面 | full runtime-promotion infra |
| `dev-stack` | local-lite API + one worker + Vite | 日常开发、mock/provider-light 调试、前后端联调 | full infra promotion、public protocol server |

## 启动入口

### local-lite

- `go run ./cmd/api --runtime-profile local-lite --runtime-backend sqlite`
- `go run ./cmd/worker --runtime-profile local-lite --runtime-backend sqlite`

### runtime-promotion

- `./scripts/run_runtime_promotion_7b.sh up`
- compose file：`deployments/docker-compose.runtime-promotion.yml`
- env example：`.env.runtime-promotion.example`

### interview-demo

- `./scripts/run_interview_demo_7b.sh all`
- env example：`deployments/.env.interview-demo.example`
- 会先 build UI，再用 deterministic seed 生成本地可读样例数据

### dev-stack

- `./scripts/run_dev_stack_7b.sh all`
- env example：`deployments/.env.dev-stack.example`
- API、worker、Vite 会一起启动

## 设计原则

- profile 是 operator/product surface 的包装，不是第二套业务逻辑
- `runtime-promotion` 继续承接 7A 的 promoted backend 证明
- `interview-demo` 用 deterministic seed 和 checked-in samples 提升外部可读性，而不是伪造新的内核语义
- `dev-stack` 以开发效率为主，不与 `runtime-promotion` 的 correctness/proof 目标混淆

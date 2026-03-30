# Interview Demo Runbook

7B closeout 之后，仓库已经有一套面向 operator surface 的稳定演示路径。  
目标不是展示“花哨前端”，而是快速证明：

- operator UI 可用
- `/api/v1` 是 canonical surface
- replay / approval / benchmark / artifact 都是同一套内核真相面的产品化暴露

## 推荐演示顺序

### 1. interview-demo

最适合面试或短时间演示。

启动：

```bash
./scripts/run_interview_demo_7b.sh all
```

会自动：

- build UI
- 运行 `cmd/demo-seed`
- 生成 deterministic demo manifest
- 启动 API-served UI

重点查看：

- `Task Graph Viewer`
- `Approval Panel`
- `Replay Viewer`
- `Benchmark Panel`

重点文件：

- `var/interview-demo/demo-manifest.json`
- `docs/product/operator-sample-index.md`

### 2. dev-stack

适合开发联调和 panel 级验证。

启动：

```bash
./scripts/run_dev_stack_7b.sh all
```

特点：

- local-lite backend
- 一个 worker
- Vite 开发态前端

### 3. runtime-promotion

适合展示 7A+7B 合起来的强运行时 + operator surface。

启动：

```bash
./scripts/run_runtime_promotion_7b.sh smoke
```

特点：

- Postgres + MinIO + 2 workers
- API-served UI
- `/api/v1/meta/profile` 可直接证明 profile/backends

## 面试演示建议

如果只有几分钟，建议固定顺序：

1. 打开首页说明 `runtime_profile / runtime_backend / blob_backend`
2. 看 `Task Graph Viewer`，证明 child workflow / deferred / dependency
3. 看 `Approval Panel`，证明 operator action 仍走 typed operator service
4. 看 `Replay Viewer`，证明 why/how / degradation / async runtime summary
5. 看 `Benchmark Panel`，证明 benchmark/reporting 已经 productized

## 与 Sample 的关系

7B 不新增第二套 truth source。  
演示过程中看到的内容来自：

- runtime/operator/replay/query services
- checked-in deterministic samples
- artifact plane 中的正式产物

样例索引见：

- `docs/product/operator-sample-index.md`

import { useEffect, useState } from "react";
import type { ReactNode } from "react";
import { api } from "./lib/api";
import type {
  ApprovalDetail,
  ApprovalRecord,
  ArtifactContentView,
  BenchmarkCompareView,
  BenchmarkRunDetail,
  BenchmarkRunSummary,
  ProfileMeta,
  ReplayComparison,
  ReplayQueryScope,
  ReplayView,
  TaskCommandResult,
  TaskGraphView,
} from "./types";

type TabKey = "graphs" | "approvals" | "replay" | "intelligence" | "artifacts" | "benchmarks";

const tabs: Array<{ key: TabKey; label: string; subtitle: string }> = [
  { key: "graphs", label: "Task Graphs", subtitle: "任务图、依赖、child workflow" },
  { key: "approvals", label: "Approvals", subtitle: "审批、冲突、operator actions" },
  { key: "replay", label: "Replay", subtitle: "why/how、async runtime、degradation" },
  { key: "intelligence", label: "Memory · Skill · Runtime", subtitle: "记忆、技能、异步运行时摘要" },
  { key: "artifacts", label: "Artifacts", subtitle: "token/cost、report、bundle、checkpoint refs" },
  { key: "benchmarks", label: "Benchmarks", subtitle: "corpus、compare、export" },
];

export default function App() {
  const [tab, setTab] = useState<TabKey>(() => readTabFromURL());
  const [profile, setProfile] = useState<ProfileMeta | null>(null);
  const [taskGraphs, setTaskGraphs] = useState<TaskGraphView[]>([]);
  const [selectedGraphId, setSelectedGraphId] = useState<string>(() => readParam("graph"));
  const [taskGraphDetail, setTaskGraphDetail] = useState<TaskGraphView | null>(null);
  const [approvals, setApprovals] = useState<ApprovalRecord[]>([]);
  const [selectedApprovalId, setSelectedApprovalId] = useState<string>(() => readParam("approval"));
  const [approvalDetail, setApprovalDetail] = useState<ApprovalDetail | null>(null);
  const [actionResult, setActionResult] = useState<TaskCommandResult | null>(null);
  const [actionError, setActionError] = useState<string>("");
  const [replayScope, setReplayScope] = useState<ReplayQueryScope>(() => (readParam("replayScope") as ReplayQueryScope) || "workflow");
  const [replayID, setReplayID] = useState<string>(() => readParam("replayId"));
  const [replayView, setReplayView] = useState<ReplayView | null>(null);
  const [replayError, setReplayError] = useState<string>("");
  const [compareLeft, setCompareLeft] = useState<string>("");
  const [compareRight, setCompareRight] = useState<string>("");
  const [replayComparison, setReplayComparison] = useState<ReplayComparison | null>(null);
  const [artifactID, setArtifactID] = useState<string>(() => readParam("artifact"));
  const [artifactView, setArtifactView] = useState<ArtifactContentView | null>(null);
  const [artifactError, setArtifactError] = useState<string>("");
  const [benchmarks, setBenchmarks] = useState<BenchmarkRunSummary[]>([]);
  const [selectedBenchmarkId, setSelectedBenchmarkId] = useState<string>(() => readParam("benchmark"));
  const [benchmarkDetail, setBenchmarkDetail] = useState<BenchmarkRunDetail | null>(null);
  const [benchmarkCompare, setBenchmarkCompare] = useState<BenchmarkCompareView | null>(null);
  const [benchmarkMarkdown, setBenchmarkMarkdown] = useState<string>("");
  const [loading, setLoading] = useState<string>("");
  const [globalError, setGlobalError] = useState<string>("");

  useEffect(() => {
    void loadBootstrap();
  }, []);

  useEffect(() => {
    syncURL({
      tab,
      graph: selectedGraphId,
      approval: selectedApprovalId,
      replayScope,
      replayId: replayID,
      artifact: artifactID,
      benchmark: selectedBenchmarkId,
    });
  }, [tab, selectedGraphId, selectedApprovalId, replayScope, replayID, artifactID, selectedBenchmarkId]);

  useEffect(() => {
    if (!selectedGraphId) {
      setTaskGraphDetail(null);
      return;
    }
    setLoading("加载 task graph 详情");
    void api
      .getTaskGraph(selectedGraphId)
      .then((detail) => {
        setTaskGraphDetail(detail);
        if (!replayID) {
          setReplayScope("task_graph");
          setReplayID(detail.snapshot.graph.graph_id);
        }
      })
      .catch((error: Error) => setGlobalError(error.message))
      .finally(() => setLoading(""));
  }, [selectedGraphId]);

  useEffect(() => {
    if (!selectedApprovalId) {
      setApprovalDetail(null);
      return;
    }
    setLoading("加载审批详情");
    void api
      .getApproval(selectedApprovalId)
      .then((detail) => {
        setApprovalDetail(detail);
      })
      .catch((error: Error) => setActionError(error.message))
      .finally(() => setLoading(""));
  }, [selectedApprovalId]);

  useEffect(() => {
    if (!replayID) {
      setReplayView(null);
      return;
    }
    setLoading("查询 replay");
    setReplayError("");
    void api
      .getReplay(replayScope, replayID)
      .then((view) => {
        setReplayView(view);
        const nextArtifact = firstArtifactID(view);
        if (nextArtifact && !artifactID) {
          setArtifactID(nextArtifact);
        }
      })
      .catch((error: Error) => setReplayError(error.message))
      .finally(() => setLoading(""));
  }, [replayScope, replayID]);

  useEffect(() => {
    if (!artifactID) {
      setArtifactView(null);
      return;
    }
    setLoading("加载 artifact 内容");
    setArtifactError("");
    void api
      .getArtifactContent(artifactID)
      .then(setArtifactView)
      .catch((error: Error) => setArtifactError(error.message))
      .finally(() => setLoading(""));
  }, [artifactID]);

  useEffect(() => {
    if (!selectedBenchmarkId) {
      setBenchmarkDetail(null);
      setBenchmarkMarkdown("");
      return;
    }
    setLoading("加载 benchmark 详情");
    void Promise.all([api.getBenchmark(selectedBenchmarkId), api.exportBenchmarkMarkdown(selectedBenchmarkId)])
      .then(([detail, markdown]) => {
        setBenchmarkDetail(detail);
        setBenchmarkMarkdown(markdown);
      })
      .catch((error: Error) => setGlobalError(error.message))
      .finally(() => setLoading(""));
  }, [selectedBenchmarkId]);

  async function loadBootstrap() {
    try {
      setLoading("加载 operator surface");
      const [meta, graphList, approvalList, benchmarkList] = await Promise.all([
        api.profile(),
        api.listTaskGraphs(),
        api.listPendingApprovals(),
        api.listBenchmarks(),
      ]);
      setProfile(meta);
      setTaskGraphs(graphList);
      setApprovals(approvalList);
      setBenchmarks(benchmarkList);
      if (!selectedGraphId && graphList[0]) {
        setSelectedGraphId(graphList[0].snapshot.graph.graph_id);
      }
      if (!selectedApprovalId && approvalList[0]) {
        setSelectedApprovalId(approvalList[0].approval_id);
      }
      if (!selectedBenchmarkId && benchmarkList[0]) {
        setSelectedBenchmarkId(benchmarkList[0].id);
      }
    } catch (error) {
      setGlobalError((error as Error).message);
    } finally {
      setLoading("");
    }
  }

  async function handleApprovalAction(action: "approve" | "deny") {
    if (!approvalDetail) return;
    setActionError("");
    setLoading(action === "approve" ? "提交审批通过" : "提交审批拒绝");
    const body = {
      request_id: `${action}-${approvalDetail.approval.approval_id}-${Date.now()}`,
      actor: "operator-ui",
      roles: ["operator"],
      note: action === "approve" ? "通过 operator ui 执行审批" : "通过 operator ui 拒绝审批",
      expected_version: approvalDetail.approval.version,
    };
    try {
      const result =
        action === "approve"
          ? await api.approveApproval(approvalDetail.approval.approval_id, body)
          : await api.denyApproval(approvalDetail.approval.approval_id, body);
      setActionResult(result);
      const [approvalList, detail] = await Promise.all([
        api.listPendingApprovals(),
        api.getApproval(approvalDetail.approval.approval_id).catch(() => approvalDetail),
      ]);
      setApprovals(approvalList);
      setApprovalDetail(detail);
    } catch (error) {
      setActionError((error as Error).message);
    } finally {
      setLoading("");
    }
  }

  async function handleReplayCompare() {
    if (!compareLeft || !compareRight) {
      setReplayError("compare 需要 left 和 right 两个 scope:id");
      return;
    }
    setLoading("比较 replay");
    setReplayError("");
    try {
      setReplayComparison(await api.compareReplay(compareLeft, compareRight));
    } catch (error) {
      setReplayError((error as Error).message);
    } finally {
      setLoading("");
    }
  }

  async function handleReplayQuery() {
    if (!replayID) {
      setReplayError("请输入 replay id");
      return;
    }
    setLoading("查询 replay");
    setReplayError("");
    try {
      const view = await api.getReplay(replayScope, replayID);
      setReplayView(view);
      const nextArtifact = firstArtifactID(view);
      if (nextArtifact && !artifactID) {
        setArtifactID(nextArtifact);
      }
    } catch (error) {
      setReplayError((error as Error).message);
    } finally {
      setLoading("");
    }
  }

  async function handleBenchmarkCompare() {
    if (benchmarks.length < 2) return;
    const left = selectedBenchmarkId || benchmarks[0]?.id;
    const right = benchmarks.find((item) => item.id !== left)?.id || left;
    if (!left || !right) return;
    setLoading("比较 benchmark");
    try {
      setBenchmarkCompare(await api.compareBenchmarks(left, right));
    } catch (error) {
      setGlobalError((error as Error).message);
    } finally {
      setLoading("");
    }
  }

  const replayArtifacts = replayView?.workflow?.artifacts ?? replayView?.task_graph?.artifacts ?? [];
  const taskGraphArtifacts = taskGraphDetail?.artifacts ?? [];
  const artifactCandidates = replayArtifacts.length > 0 ? replayArtifacts : taskGraphArtifacts;

  return (
    <main className="app-shell">
      <section className="hero">
        <div className="hero-copy">
          <p className="eyebrow">Personal CFO OS · 7B</p>
          <h1>Operator Console for a governed, verifiable, observable agent system.</h1>
          <p className="copy">
            这一层只可视化现有 typed runtime、operator、replay、artifact、benchmark surfaces，不把业务判断搬进前端。
          </p>
        </div>
        <div className="hero-meta">
          <MetaChip label="Profile" value={profile?.runtime_profile ?? "loading"} />
          <MetaChip label="Backend" value={profile?.runtime_backend ?? "loading"} />
          <MetaChip label="Blob" value={profile?.blob_backend ?? "loading"} />
          <MetaChip label="UI" value={profile?.ui_mode ?? "loading"} />
        </div>
      </section>

      <nav className="tab-strip" aria-label="Operator tabs">
        {tabs.map((item) => (
          <button
            key={item.key}
            className={item.key === tab ? "tab-button is-active" : "tab-button"}
            onClick={() => setTab(item.key)}
            type="button"
          >
            <span>{item.label}</span>
            <small>{item.subtitle}</small>
          </button>
        ))}
      </nav>

      {globalError ? <p className="banner is-error">{globalError}</p> : null}
      {loading ? <p className="banner">{loading}</p> : null}

      <section className="content-grid">
        {tab === "graphs" ? (
          <>
            <Panel title="Task Graph Viewer" subtitle="task graph、dependency、deferred、child workflow">
              <ul className="list">
                {taskGraphs.map((item) => {
                  const graphId = item.snapshot.graph.graph_id;
                  return (
                    <li key={graphId}>
                      <button className="list-button" type="button" onClick={() => setSelectedGraphId(graphId)}>
                        <strong>{graphId}</strong>
                        <span>{item.snapshot.graph.generation_summary || item.snapshot.graph.parent_workflow_id}</span>
                      </button>
                    </li>
                  );
                })}
              </ul>
            </Panel>
            <Panel title="Task Graph Detail" subtitle={taskGraphDetail?.snapshot.graph.graph_id || "选择一个 task graph"}>
              {taskGraphDetail ? (
                <>
                  <KeyValueGrid
                    items={[
                      ["Parent Workflow", taskGraphDetail.snapshot.graph.parent_workflow_id],
                      ["Parent Task", taskGraphDetail.snapshot.graph.parent_task_id],
                      ["Pending Approval", taskGraphDetail.pending_approval?.approval_id || "none"],
                      ["Executed Tasks", String(taskGraphDetail.snapshot.executed_tasks?.length || 0)],
                    ]}
                  />
                  <SectionList title="Registered Tasks" items={taskGraphDetail.snapshot.registered_tasks?.map((item) => `${item.task.id} · ${item.status} · ${item.task.user_intent_type}`) || []} />
                  <SectionList title="Dependencies" items={taskGraphDetail.snapshot.graph.dependencies?.map((item) => `${item.upstream_task_id} -> ${item.downstream_task_id} · ${item.reason}`) || []} />
                  <SectionList title="Deferred" items={taskGraphDetail.snapshot.deferred?.map((item) => `${item.task_id} · ${item.reason}`) || []} />
                </>
              ) : (
                <EmptyState>选择左侧 task graph 查看详情。</EmptyState>
              )}
            </Panel>
          </>
        ) : null}

        {tab === "approvals" ? (
          <>
            <Panel title="Pending Approvals" subtitle="pending list">
              <ul className="list">
                {approvals.map((item) => (
                  <li key={item.approval_id}>
                    <button className="list-button" type="button" onClick={() => setSelectedApprovalId(item.approval_id)}>
                      <strong>{item.approval_id}</strong>
                      <span>{item.requested_action} · v{item.version}</span>
                    </button>
                  </li>
                ))}
              </ul>
            </Panel>
            <Panel title="Approval Detail" subtitle={approvalDetail?.approval.approval_id || "选择一个 approval"}>
              {approvalDetail ? (
                <>
                  <KeyValueGrid
                    items={[
                      ["Workflow", approvalDetail.approval.workflow_id],
                      ["Graph", approvalDetail.approval.graph_id],
                      ["Task", approvalDetail.approval.task_id],
                      ["Status", approvalDetail.approval.status],
                      ["Requested Action", approvalDetail.approval.requested_action],
                      ["Version", String(approvalDetail.approval.version)],
                    ]}
                  />
                  <div className="button-row">
                    <button type="button" onClick={() => void handleApprovalAction("approve")}>
                      Approve
                    </button>
                    <button type="button" className="danger" onClick={() => void handleApprovalAction("deny")}>
                      Deny
                    </button>
                  </div>
                  {actionError ? <p className="inline-error">{actionError}</p> : null}
                  {actionResult ? (
                    <div className="result-box">
                      <strong>Action Result</strong>
                      <p>{actionResult.action.action_type} · async_dispatch={String(actionResult.async_dispatch_accepted)}</p>
                    </div>
                  ) : null}
                </>
              ) : (
                <EmptyState>当前没有选中的审批项。</EmptyState>
              )}
            </Panel>
          </>
        ) : null}

        {tab === "replay" ? (
          <>
            <Panel title="Replay Query" subtitle="workflow / task-graph / task / execution / approval">
              <div className="form-grid">
                <label>
                  <span>Scope</span>
                  <select value={replayScope} onChange={(event) => setReplayScope(event.target.value as ReplayQueryScope)}>
                    <option value="workflow">workflow</option>
                    <option value="task_graph">task_graph</option>
                    <option value="task">task</option>
                    <option value="execution">execution</option>
                    <option value="approval">approval</option>
                  </select>
                </label>
                <label>
                  <span>ID</span>
                  <input value={replayID} onChange={(event) => setReplayID(event.target.value)} placeholder="输入 replay scope id" />
                </label>
              </div>
              <div className="form-grid">
                <label>
                  <span>Compare Left</span>
                  <input value={compareLeft} onChange={(event) => setCompareLeft(event.target.value)} placeholder="workflow:workflow-id" />
                </label>
                <label>
                  <span>Compare Right</span>
                  <input value={compareRight} onChange={(event) => setCompareRight(event.target.value)} placeholder="approval:approval-id" />
                </label>
              </div>
              <div className="button-row">
                <button type="button" onClick={() => void handleReplayQuery()}>
                  Refresh Replay
                </button>
                <button type="button" onClick={() => void handleReplayCompare()}>
                  Compare
                </button>
              </div>
              {replayError ? <p className="inline-error">{replayError}</p> : null}
            </Panel>
            <Panel title="Replay Result" subtitle={replayView ? `${replayView.scope.kind}:${replayView.scope.id}` : "尚未查询"}>
              {replayView ? (
                <>
                  <KeyValueGrid
                    items={[
                      ["Final State", replayView.summary.final_state || replayView.workflow?.runtime_state || "unknown"],
                      ["Projection", replayView.projection_status || "n/a"],
                      ["Projection Version", replayView.projection_version ? String(replayView.projection_version) : "n/a"],
                      ["Degraded", String(replayView.degraded)],
                    ]}
                  />
                  <SectionList title="Plan Summary" items={replayView.summary.plan_summary || []} />
                  <SectionList title="Async Runtime Summary" items={replayView.summary.async_runtime_summary || []} />
                  <SectionList title="Why / How" items={flattenReplayExplanation(replayView)} />
                  <SectionList title="Provenance Summary" items={summarizeProvenance(replayView)} />
                  <SectionList title="Degradation Reasons" items={replayView.degradation_reasons?.map((item) => `${item.reason} · ${item.message}`) || []} />
                  {replayComparison ? <SectionList title="Compare Summary" items={replayComparison.summary || replayComparison.diffs?.map((item) => `${item.category} · ${item.summary}`) || []} /> : null}
                </>
              ) : (
                <EmptyState>输入 scope 与 id 后会显示 replay、why/how 与 async runtime summary。</EmptyState>
              )}
            </Panel>
          </>
        ) : null}

        {tab === "intelligence" ? (
          <>
            <Panel title="Memory Summary" subtitle="selected / rejected / written">
              <SectionList title="Memory" items={replayView?.summary.memory_summary || []} />
              <SectionList title="Why Memory" items={replayView?.explanation.why_memory_decision || []} />
            </Panel>
            <Panel title="Skill + Runtime Summary" subtitle="skill selection、claim/reclaim/retry/resume">
              <SectionList title="Skill" items={replayView?.summary.skill_summary || []} />
              <SectionList title="Why Skill" items={replayView?.explanation.why_skill_selected || []} />
              <SectionList title="Runtime" items={replayView?.summary.async_runtime_summary || []} />
              <SectionList title="Why Runtime" items={replayView?.explanation.why_async_runtime || []} />
            </Panel>
          </>
        ) : null}

        {tab === "artifacts" ? (
          <>
            <Panel title="Artifacts" subtitle="report / replay bundle / checkpoint refs">
              <ul className="list">
                {artifactCandidates.map((item) => (
                  <li key={item.id}>
                    <button className="list-button" type="button" onClick={() => setArtifactID(item.id)}>
                      <strong>{item.kind}</strong>
                      <span>{artifactCandidateLabel(item)}</span>
                    </button>
                  </li>
                ))}
              </ul>
              {artifactError ? <p className="inline-error">{artifactError}</p> : null}
            </Panel>
            <Panel title="Token / Cost / Report / Artifact Detail" subtitle={artifactView?.artifact.id || "选择一个 artifact"}>
              {artifactView ? (
                <>
                  <KeyValueGrid
                    items={[
                      ["Kind", artifactView.artifact.kind],
                      ["Workflow", artifactView.artifact.workflow_id],
                      ["Task", artifactView.artifact.task_id],
                      ["Location", artifactView.artifact.ref?.location || "n/a"],
                      ["Total Tokens", String(artifactView.usage_summary?.total_tokens || 0)],
                      ["Estimated Cost", artifactView.usage_summary?.estimated_cost_usd?.toFixed(4) || "0.0000"],
                    ]}
                  />
                  <SectionList title="Prompt / Provider / Cost Evidence" items={artifactPromptSummary(artifactView)} />
                  <SectionList title="Artifact Summary" items={artifactView.summary_lines || []} />
                  <pre className="code-block">{JSON.stringify(artifactView.structured ?? artifactView.raw_text ?? {}, null, 2)}</pre>
                </>
              ) : (
                <EmptyState>从 replay 或 task graph 里选择一个 artifact 查看 payload 和 token/cost 摘要。</EmptyState>
              )}
            </Panel>
          </>
        ) : null}

        {tab === "benchmarks" ? (
          <>
            <Panel title="Benchmark Catalog" subtitle="deterministic corpus / export-ready">
              <ul className="list">
                {benchmarks.map((item) => (
                  <li key={item.id}>
                    <button className="list-button" type="button" onClick={() => setSelectedBenchmarkId(item.id)}>
                      <strong>{item.id}</strong>
                      <span>{item.corpus_id} · passed {item.passed_count}/{item.scenario_count}</span>
                    </button>
                  </li>
                ))}
              </ul>
              <div className="button-row">
                <button type="button" onClick={() => void handleBenchmarkCompare()}>
                  Compare First Two
                </button>
              </div>
            </Panel>
            <Panel title="Benchmark Detail" subtitle={benchmarkDetail?.summary.id || "选择一个 benchmark"}>
              {benchmarkDetail ? (
                <>
                  <KeyValueGrid
                    items={[
                      ["Corpus", benchmarkDetail.summary.corpus_id],
                      ["Deterministic", String(benchmarkDetail.summary.deterministic_only)],
                      ["Passed", `${benchmarkDetail.summary.passed_count}/${benchmarkDetail.summary.scenario_count}`],
                      ["Latency", `${benchmarkDetail.summary.average_latency_ms.toFixed(2)} ms`],
                      ["Tokens", String(benchmarkDetail.summary.total_token_usage)],
                    ]}
                  />
                  <SectionList title="Summary Lines" items={benchmarkDetail.run.summary.summary_lines || []} />
                  <SectionList title="Scenario Status" items={benchmarkDetail.run.results.map((item) => `${item.scenario_id} · ${item.passed ? "passed" : "failed"} · ${item.runtime_state}`)} />
                  {benchmarkCompare ? <SectionList title="Compare Summary" items={benchmarkCompare.summary || benchmarkCompare.diff.summary || []} /> : null}
                  {benchmarkMarkdown ? <pre className="code-block">{benchmarkMarkdown}</pre> : null}
                </>
              ) : (
                <EmptyState>benchmarks panel 只做 deterministic run 的读、比较和导出，不在 UI 中触发 live eval。</EmptyState>
              )}
            </Panel>
          </>
        ) : null}
      </section>
    </main>
  );
}

function MetaChip(props: { label: string; value: string }) {
  return (
    <div className="meta-chip">
      <span>{props.label}</span>
      <strong>{props.value}</strong>
    </div>
  );
}

function Panel(props: { title: string; subtitle?: string; children: ReactNode }) {
  return (
    <article className="panel panel-large">
      <header className="panel-header">
        <div>
          <h2>{props.title}</h2>
          {props.subtitle ? <p>{props.subtitle}</p> : null}
        </div>
      </header>
      {props.children}
    </article>
  );
}

function KeyValueGrid(props: { items: Array<[string, string]> }) {
  return (
    <dl className="kv-grid">
      {props.items.map(([key, value]) => (
        <div key={key} className="kv-item">
          <dt>{key}</dt>
          <dd>{value}</dd>
        </div>
      ))}
    </dl>
  );
}

function SectionList(props: { title: string; items: string[] }) {
  if (!props.items || props.items.length === 0) {
    return null;
  }
  return (
    <section className="section-list">
      <h3>{props.title}</h3>
      <ul>
        {props.items.map((item) => (
          <li key={item}>{item}</li>
        ))}
      </ul>
    </section>
  );
}

function EmptyState(props: { children: ReactNode }) {
  return <p className="empty-state">{props.children}</p>;
}

function flattenReplayExplanation(view: ReplayView): string[] {
  return [
    ...(view.explanation.why_skill_selected || []),
    ...(view.explanation.why_generated_task || []),
    ...(view.explanation.why_child_executed || []),
    ...(view.explanation.why_memory_decision || []),
    ...(view.explanation.why_validation_failed || []),
    ...(view.explanation.why_async_runtime || []),
    ...(view.explanation.why_failed ? [view.explanation.why_failed] : []),
    ...(view.explanation.why_waiting_approval ? [view.explanation.why_waiting_approval] : []),
  ];
}

function summarizeProvenance(view: ReplayView): string[] {
  const edges = view.provenance.edges || [];
  const nodes = view.provenance.nodes || [];
  return [
    `nodes=${nodes.length}`,
    `edges=${edges.length}`,
    ...edges.slice(0, 8).map((edge) => `${edge.type} · ${edge.from_node_id} -> ${edge.to_node_id}`),
  ];
}

function artifactPromptSummary(view: ArtifactContentView): string[] {
  const items: string[] = [];
  if (view.usage_summary?.providers?.length) {
    items.push(`providers=${view.usage_summary.providers.join(",")}`);
  }
  if (view.usage_summary?.models?.length) {
    items.push(`models=${view.usage_summary.models.join(",")}`);
  }
  if (view.usage_summary?.prompt_ids?.length) {
    items.push(`prompt_ids=${view.usage_summary.prompt_ids.join(",")}`);
  }
  if (view.usage_summary?.call_count) {
    items.push(`call_count=${view.usage_summary.call_count}`);
  }
  return items;
}

function firstArtifactID(view: ReplayView): string {
  const items = view.workflow?.artifacts || view.task_graph?.artifacts || [];
  return items[0]?.id || "";
}

function artifactCandidateLabel(item: { id: string; ref?: { location?: string; summary?: string }; summary?: string; location?: string }) {
  return item.summary || item.location || item.ref?.summary || item.ref?.location || item.id;
}

function readParam(key: string): string {
  return new URLSearchParams(window.location.search).get(key) || "";
}

function readTabFromURL(): TabKey {
  const value = new URLSearchParams(window.location.search).get("tab");
  return (tabs.find((item) => item.key === value)?.key as TabKey | undefined) || "graphs";
}

function syncURL(values: Record<string, string>) {
  const params = new URLSearchParams(window.location.search);
  Object.entries(values).forEach(([key, value]) => {
    if (!value) {
      params.delete(key);
      return;
    }
    params.set(key, value);
  });
  window.history.replaceState({}, "", `${window.location.pathname}?${params.toString()}`);
}

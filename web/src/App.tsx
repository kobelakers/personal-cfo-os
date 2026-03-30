import { useEffect, useMemo, useState } from "react";
import { api } from "./lib/api";
import { useArtifact } from "./hooks/useArtifact";
import { useBenchmarks } from "./hooks/useBenchmarks";
import { useBootstrap } from "./hooks/useBootstrap";
import { useReplay } from "./hooks/useReplay";
import { ApprovalPanel } from "./panels/ApprovalPanel";
import { ArtifactPanel } from "./panels/ArtifactPanel";
import { BenchmarkPanel } from "./panels/BenchmarkPanel";
import { MetaChip } from "./panels/common";
import { IntelligencePanel } from "./panels/IntelligencePanel";
import { ReplayPanel } from "./panels/ReplayPanel";
import { TaskGraphPanel } from "./panels/TaskGraphPanel";
import type { ApprovalDetail, ApprovalRecord, ReplayQueryScope, TaskCommandResult, TaskGraphView } from "./types";

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
  const [selectedGraphId, setSelectedGraphId] = useState<string>(() => readParam("graph"));
  const [taskGraphDetail, setTaskGraphDetail] = useState<TaskGraphView | null>(null);
  const [selectedApprovalId, setSelectedApprovalId] = useState<string>(() => readParam("approval"));
  const [approvalDetail, setApprovalDetail] = useState<ApprovalDetail | null>(null);
  const [actionResult, setActionResult] = useState<TaskCommandResult | null>(null);
  const [actionError, setActionError] = useState("");
  const [replayScope, setReplayScope] = useState<ReplayQueryScope>(() => (readParam("replayScope") as ReplayQueryScope) || "workflow");
  const [replayID, setReplayID] = useState<string>(() => readParam("replayId"));
  const [compareLeft, setCompareLeft] = useState("");
  const [compareRight, setCompareRight] = useState("");
  const [artifactID, setArtifactID] = useState<string>(() => readParam("artifact"));
  const [selectedBenchmarkId, setSelectedBenchmarkId] = useState<string>(() => readParam("benchmark"));
  const [globalError, setGlobalError] = useState("");

  const {
    profile,
    taskGraphs,
    approvals,
    benchmarks,
    loading: bootstrapLoading,
    error: bootstrapError,
    setApprovals,
  } = useBootstrap({
    onLoaded: ({ taskGraphs: graphList, approvals: approvalList, benchmarks: benchmarkList }) => {
      if (!selectedGraphId && graphList[0]) {
        setSelectedGraphId(graphList[0].snapshot.graph.graph_id);
      }
      if (!selectedApprovalId && approvalList[0]) {
        setSelectedApprovalId(approvalList[0].approval_id);
      }
      if (!selectedBenchmarkId && benchmarkList[0]) {
        setSelectedBenchmarkId(benchmarkList[0].id);
      }
    },
  });
  const { replayView, replayComparison, replayError, loading: replayLoading, queryReplay, compareReplay, setReplayComparison } = useReplay({
    replayScope,
    replayID,
    artifactID,
    onArtifactDiscovered: setArtifactID,
  });
  const { artifactView, artifactError, loading: artifactLoading } = useArtifact(artifactID);
  const { benchmarkDetail, benchmarkCompare, benchmarkMarkdown, loading: benchmarkLoading, error: benchmarkError, compareBenchmarks } = useBenchmarks(selectedBenchmarkId);

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
    void api
      .getTaskGraph(selectedGraphId)
      .then((detail) => {
        setTaskGraphDetail(detail);
        if (!replayID) {
          setReplayScope("task_graph");
          setReplayID(detail.snapshot.graph.graph_id);
        }
      })
      .catch((error: Error) => setGlobalError(error.message));
  }, [selectedGraphId, replayID]);

  useEffect(() => {
    if (!selectedApprovalId) {
      setApprovalDetail(null);
      return;
    }
    void api
      .getApproval(selectedApprovalId)
      .then(setApprovalDetail)
      .catch((error: Error) => setActionError(error.message));
  }, [selectedApprovalId]);

  useEffect(() => {
    if (bootstrapError) {
      setGlobalError(bootstrapError);
    }
  }, [bootstrapError]);

  useEffect(() => {
    if (benchmarkError) {
      setGlobalError(benchmarkError);
    }
  }, [benchmarkError]);

  async function handleApprovalAction(action: "approve" | "deny") {
    if (!approvalDetail) return;
    setActionError("");
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
    }
  }

  const artifactCandidates = useMemo(() => {
    const replayArtifacts = replayView?.workflow?.artifacts ?? replayView?.task_graph?.artifacts ?? [];
    const taskGraphArtifacts = taskGraphDetail?.artifacts ?? [];
    return replayArtifacts.length > 0 ? replayArtifacts : taskGraphArtifacts;
  }, [replayView, taskGraphDetail]);

  const loading = firstNonEmpty(
    bootstrapLoading ? "加载 operator surface" : "",
    replayLoading ? "查询 replay" : "",
    artifactLoading ? "加载 artifact 内容" : "",
    benchmarkLoading ? "加载 benchmark" : "",
  );

  return (
    <main className="app-shell">
      <section className="hero">
        <div className="hero-copy">
          <p className="eyebrow">Personal CFO OS · 7B</p>
          <h1>Operator Console for a governed, verifiable, observable agent system.</h1>
          <p className="copy">这一层只可视化现有 typed runtime、operator、replay、artifact、benchmark surfaces，不把业务判断搬进前端。</p>
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
          <button key={item.key} className={item.key === tab ? "tab-button is-active" : "tab-button"} onClick={() => setTab(item.key)} type="button">
            <span>{item.label}</span>
            <small>{item.subtitle}</small>
          </button>
        ))}
      </nav>

      {globalError ? <p className="banner is-error">{globalError}</p> : null}
      {loading ? <p className="banner">{loading}</p> : null}

      <section className="content-grid">
        {tab === "graphs" ? (
          <TaskGraphPanel taskGraphs={taskGraphs} selectedGraphId={selectedGraphId} onSelectGraph={setSelectedGraphId} taskGraphDetail={taskGraphDetail} />
        ) : null}

        {tab === "approvals" ? (
          <ApprovalPanel
            approvals={approvals}
            selectedApprovalId={selectedApprovalId}
            onSelectApproval={setSelectedApprovalId}
            approvalDetail={approvalDetail}
            actionResult={actionResult}
            actionError={actionError}
            onApprove={() => void handleApprovalAction("approve")}
            onDeny={() => void handleApprovalAction("deny")}
          />
        ) : null}

        {tab === "replay" ? (
          <ReplayPanel
            replayScope={replayScope}
            replayID={replayID}
            compareLeft={compareLeft}
            compareRight={compareRight}
            replayView={replayView}
            replayComparison={replayComparison}
            replayError={replayError}
            onChangeScope={setReplayScope}
            onChangeReplayID={setReplayID}
            onChangeCompareLeft={setCompareLeft}
            onChangeCompareRight={setCompareRight}
            onQueryReplay={() => void queryReplay()}
            onCompareReplay={() => void compareReplay(compareLeft, compareRight)}
          />
        ) : null}

        {tab === "intelligence" ? <IntelligencePanel replayView={replayView} /> : null}

        {tab === "artifacts" ? (
          <ArtifactPanel
            artifactCandidates={artifactCandidates}
            artifactID={artifactID}
            onSelectArtifact={setArtifactID}
            artifactView={artifactView}
            artifactError={artifactError}
          />
        ) : null}

        {tab === "benchmarks" ? (
          <BenchmarkPanel
            benchmarks={benchmarks}
            selectedBenchmarkId={selectedBenchmarkId}
            onSelectBenchmark={setSelectedBenchmarkId}
            benchmarkDetail={benchmarkDetail}
            benchmarkCompare={benchmarkCompare}
            benchmarkMarkdown={benchmarkMarkdown}
            onCompareBenchmarks={() => void compareBenchmarks(benchmarks)}
          />
        ) : null}
      </section>
    </main>
  );
}

function firstNonEmpty(...values: string[]) {
  for (const value of values) {
    if (value) {
      return value;
    }
  }
  return "";
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

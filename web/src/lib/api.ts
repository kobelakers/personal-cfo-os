import type {
  ApprovalDetail,
  ApprovalRecord,
  ArtifactContentView,
  ArtifactMeta,
  BenchmarkCompareView,
  BenchmarkRunDetail,
  BenchmarkRunSummary,
  ProfileMeta,
  ReplayComparison,
  ReplayQueryScope,
  ReplayView,
  TaskCommandResult,
  TaskGraphView,
} from "../types";

const API_BASE = (import.meta.env.VITE_API_BASE as string | undefined)?.replace(/\/$/, "") || "/api/v1";

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(`${API_BASE}${path}`, {
    headers: {
      "Content-Type": "application/json",
      ...(init?.headers ?? {}),
    },
    ...init,
  });
  if (!response.ok) {
    let message = `${response.status} ${response.statusText}`;
    try {
      const payload = (await response.json()) as { error?: string };
      if (payload.error) {
        message = payload.error;
      }
    } catch {
      // ignore body decode failures for non-json errors
    }
    throw new Error(message);
  }
  if (response.headers.get("content-type")?.includes("text/markdown")) {
    return (await response.text()) as T;
  }
  return (await response.json()) as T;
}

export const api = {
  profile(): Promise<ProfileMeta> {
    return request<ProfileMeta>("/meta/profile");
  },
  listTaskGraphs(): Promise<TaskGraphView[]> {
    return request<TaskGraphView[]>("/task-graphs");
  },
  getTaskGraph(id: string): Promise<TaskGraphView> {
    return request<TaskGraphView>(`/task-graphs/${id}`);
  },
  listPendingApprovals(): Promise<ApprovalRecord[]> {
    return request<ApprovalRecord[]>("/approvals/pending");
  },
  getApproval(id: string): Promise<ApprovalDetail> {
    return request<ApprovalDetail>(`/approvals/${id}`);
  },
  approveApproval(id: string, body: Record<string, unknown>): Promise<TaskCommandResult> {
    return request<TaskCommandResult>(`/approvals/${id}/approve`, {
      method: "POST",
      body: JSON.stringify(body),
    });
  },
  denyApproval(id: string, body: Record<string, unknown>): Promise<TaskCommandResult> {
    return request<TaskCommandResult>(`/approvals/${id}/deny`, {
      method: "POST",
      body: JSON.stringify(body),
    });
  },
  resumeFollowUp(taskId: string, body: Record<string, unknown>): Promise<TaskCommandResult> {
    return request<TaskCommandResult>(`/follow-ups/${taskId}/resume`, {
      method: "POST",
      body: JSON.stringify(body),
    });
  },
  retryFollowUp(taskId: string, body: Record<string, unknown>): Promise<TaskCommandResult> {
    return request<TaskCommandResult>(`/follow-ups/${taskId}/retry`, {
      method: "POST",
      body: JSON.stringify(body),
    });
  },
  reevaluateTaskGraph(graphId: string, body: Record<string, unknown>): Promise<unknown> {
    return request<unknown>(`/task-graphs/${graphId}/reevaluate`, {
      method: "POST",
      body: JSON.stringify(body),
    });
  },
  getReplay(scope: ReplayQueryScope, id: string): Promise<ReplayView> {
    const pathByScope: Record<ReplayQueryScope, string> = {
      workflow: `/replay/workflows/${id}`,
      task_graph: `/replay/task-graphs/${id}`,
      task: `/replay/tasks/${id}`,
      execution: `/replay/executions/${id}`,
      approval: `/replay/approvals/${id}`,
    };
    return request<ReplayView>(pathByScope[scope]);
  },
  compareReplay(left: string, right: string): Promise<ReplayComparison> {
    return request<ReplayComparison>(`/replay/compare?left=${encodeURIComponent(left)}&right=${encodeURIComponent(right)}`);
  },
  getArtifact(id: string): Promise<ArtifactMeta> {
    return request<ArtifactMeta>(`/artifacts/${id}`);
  },
  getArtifactContent(id: string): Promise<ArtifactContentView> {
    return request<ArtifactContentView>(`/artifacts/${id}/content`);
  },
  listBenchmarks(): Promise<BenchmarkRunSummary[]> {
    return request<BenchmarkRunSummary[]>("/benchmarks/runs");
  },
  getBenchmark(id: string): Promise<BenchmarkRunDetail> {
    return request<BenchmarkRunDetail>(`/benchmarks/runs/${id}`);
  },
  compareBenchmarks(left: string, right: string): Promise<BenchmarkCompareView> {
    return request<BenchmarkCompareView>(`/benchmarks/compare?left=${encodeURIComponent(left)}&right=${encodeURIComponent(right)}`);
  },
  exportBenchmarkMarkdown(id: string): Promise<string> {
    return request<string>(`/benchmarks/exports/${id}?format=md`);
  },
};

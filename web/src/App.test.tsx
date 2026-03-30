import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import App from "./App";

const baseResponses: Record<string, unknown> = {
  "/api/v1/meta/profile": {
    runtime_profile: "interview-demo",
    runtime_backend: "sqlite",
    blob_backend: "localfs",
    ui_mode: "served-dist",
    supported_schema_versions: ["v1"],
    benchmark_catalog: ["phase6b"],
  },
  "/api/v1/task-graphs": [
    {
      snapshot: {
        graph: {
          graph_id: "graph-1",
          parent_workflow_id: "workflow-life-event-1",
          parent_task_id: "task-parent-1",
          generation_summary: "life event generated follow-up tasks",
          dependencies: [{ upstream_task_id: "task-tax", downstream_task_id: "task-portfolio", reason: "tax first" }],
        },
        registered_tasks: [
          {
            task: { id: "task-tax", goal: "optimize taxes", user_intent_type: "tax_optimization" },
            status: "completed",
          },
        ],
        deferred: [{ task_id: "task-portfolio", reason: "due window pending" }],
      },
      pending_approval: { approval_id: "approval-1" },
      artifacts: [{ id: "artifact-1", kind: "replay_bundle", workflow_id: "workflow-life-event-1", task_id: "task-tax" }],
    },
  ],
  "/api/v1/task-graphs/graph-1": {
    snapshot: {
      graph: {
        graph_id: "graph-1",
        parent_workflow_id: "workflow-life-event-1",
        parent_task_id: "task-parent-1",
        generation_summary: "life event generated follow-up tasks",
        dependencies: [{ upstream_task_id: "task-tax", downstream_task_id: "task-portfolio", reason: "tax first" }],
      },
      registered_tasks: [
        {
          task: { id: "task-tax", goal: "optimize taxes", user_intent_type: "tax_optimization" },
          status: "completed",
        },
      ],
      deferred: [{ task_id: "task-portfolio", reason: "due window pending" }],
      executed_tasks: [{ execution_id: "exec-1", task_id: "task-tax", workflow_id: "workflow-child-tax", status: "completed" }],
    },
    pending_approval: { approval_id: "approval-1" },
    artifacts: [{ id: "artifact-1", kind: "replay_bundle", workflow_id: "workflow-life-event-1", task_id: "task-tax" }],
    actions: [],
  },
  "/api/v1/approvals/pending": [
    {
      approval_id: "approval-1",
      graph_id: "graph-1",
      task_id: "task-behavior-1",
      workflow_id: "workflow-behavior-1",
      requested_action: "follow_up_execution",
      requested_at: "2026-03-31T10:00:00Z",
      status: "pending",
      version: 2,
    },
  ],
  "/api/v1/approvals/approval-1": {
    approval: {
      approval_id: "approval-1",
      graph_id: "graph-1",
      task_id: "task-behavior-1",
      workflow_id: "workflow-behavior-1",
      requested_action: "follow_up_execution",
      requested_at: "2026-03-31T10:00:00Z",
      status: "pending",
      version: 2,
    },
    task_graph: {
      snapshot: {
        graph: {
          graph_id: "graph-1",
          parent_workflow_id: "workflow-life-event-1",
          parent_task_id: "task-parent-1",
        },
      },
    },
    replay_hint: { approval_id: "approval-1" },
  },
  "/api/v1/replay/task-graphs/graph-1": {
    scope: { kind: "task_graph", id: "graph-1" },
    task_graph: {
      task_graph_id: "graph-1",
      parent_workflow_id: "workflow-life-event-1",
      task_ids: ["task-tax", "task-portfolio"],
      execution_ids: ["exec-1"],
      artifacts: [{ id: "artifact-1", kind: "replay_bundle", workflow_id: "workflow-life-event-1", task_id: "task-tax" }],
    },
    summary: {
      final_state: "completed",
      plan_summary: ["planner blocks=task-tax,task-portfolio"],
      memory_summary: ["selected=memory-1", "rejected=memory-2"],
      skill_summary: ["selected=subscription_cleanup/subscription_cleanup.v1"],
      async_runtime_summary: ["work claimed by worker-a"],
    },
    explanation: {
      why_async_runtime: ["approval resumed on worker-b"],
      why_memory_decision: ["memory selected because recent"],
    },
    provenance: {
      nodes: [{ id: "workflow:1", type: "workflow", label: "workflow-1" }],
      edges: [{ id: "edge-1", type: "generated_task", from_node_id: "workflow:1", to_node_id: "task:1" }],
    },
    degraded: true,
    degradation_reasons: [{ reason: "projection_incomplete", message: "best effort assembly" }],
  },
  "/api/v1/replay/workflows/workflow-1": {
    scope: { kind: "workflow", id: "workflow-1" },
    workflow: {
      workflow_id: "workflow-1",
      runtime_state: "completed",
      summary: "workflow replay summary",
      artifacts: [{ id: "artifact-1", kind: "replay_bundle", workflow_id: "workflow-1", task_id: "task-1" }],
    },
    summary: {
      final_state: "completed",
      plan_summary: ["plan ready"],
      memory_summary: ["selected=memory-1"],
      async_runtime_summary: ["lease renewed"],
    },
    explanation: {
      why_async_runtime: ["scheduler wakeup triggered child workflow"],
    },
    provenance: { nodes: [], edges: [] },
    degraded: true,
    degradation_reasons: [{ reason: "projection_stale", message: "rebuilder suggested" }],
  },
  "/api/v1/replay/compare?left=workflow%3Aworkflow-1&right=approval%3Aapproval-1": {
    left: { kind: "workflow", id: "workflow-1" },
    right: { kind: "approval", id: "approval-1" },
    summary: ["memory changed", "runtime changed"],
    diffs: [{ category: "memory", field: "selection", summary: "memory changed" }],
  },
  "/api/v1/artifacts/artifact-1/content": {
    artifact: {
      id: "artifact-1",
      kind: "replay_bundle",
      workflow_id: "workflow-life-event-1",
      task_id: "task-tax",
      ref: { location: "var/blob/artifact-1.json" },
      created_at: "2026-03-31T10:00:00Z",
    },
    content_type: "application/json",
    structured: { scenario: "interview-demo" },
    usage_summary: {
      providers: ["openai"],
      models: ["gpt-5.4"],
      prompt_ids: ["planner"],
      total_tokens: 320,
      estimated_cost_usd: 0.14,
      call_count: 1,
    },
    summary_lines: ["artifact=artifact-1 kind=replay_bundle"],
  },
  "/api/v1/benchmarks/runs": [
    {
      id: "phase6b",
      source: "sample",
      source_ref: "docs/eval/samples/phase6b_eval_default_corpus.json",
      title: "phase6b",
      corpus_id: "phase6b-default",
      run_id: "phase6b-default-run",
      deterministic_only: true,
      scenario_count: 4,
      passed_count: 4,
      failed_count: 0,
      approval_frequency: 0.25,
      average_latency_ms: 42,
      total_token_usage: 780,
      validator_pass_rate: 1,
      policy_violation_rate: 0,
      cost_summary: { usd: 0.0142, precision: "estimated_from_tokens", source: "token_policy_v1" },
    },
    {
      id: "phase6a",
      source: "sample",
      source_ref: "docs/eval/samples/phase6a_eval_default_corpus.json",
      title: "phase6a",
      corpus_id: "phase6a-default",
      run_id: "phase6a-default-run",
      deterministic_only: true,
      scenario_count: 11,
      passed_count: 11,
      failed_count: 0,
      approval_frequency: 0.18,
      average_latency_ms: 37,
      total_token_usage: 640,
      validator_pass_rate: 1,
      policy_violation_rate: 0,
      cost_summary: { usd: 0.0116, precision: "estimated_from_tokens", source: "token_policy_v1" },
    },
  ],
  "/api/v1/benchmarks/runs/phase6b": {
    summary: {
      id: "phase6b",
      source: "sample",
      source_ref: "docs/eval/samples/phase6b_eval_default_corpus.json",
      title: "phase6b",
      corpus_id: "phase6b-default",
      run_id: "phase6b-default-run",
      deterministic_only: true,
      scenario_count: 4,
      passed_count: 4,
      failed_count: 0,
      approval_frequency: 0.25,
      average_latency_ms: 42,
      total_token_usage: 780,
      validator_pass_rate: 1,
      policy_violation_rate: 0,
      cost_summary: { usd: 0.0142, precision: "estimated_from_tokens", source: "token_policy_v1" },
    },
    run: {
      summary: {
        summary_lines: ["phase6b deterministic corpus"],
      },
      results: [{ scenario_id: "behavior_intervention_happy_path", passed: true, runtime_state: "completed", token_usage: 240, duration_milliseconds: 40 }],
    },
  },
  "/api/v1/benchmarks/compare?left=phase6b&right=phase6a": {
    left: { id: "phase6b", title: "phase6b", corpus_id: "phase6b-default" },
    right: { id: "phase6a", title: "phase6a", corpus_id: "phase6a-default" },
    diff: { summary: ["scenario set changed"] },
    summary: ["scenario set changed"],
  },
};

describe("App", () => {
  beforeEach(() => {
    window.history.replaceState({}, "", "/");
  });

  afterEach(() => {
    cleanup();
    vi.restoreAllMocks();
  });

  it("renders profile and task graph viewer from /api/v1", async () => {
    installFetchMock();
    render(<App />);

    await waitFor(() => expect(screen.getByText("interview-demo")).toBeInTheDocument());
    expect(screen.getByText("Task Graph Viewer")).toBeInTheDocument();
    expect(screen.getByText("graph-1")).toBeInTheDocument();
    expect(screen.getByText("life event generated follow-up tasks")).toBeInTheDocument();
  });

  it("shows approval conflicts without bypassing operator service", async () => {
    installFetchMock({
      "/api/v1/approvals/approval-1/approve": {
        status: 409,
        body: { error: "approval \"approval-1\" conflict: version mismatch" },
      },
    });
    render(<App />);

    await waitFor(() => expect(screen.getByText("approval-1")).toBeInTheDocument());
    fireEvent.click(screen.getByRole("button", { name: /Approvals/i }));
    await waitFor(() => expect(screen.getByText("Approval Detail")).toBeInTheDocument());
    fireEvent.click(screen.getByText("Approve"));
    await waitFor(() => expect(screen.getByText(/version mismatch/)).toBeInTheDocument());
  });

  it("renders replay degradation and compare summary", async () => {
    installFetchMock();
    window.history.replaceState({}, "", "/?tab=replay&replayScope=workflow&replayId=workflow-1");
    render(<App />);

    await waitFor(() => expect(screen.getByText("Replay Result")).toBeInTheDocument());
    expect(screen.getByText("projection_stale · rebuilder suggested")).toBeInTheDocument();
    fireEvent.change(screen.getByPlaceholderText("workflow:workflow-id"), { target: { value: "workflow:workflow-1" } });
    fireEvent.change(screen.getByPlaceholderText("approval:approval-id"), { target: { value: "approval:approval-1" } });
    fireEvent.click(screen.getByText("Compare"));
    await waitFor(() => expect(screen.getByText("memory changed")).toBeInTheDocument());
  });

  it("renders benchmark catalog, compare summary, and artifact usage", async () => {
    installFetchMock();
    window.history.replaceState({}, "", "/?tab=benchmarks&artifact=artifact-1");
    render(<App />);

    await waitFor(() => expect(screen.getByText("phase6b")).toBeInTheDocument());
    fireEvent.click(screen.getByText("Compare First Two"));
    await waitFor(() => expect(screen.getByText("scenario set changed")).toBeInTheDocument());
    fireEvent.click(screen.getByText("Artifacts"));
    await waitFor(() => expect(screen.getByText("Prompt / Provider / Cost Evidence")).toBeInTheDocument());
    expect(screen.getByText("providers=openai")).toBeInTheDocument();
  });
});

function installFetchMock(overrides: Record<string, { status?: number; body: unknown }> = {}) {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL) => {
      const url = new URL(typeof input === "string" ? input : input.toString(), "http://localhost");
      const path = `${url.pathname}${url.search}`;
      if (path.includes("/benchmarks/exports/")) {
        return new Response("# Benchmark phase6b\n\n- export", {
          status: 200,
          headers: { "Content-Type": "text/markdown; charset=utf-8" },
        });
      }
      const override = overrides[path];
      if (override) {
        return new Response(JSON.stringify(override.body), {
          status: override.status ?? 200,
          headers: { "Content-Type": "application/json" },
        });
      }
      const body = baseResponses[path];
      if (body === undefined) {
        return new Response(JSON.stringify({ error: `missing mock for ${path}` }), {
          status: 404,
          headers: { "Content-Type": "application/json" },
        });
      }
      return new Response(JSON.stringify(body), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      });
    }),
  );
}

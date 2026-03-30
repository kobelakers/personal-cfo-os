export type ReplayQueryScope = "workflow" | "task_graph" | "task" | "execution" | "approval";

export interface ProfileMeta {
  runtime_profile: string;
  runtime_backend: string;
  blob_backend: string;
  ui_mode: string;
  supported_schema_versions: string[];
  benchmark_catalog?: string[];
}

export interface GeneratedTask {
  task: {
    id: string;
    goal: string;
    user_intent_type: string;
  };
}

export interface TaskDependency {
  upstream_task_id: string;
  downstream_task_id: string;
  reason: string;
}

export interface FollowUpTaskRecord {
  task: {
    id: string;
    goal: string;
    user_intent_type: string;
  };
  status: string;
  required_capability?: string;
  missing_capability_reason?: string;
  blocking_reasons?: string[];
}

export interface TaskGraphView {
  snapshot: {
    graph: {
      graph_id: string;
      parent_workflow_id: string;
      parent_task_id: string;
      generation_summary?: string;
      dependencies?: TaskDependency[];
      generated_tasks?: GeneratedTask[];
    };
    registered_tasks?: FollowUpTaskRecord[];
    deferred?: Array<{
      task_id: string;
      reason: string;
      not_before?: string;
      not_after?: string;
    }>;
    executed_tasks?: Array<{
      execution_id: string;
      task_id: string;
      workflow_id: string;
      status: string;
      failure_summary?: string;
    }>;
  };
  pending_approval?: ApprovalRecord;
  artifacts?: ArtifactMeta[];
  actions?: OperatorActionRecord[];
}

export interface ApprovalRecord {
  approval_id: string;
  graph_id: string;
  task_id: string;
  workflow_id: string;
  execution_id?: string;
  requested_action: string;
  requested_at: string;
  status: string;
  resolved_at?: string;
  resolved_by?: string;
  resolution_note?: string;
  version: number;
}

export interface ApprovalDetail {
  approval: ApprovalRecord;
  task_graph?: TaskGraphView;
  replay_hint: {
    approval_id?: string;
  };
}

export interface OperatorActionRecord {
  action_id: string;
  request_id: string;
  action_type: string;
  actor: string;
  status: string;
  note?: string;
  requested_at: string;
  applied_at?: string;
  failure_summary?: string;
}

export interface ReplayArtifactRef {
  id: string;
  kind: string;
  workflow_id?: string;
  task_id?: string;
  location?: string;
  summary?: string;
  created_at?: string;
}

export interface ReplayView {
  scope: {
    kind: string;
    id: string;
  };
  workflow?: {
    workflow_id: string;
    task_id?: string;
    intent?: string;
    runtime_state: string;
    failure_summary?: string;
    summary?: string;
    artifacts?: ReplayArtifactRef[];
  };
  task_graph?: {
    task_graph_id: string;
    parent_workflow_id?: string;
    parent_task_id?: string;
    pending_approval_id?: string;
    task_ids?: string[];
    execution_ids?: string[];
    artifacts?: ReplayArtifactRef[];
  };
  approval?: {
    approval_id: string;
    workflow_id?: string;
    task_graph_id?: string;
    task_id?: string;
    status: string;
    requested_action?: string;
    requested_at?: string;
    resolved_at?: string;
    resolved_by?: string;
  };
  summary: {
    goal_summary?: string;
    plan_summary?: string[];
    skill_summary?: string[];
    memory_summary?: string[];
    validator_summary?: string[];
    governance_summary?: string[];
    async_runtime_summary?: string[];
    child_workflow_summary?: string[];
    final_state?: string;
  };
  explanation: {
    why_failed?: string;
    why_waiting_approval?: string;
    why_skill_selected?: string[];
    why_generated_task?: string[];
    why_child_executed?: string[];
    why_memory_decision?: string[];
    why_validation_failed?: string[];
    why_async_runtime?: string[];
  };
  provenance: {
    nodes?: Array<{
      id: string;
      type: string;
      label: string;
      summary?: string;
      attributes?: Record<string, string>;
    }>;
    edges?: Array<{
      id: string;
      type: string;
      from_node_id: string;
      to_node_id: string;
      reason?: string;
    }>;
  };
  degraded: boolean;
  degradation_reasons?: Array<{
    reason: string;
    message: string;
  }>;
  projection_status?: string;
  projection_version?: number;
}

export interface ReplayComparison {
  left: { kind: string; id: string };
  right: { kind: string; id: string };
  diffs?: Array<{
    category: string;
    field: string;
    summary: string;
    details?: string[];
    left?: string[];
    right?: string[];
  }>;
  summary?: string[];
}

export interface ArtifactMeta {
  id: string;
  kind: string;
  workflow_id: string;
  task_id: string;
  produced_by?: string;
  ref?: {
    id?: string;
    summary?: string;
    location?: string;
  };
  created_at: string;
  content_json?: string;
}

export interface ArtifactContentView {
  artifact: ArtifactMeta;
  content_type: string;
  structured?: unknown;
  raw_text?: string;
  usage_summary?: {
    providers?: string[];
    models?: string[];
    prompt_ids?: string[];
    total_tokens?: number;
    estimated_cost_usd?: number;
    call_count?: number;
  };
  summary_lines?: string[];
  reference_only?: boolean;
}

export interface TaskCommandResult {
  action: OperatorActionRecord;
  graph_id: string;
  task_id?: string;
  approval_id?: string;
  execution_id?: string;
  status?: string;
  failure_summary?: string;
  enqueued_work_kinds?: string[];
  async_dispatch_accepted?: boolean;
}

export interface BenchmarkRunSummary {
  id: string;
  title: string;
  corpus_id: string;
  run_id: string;
  deterministic_only: boolean;
  scenario_count: number;
  passed_count: number;
  failed_count: number;
  approval_frequency: number;
  average_latency_ms: number;
  total_token_usage: number;
  validator_pass_rate: number;
  policy_violation_rate: number;
}

export interface BenchmarkRunDetail {
  summary: BenchmarkRunSummary;
  run: {
    summary: {
      summary_lines?: string[];
      passed_scenarios?: string[];
      failed_scenarios?: string[];
      approval_scenarios?: string[];
    };
    results: Array<{
      scenario_id: string;
      passed: boolean;
      runtime_state: string;
      token_usage: number;
      duration_milliseconds: number;
      regression_failures?: Array<{ message: string }>;
    }>;
  };
}

export interface BenchmarkCompareView {
  left: BenchmarkRunSummary;
  right: BenchmarkRunSummary;
  diff: {
    summary?: string[];
    differences?: Array<{
      scenario_id: string;
      summary: string;
      details?: string[];
    }>;
  };
  summary?: string[];
  description?: string;
}

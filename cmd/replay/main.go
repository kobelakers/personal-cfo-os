package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/kobelakers/personal-cfo-os/internal/observability"
	"github.com/kobelakers/personal-cfo-os/internal/runtime"
)

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string, stdout io.Writer, stderr io.Writer) error {
	var (
		fs           = flag.NewFlagSet("replay", flag.ContinueOnError)
		runtimeDB    = fs.String("runtime-db", fallbackRuntimeDBPath(), "runtime sqlite db path")
		workflowID   = fs.String("workflow-id", "", "query replay by workflow id")
		taskGraphID  = fs.String("task-graph-id", "", "query replay by task graph id")
		taskID       = fs.String("task-id", "", "query replay by task id")
		executionID  = fs.String("execution-id", "", "query replay by execution id")
		approvalID   = fs.String("approval-id", "", "query replay by approval id")
		compareLeft  = fs.String("compare-left", "", "compare left replay scope in the form workflow:<id>, task_graph:<id>, task:<id>, execution:<id>, approval:<id>")
		compareRight = fs.String("compare-right", "", "compare right replay scope in the form workflow:<id>, task_graph:<id>, task:<id>, execution:<id>, approval:<id>")
		format       = fs.String("format", "summary", "output format: json or summary")
		rebuild      = fs.Bool("rebuild-projections", false, "rebuild replay/debug projections from authoritative runtime truth")
		all          = fs.Bool("all", false, "rebuild all replay/debug projections")
	)
	fs.SetOutput(stderr)
	if err := fs.Parse(args); err != nil {
		return err
	}

	stores, err := runtime.NewSQLiteRuntimeStores(*runtimeDB)
	if err != nil {
		return fmt.Errorf("open runtime stores: %w", err)
	}
	defer func() { _ = stores.DB.Close() }()
	service := runtime.NewService(runtime.ServiceOptions{
		CheckpointStore: stores.Checkpoints,
		TaskGraphs:      stores.TaskGraphs,
		Executions:      stores.Executions,
		Approvals:       stores.Approvals,
		OperatorActions: stores.OperatorActions,
		Replay:          stores.Replay,
		Artifacts:       stores.Artifacts,
		Controller:      runtime.DefaultWorkflowController{},
		Now:             func() time.Time { return time.Now().UTC() },
	})
	rebuilder := runtime.NewReplayProjectionRebuilder(service, stores.WorkflowRuns, stores.ReplayProjection, stores.Artifacts, stores.Replay, func() time.Time { return time.Now().UTC() })
	service.SetReplayProjectionWriter(rebuilder)
	queryService := runtime.NewReplayQueryService(service, stores.WorkflowRuns, stores.ReplayQuery, stores.Artifacts, stores.Replay)

	ctx := context.Background()
	if *rebuild {
		if err := runRebuild(ctx, rebuilder, strings.TrimSpace(*workflowID), strings.TrimSpace(*taskGraphID), *all, *format, stdout); err != nil {
			return fmt.Errorf("rebuild replay projections: %w", err)
		}
		if emptyQueryFlags(*workflowID, *taskGraphID, *taskID, *executionID, *approvalID, *compareLeft, *compareRight) {
			return nil
		}
	}

	if strings.TrimSpace(*compareLeft) != "" || strings.TrimSpace(*compareRight) != "" {
		if strings.TrimSpace(*compareLeft) == "" || strings.TrimSpace(*compareRight) == "" {
			return fmt.Errorf("--compare-left and --compare-right must be provided together")
		}
		left, err := parseScopedReplayQuery(*compareLeft)
		if err != nil {
			return fmt.Errorf("parse compare-left: %w", err)
		}
		right, err := parseScopedReplayQuery(*compareRight)
		if err != nil {
			return fmt.Errorf("parse compare-right: %w", err)
		}
		comparison, err := queryService.Compare(ctx, left, right)
		if err != nil {
			return fmt.Errorf("compare replay views: %w", err)
		}
		printComparison(stdout, comparison, *format)
		return nil
	}

	query, err := buildReplayQuery(*workflowID, *taskGraphID, *taskID, *executionID, *approvalID)
	if err != nil {
		return err
	}
	view, err := queryService.Query(ctx, query)
	if err != nil {
		return fmt.Errorf("query replay view: %w", err)
	}
	printReplayView(stdout, view, *format)
	return nil
}

func runRebuild(ctx context.Context, rebuilder *runtime.ReplayProjectionRebuilder, workflowID string, taskGraphID string, all bool, format string, stdout io.Writer) error {
	switch {
	case all:
		builds, err := rebuilder.RebuildAll(ctx)
		if err != nil {
			return err
		}
		printValue(stdout, builds, format)
		return nil
	case workflowID != "":
		build, err := rebuilder.RebuildWorkflow(ctx, workflowID)
		if err != nil {
			return err
		}
		printValue(stdout, build, format)
		return nil
	case taskGraphID != "":
		build, err := rebuilder.RebuildTaskGraph(ctx, taskGraphID)
		if err != nil {
			return err
		}
		printValue(stdout, build, format)
		return nil
	default:
		return fmt.Errorf("rebuild requires --workflow-id, --task-graph-id, or --all")
	}
}

func buildReplayQuery(workflowID string, taskGraphID string, taskID string, executionID string, approvalID string) (observability.ReplayQuery, error) {
	query := observability.ReplayQuery{
		WorkflowID:  strings.TrimSpace(workflowID),
		TaskGraphID: strings.TrimSpace(taskGraphID),
		TaskID:      strings.TrimSpace(taskID),
		ExecutionID: strings.TrimSpace(executionID),
		ApprovalID:  strings.TrimSpace(approvalID),
	}
	count := 0
	if query.WorkflowID != "" {
		count++
	}
	if query.TaskGraphID != "" {
		count++
	}
	if query.TaskID != "" {
		count++
	}
	if query.ExecutionID != "" {
		count++
	}
	if query.ApprovalID != "" {
		count++
	}
	if count != 1 {
		return observability.ReplayQuery{}, fmt.Errorf("exactly one of --workflow-id, --task-graph-id, --task-id, --execution-id, or --approval-id is required")
	}
	return query, nil
}

func parseScopedReplayQuery(raw string) (observability.ReplayQuery, error) {
	scope, id, ok := strings.Cut(strings.TrimSpace(raw), ":")
	if !ok || strings.TrimSpace(id) == "" {
		return observability.ReplayQuery{}, fmt.Errorf("expected scope:id, got %q", raw)
	}
	switch strings.TrimSpace(scope) {
	case "workflow":
		return observability.ReplayQuery{WorkflowID: strings.TrimSpace(id)}, nil
	case "task_graph":
		return observability.ReplayQuery{TaskGraphID: strings.TrimSpace(id)}, nil
	case "task":
		return observability.ReplayQuery{TaskID: strings.TrimSpace(id)}, nil
	case "execution":
		return observability.ReplayQuery{ExecutionID: strings.TrimSpace(id)}, nil
	case "approval":
		return observability.ReplayQuery{ApprovalID: strings.TrimSpace(id)}, nil
	default:
		return observability.ReplayQuery{}, fmt.Errorf("unsupported replay scope %q", scope)
	}
}

func printReplayView(stdout io.Writer, view observability.ReplayView, format string) {
	switch format {
	case "json":
		printValue(stdout, view, format)
	default:
		summary := observability.BuildDebugSummaryFromReplay(view)
		fmt.Fprintf(stdout, "scope=%s:%s\n", view.Scope.Kind, view.Scope.ID)
		fmt.Fprintf(stdout, "final_state=%s degraded=%t projection_status=%s projection_version=%d\n", view.Summary.FinalState, view.Degraded, view.ProjectionStatus, view.ProjectionVersion)
		if summary.WorkflowID != "" {
			fmt.Fprintf(stdout, "workflow_id=%s\n", summary.WorkflowID)
		}
		if summary.TaskGraphID != "" {
			fmt.Fprintf(stdout, "task_graph_id=%s\n", summary.TaskGraphID)
		}
		printSection(stdout, "plan", summary.PlanSummary)
		printSection(stdout, "skill", summary.SkillSummary)
		printSection(stdout, "memory", summary.MemorySummary)
		printSection(stdout, "validation", summary.ValidatorSummary)
		printSection(stdout, "governance", summary.GovernanceSummary)
		printSection(stdout, "child_workflows", summary.ChildWorkflows)
		printSection(stdout, "explanation", summary.Explanation)
		if len(view.DegradationReasons) > 0 {
			lines := make([]string, 0, len(view.DegradationReasons))
			for _, item := range view.DegradationReasons {
				lines = append(lines, fmt.Sprintf("%s: %s", item.Reason, item.Message))
			}
			printSection(stdout, "degradation", lines)
		}
	}
}

func printComparison(stdout io.Writer, comparison observability.ReplayComparison, format string) {
	if format == "json" {
		printValue(stdout, comparison, format)
		return
	}
	fmt.Fprintf(stdout, "compare_left=%s:%s\n", comparison.Left.Kind, comparison.Left.ID)
	fmt.Fprintf(stdout, "compare_right=%s:%s\n", comparison.Right.Kind, comparison.Right.ID)
	printSection(stdout, "summary", comparison.Summary)
	for _, diff := range comparison.Diffs {
		fmt.Fprintf(stdout, "[%s] %s\n", diff.Category, diff.Summary)
		if len(diff.Left) > 0 {
			fmt.Fprintf(stdout, "  left=%s\n", strings.Join(diff.Left, " | "))
		}
		if len(diff.Right) > 0 {
			fmt.Fprintf(stdout, "  right=%s\n", strings.Join(diff.Right, " | "))
		}
		for _, detail := range diff.Details {
			fmt.Fprintf(stdout, "  detail=%s\n", detail)
		}
	}
}

func printSection(stdout io.Writer, title string, lines []string) {
	if len(lines) == 0 {
		return
	}
	fmt.Fprintf(stdout, "%s:\n", title)
	for _, line := range lines {
		fmt.Fprintf(stdout, "- %s\n", line)
	}
}

func printValue(stdout io.Writer, value any, format string) {
	switch format {
	case "json":
		encoder := json.NewEncoder(stdout)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(value); err != nil {
			failf("encode json: %v", err)
		}
	default:
		payload, err := json.MarshalIndent(value, "", "  ")
		if err != nil {
			failf("encode summary payload: %v", err)
		}
		fmt.Fprintln(stdout, string(payload))
	}
}

func emptyQueryFlags(workflowID string, taskGraphID string, taskID string, executionID string, approvalID string, compareLeft string, compareRight string) bool {
	return strings.TrimSpace(workflowID) == "" &&
		strings.TrimSpace(taskGraphID) == "" &&
		strings.TrimSpace(taskID) == "" &&
		strings.TrimSpace(executionID) == "" &&
		strings.TrimSpace(approvalID) == "" &&
		strings.TrimSpace(compareLeft) == "" &&
		strings.TrimSpace(compareRight) == ""
}

func fallbackRuntimeDBPath() string {
	if env := strings.TrimSpace(os.Getenv("RUNTIME_DB_PATH")); env != "" {
		return env
	}
	return filepath.Join(".", "var", "runtime.db")
}

func failf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}

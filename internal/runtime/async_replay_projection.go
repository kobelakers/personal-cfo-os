package runtime

import (
	"fmt"
	"slices"
	"strings"

	"github.com/kobelakers/personal-cfo-os/internal/observability"
)

func asyncRuntimeSummaryFromEvents(events []ReplayEventRecord) []string {
	result := make([]string, 0)
	for _, event := range events {
		details, ok := decodeAsyncReplayDetails(event.DetailsJSON)
		if !ok {
			continue
		}
		switch event.ActionType {
		case "work_claimed":
			result = append(result, fmt.Sprintf("claimed=%s:%s:%s", details.WorkerID, details.WorkItemKind, details.WorkItemID))
		case "lease_heartbeat":
			result = append(result, fmt.Sprintf("heartbeat=%s:%s", details.WorkerID, details.WorkItemID))
		case "lease_reclaimed":
			result = append(result, fmt.Sprintf("reclaimed=%s:%s", details.WorkItemID, firstNonEmpty(details.ReclaimReason, "lease_reclaimed")))
		case "scheduler_wakeup_dispatch":
			result = append(result, fmt.Sprintf("scheduler=%s:%s", details.SchedulerDecision, details.WorkItemKind))
		case "reevaluate_task_graph":
			result = append(result, fmt.Sprintf("reevaluate=%s ready=%s", details.GraphID, strings.Join(details.ReadyTaskIDs, ",")))
		case "execute_ready_task":
			result = append(result, fmt.Sprintf("executed=%s", strings.Join(details.ExecutedTaskIDs, ",")))
		case "resume_approved_checkpoint":
			result = append(result, fmt.Sprintf("resume=%s worker=%s", details.ApprovalID, details.WorkerID))
		case "retry_failed_execution":
			result = append(result, fmt.Sprintf("retry=%s worker=%s", details.ExecutionID, details.WorkerID))
		}
	}
	return dedupeAsyncStrings(result)
}

func asyncRuntimeExplanationFromEvents(events []ReplayEventRecord) []string {
	result := make([]string, 0)
	for _, event := range events {
		details, ok := decodeAsyncReplayDetails(event.DetailsJSON)
		if !ok {
			continue
		}
		switch event.ActionType {
		case "scheduler_wakeup_dispatch":
			result = append(result, fmt.Sprintf("scheduler dispatched %s because %s", details.WorkItemKind, firstNonEmpty(details.SchedulerDecision, event.Summary)))
		case "work_claimed":
			result = append(result, fmt.Sprintf("worker %s claimed %s with lease %s", details.WorkerID, details.WorkItemID, details.LeaseID))
		case "lease_reclaimed":
			result = append(result, fmt.Sprintf("work item %s was reclaimed because %s", details.WorkItemID, firstNonEmpty(details.ReclaimReason, "lease expired")))
		case "resume_approved_checkpoint":
			result = append(result, fmt.Sprintf("approval %s resumed on worker %s", details.ApprovalID, details.WorkerID))
		case "retry_failed_execution":
			result = append(result, fmt.Sprintf("execution %s retried after %s", details.ExecutionID, firstNonEmpty(details.RetryBackoffDecision, "backoff scheduling")))
		}
	}
	return dedupeAsyncStrings(result)
}

func dedupeAsyncStrings(values []string) []string {
	result := make([]string, 0, len(values))
	for _, item := range values {
		if strings.TrimSpace(item) == "" || slices.Contains(result, item) {
			continue
		}
		result = append(result, item)
	}
	return result
}

func augmentProvenanceWithAsyncEvents(graph observability.ProvenanceGraph, events []ReplayEventRecord) observability.ProvenanceGraph {
	nodes := append([]observability.ProvenanceNode{}, graph.Nodes...)
	edges := append([]observability.ProvenanceEdge{}, graph.Edges...)
	hasNode := make(map[string]struct{}, len(nodes))
	for _, node := range nodes {
		hasNode[node.ID] = struct{}{}
	}
	addNode := func(node observability.ProvenanceNode) {
		if _, ok := hasNode[node.ID]; ok {
			return
		}
		hasNode[node.ID] = struct{}{}
		nodes = append(nodes, node)
	}
	for _, event := range events {
		details, ok := decodeAsyncReplayDetails(event.DetailsJSON)
		if !ok || details.WorkItemID == "" {
			continue
		}
		workNodeID := "work_item:" + details.WorkItemID
		addNode(observability.ProvenanceNode{
			ID:      workNodeID,
			Type:    "work_item",
			RefID:   details.WorkItemID,
			Label:   details.WorkItemKind,
			Summary: event.Summary,
		})
		if details.WorkerID != "" {
			workerNodeID := "worker:" + details.WorkerID
			addNode(observability.ProvenanceNode{
				ID:    workerNodeID,
				Type:  "worker",
				RefID: details.WorkerID,
				Label: details.WorkerID,
			})
			edges = append(edges, observability.ProvenanceEdge{
				ID:         fmt.Sprintf("edge:%s:%s", workerNodeID, workNodeID),
				FromNodeID: workerNodeID,
				ToNodeID:   workNodeID,
				Type:       "claimed_work_item",
				Reason:     event.ActionType,
			})
		}
		if details.GraphID != "" {
			edges = append(edges, observability.ProvenanceEdge{
				ID:         fmt.Sprintf("edge:graph:%s:%s", details.GraphID, details.WorkItemID),
				FromNodeID: "task_graph:" + details.GraphID,
				ToNodeID:   workNodeID,
				Type:       "scheduled_work_item",
				Reason:     event.ActionType,
			})
		}
	}
	graph.Nodes = nodes
	graph.Edges = edges
	return graph
}

func decodeAsyncReplayDetails(raw string) (AsyncReplayEventDetails, bool) {
	if strings.TrimSpace(raw) == "" {
		return AsyncReplayEventDetails{}, false
	}
	var details AsyncReplayEventDetails
	if err := unmarshalJSON(raw, &details); err != nil {
		return AsyncReplayEventDetails{}, false
	}
	if details.WorkItemID == "" && details.WorkItemKind == "" && details.WorkerID == "" && details.GraphID == "" && details.ExecutionID == "" && details.ApprovalID == "" {
		return AsyncReplayEventDetails{}, false
	}
	return details, true
}

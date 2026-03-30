import type { ApprovalDetail, ApprovalRecord, TaskCommandResult } from "../types";
import { EmptyState, KeyValueGrid, Panel } from "./common";

interface ApprovalPanelProps {
  approvals: ApprovalRecord[];
  selectedApprovalId: string;
  onSelectApproval: (approvalId: string) => void;
  approvalDetail: ApprovalDetail | null;
  actionResult: TaskCommandResult | null;
  actionError: string;
  onApprove: () => void;
  onDeny: () => void;
}

export function ApprovalPanel(props: ApprovalPanelProps) {
  const { approvals, selectedApprovalId, onSelectApproval, approvalDetail, actionResult, actionError, onApprove, onDeny } = props;
  return (
    <>
      <Panel title="Pending Approvals" subtitle="pending list">
        <ul className="list">
          {approvals.map((item) => (
            <li key={item.approval_id}>
              <button
                className={item.approval_id === selectedApprovalId ? "list-button is-active" : "list-button"}
                type="button"
                onClick={() => onSelectApproval(item.approval_id)}
              >
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
              <button type="button" onClick={onApprove}>
                Approve
              </button>
              <button type="button" className="danger" onClick={onDeny}>
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
  );
}

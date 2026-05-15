package store

import (
	"strings"
)

func normalizeCancelReason(reason CancelReasonInput) CancelReasonInput {
	if reason.TerminalReason == "" {
		reason.TerminalReason = terminalReasonForCancelReason(reason.CancelReason)
	}
	if reason.CancelReason == "" {
		reason.CancelReason = cancelReasonForTerminalReason(reason.TerminalReason)
	}
	if reason.TerminalReason == "" || reason.CancelReason == "" {
		classified := classifyCancelReason(reason.Message)
		if reason.TerminalReason == "" {
			reason.TerminalReason = classified.TerminalReason
		}
		if reason.CancelReason == "" {
			reason.CancelReason = classified.CancelReason
		}
	}
	if strings.TrimSpace(reason.Message) == "" {
		reason.Message = defaultCancelMessage(reason.CancelReason)
	}
	return reason
}

func classifyCancelReason(message string) CancelReasonInput {
	if strings.TrimSpace(message) == "" {
		message = "cancelled"
	}
	lower := strings.ToLower(message)
	switch {
	case strings.Contains(lower, "shutdown"):
		return CancelReasonInput{Message: message, TerminalReason: TerminalReasonShutdown, CancelReason: CancelReasonShutdown}
	case strings.Contains(lower, "issue"):
		return CancelReasonInput{Message: message, TerminalReason: TerminalReasonIssueCancelled, CancelReason: CancelReasonIssue}
	case strings.Contains(lower, "orphan"):
		return CancelReasonInput{Message: message, TerminalReason: TerminalReasonOrphanRecovered, CancelReason: CancelReasonOrphan}
	case strings.Contains(lower, "stale"):
		return CancelReasonInput{Message: message, TerminalReason: TerminalReasonStaleRecovered, CancelReason: CancelReasonStale}
	default:
		return CancelReasonInput{Message: message, TerminalReason: TerminalReasonUserCancelled, CancelReason: CancelReasonUser}
	}
}

func terminalReasonForCancelReason(reason string) string {
	switch reason {
	case CancelReasonShutdown:
		return TerminalReasonShutdown
	case CancelReasonIssue:
		return TerminalReasonIssueCancelled
	case CancelReasonOrphan:
		return TerminalReasonOrphanRecovered
	case CancelReasonStale:
		return TerminalReasonStaleRecovered
	case CancelReasonUser:
		return TerminalReasonUserCancelled
	default:
		return ""
	}
}

func cancelReasonForTerminalReason(reason string) string {
	switch reason {
	case TerminalReasonShutdown:
		return CancelReasonShutdown
	case TerminalReasonIssueCancelled:
		return CancelReasonIssue
	case TerminalReasonOrphanRecovered:
		return CancelReasonOrphan
	case TerminalReasonStaleRecovered:
		return CancelReasonStale
	case TerminalReasonUserCancelled:
		return CancelReasonUser
	default:
		return ""
	}
}

func defaultCancelMessage(reason string) string {
	switch reason {
	case CancelReasonShutdown:
		return "shutdown"
	case CancelReasonIssue:
		return "issue cancelled"
	case CancelReasonOrphan:
		return "orphan recovered"
	case CancelReasonStale:
		return "stale recovered"
	default:
		return "user cancelled"
	}
}

func cancelComment(reason CancelReasonInput) string {
	switch reason.CancelReason {
	case CancelReasonShutdown:
		return "서버 종료로 실행이 취소되었습니다"
	case CancelReasonIssue:
		return "이슈 취소로 실행이 취소되었습니다"
	case CancelReasonOrphan:
		return "재시작 중 진행 작업이 취소되었습니다 (orphan recovered)"
	case CancelReasonStale:
		return "오래된 진행 작업이 취소되었습니다 (stale recovered)"
	default:
		return "사용자가 실행을 취소했습니다"
	}
}

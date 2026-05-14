const triggerLabels: Record<string, string> = {
  issue_created: '이슈 생성',
  rerun: '재실행',
  mention: '멘션',
  autopilot: '오토파일럿'
};

const terminalReasonLabels: Record<string, string> = {
  completed: '정상 종료',
  exit_nonzero: '비정상 종료',
  timeout: '시간 초과',
  executor_error: '실행기 오류',
  worker_panic: '워커 패닉',
  claim_preparation_failed: '실행 준비 실패',
  unknown_failure: '알 수 없는 실패',
  user_cancelled: '사용자 취소',
  issue_cancelled: '이슈 취소',
  shutdown: '서버 종료',
  orphan_recovered: '고아 실행 복구',
  stale_recovered: '오래된 실행 복구'
};

const failureKindLabels: Record<string, string> = {
  exit_nonzero: 'exit non-zero',
  timeout: 'timeout',
  executor_error: 'executor error',
  worker_panic: 'worker panic',
  claim_preparation_failed: 'prepare failed',
  unknown: 'unknown failure'
};

const cancelReasonLabels: Record<string, string> = {
  user: '사용자 요청',
  issue: '이슈 취소',
  shutdown: '종료 처리',
  orphan: 'orphan 회수',
  stale: 'stale 회수'
};

const runEventLabels: Record<string, string> = {
  run_queued: '큐 등록',
  run_claimed: '워커 할당',
  executor_starting: '실행 시작',
  stdout_truncated: 'stdout 절단',
  cancel_requested: '취소 요청',
  run_cancelled: '취소 완료',
  run_completed: '완료',
  run_failed: '실패',
  run_prepare_failed: '준비 실패',
  orphan_recovered: 'orphan 복구',
  stale_recovered: 'stale 복구'
};

function getMappedLabel(labels: Record<string, string>, value: string) {
  return labels[value] ?? value;
}

export function getTriggerLabel(trigger: string) {
  return getMappedLabel(triggerLabels, trigger);
}

export function getTerminalReasonLabel(value: string) {
  return getMappedLabel(terminalReasonLabels, value);
}

export function getFailureKindLabel(value: string) {
  return getMappedLabel(failureKindLabels, value);
}

export function getCancelReasonLabel(value: string) {
  return getMappedLabel(cancelReasonLabels, value);
}

export function getRunEventLabel(value: string) {
  return getMappedLabel(runEventLabels, value);
}

// Text for the cells of audit rows, which the audit log viewer and the
// review queue share.

import type { AuditEntry } from '../gen/wathena/admin/v1/admin_pb';

/** An audit row's arguments as key=value pairs. */
export function argumentsText(entry: AuditEntry): string {
  const fields = entry.arguments ?? {};
  return Object.keys(fields)
    .sort()
    .map((k) => `${k}=${JSON.stringify(fields[k])}`)
    .join(' ');
}

/** The review cell of an audit row. */
export function reviewText(entry: AuditEntry): string {
  if (entry.reviewedAt !== undefined) {
    let text = `Reviewed by ${entry.reviewedBy}`;
    if (entry.verdict !== '') {
      text += `: ${entry.verdict}`;
    }
    if (entry.reviewNote !== '') {
      text += `, ${entry.reviewNote}`;
    }
    return text;
  }
  return entry.needsReview ? 'Needs review' : '';
}

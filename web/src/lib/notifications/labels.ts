import type { ChannelKind, EventType, Severity } from '#lib/api/notify.js';
import { m } from '#lib/paraglide/messages.js';

const kindLabels: Record<ChannelKind, () => string> = {
  discord: m.channel_kind_discord,
  slack: m.channel_kind_slack,
  ntfy: m.channel_kind_ntfy,
  webhook: m.channel_kind_webhook,
  email: m.channel_kind_email
};

const eventLabels: Record<EventType, () => string> = {
  maintenance_due: m.event_maintenance_due,
  tests_due: m.event_tests_due,
  out_of_range: m.event_out_of_range,
  backup_failed: m.event_backup_failed
};

const severityLabels: Record<Severity, () => string> = {
  info: m.severity_info,
  warning: m.severity_warning,
  critical: m.severity_critical
};

export const kindLabel = (kind: ChannelKind): string => kindLabels[kind]();
export const eventLabel = (type: EventType): string => eventLabels[type]();
export const severityLabel = (severity: Severity): string => severityLabels[severity]();

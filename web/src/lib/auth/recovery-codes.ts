import { todayIn } from '#lib/format/local-time.js';
import { m } from '#lib/paraglide/messages.js';

export const recoveryCodesFilename = 'reefshepherd-recovery-codes.txt';

export interface RecoveryFileInfo {
  username: string;
  instanceName: string;
  createdAt: Date;
  /** The IANA timezone the creation date is written in. */
  timeZone: string;
}

/** The text of the downloadable recovery code file: a short header, then one code per line. */
export function recoveryCodesText(codes: readonly string[], info: RecoveryFileInfo): string {
  const lines = [
    m.recovery_file_title({ app: m.app_name() }),
    '',
    m.recovery_file_account({ username: info.username }),
    m.recovery_file_instance({ name: info.instanceName }),
    m.recovery_file_created({ date: todayIn(info.timeZone, info.createdAt) }),
    '',
    m.recovery_file_note(),
    '',
    ...codes
  ];
  return lines.join('\n') + '\n';
}

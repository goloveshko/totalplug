import { BackupLocation, GetBackupPolicy, SetBackupPolicy } from '../wailsjs/go/gui/App';
import { get, writable } from 'svelte/store';

// Settings > Backups. How long a pre-update copy of a plugin is kept is stored in
// the settings file, because a command-line update honours the same answer, so
// this store holds that file's numbers instead of a copy kept here.

// `error` is a rejected edit: while it is set, the two fields keep the user's own
// numbers, because replacing them would hide the value they have to fix.
export const backupState = writable({ policy: null, location: '', error: '', keep: '', days: '' });

const patchBackup = (patch) => backupState.update((state) => ({ ...state, ...patch }));

// Both numbers are one answer, so the fields that show them are written together.
const fieldsOf = (policy) => ({ policy, keep: String(policy.keep_newest), days: String(policy.max_age_days) });

export async function loadBackupSettings() {
    try {
        const [stored, where] = await Promise.all([GetBackupPolicy(), BackupLocation()]);
        patchBackup({
            ...fieldsOf({ keep_newest: stored?.keep_newest ?? 0, max_age_days: stored?.max_age_days ?? 0 }),
            location: where ?? '',
            error: '',
        });
    } catch (err) {
        patchBackup({ error: String(err?.message ?? err) });
    }
}

// Both numbers travel together: a policy is one answer, and storing half of it
// would leave the other half from an edit the user can no longer see.
export async function applyBackupPolicy() {
    const { policy, keep: keepText, days: daysText } = get(backupState);
    const keep = Number(keepText);
    const days = Number(daysText);
    if (!Number.isInteger(keep) || !Number.isInteger(days)) {
        // Neither a number nor a blank can be stored, so the fields go back to
        // what is actually in force.
        if (policy) patchBackup({ ...fieldsOf(policy), error: '' });
        return;
    }

    const next = { keep_newest: Math.max(0, keep), max_age_days: Math.max(0, days) };
    // Retyping the policy that is already stored is not an edit, but it does
    // retire a rejected one and normalise what the fields show.
    if (policy && next.keep_newest === policy.keep_newest && next.max_age_days === policy.max_age_days) {
        patchBackup({ ...fieldsOf(policy), error: '' });
        return;
    }
    patchBackup({ error: '' });
    try {
        await SetBackupPolicy(next);
    } catch (err) {
        patchBackup({ error: String(err?.message ?? err) });
        return;
    }
    patchBackup(fieldsOf(next));
}

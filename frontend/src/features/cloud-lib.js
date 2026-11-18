// The pure part of cloud placeholder marks (cloud.js, issue #30).
import { F } from '../core/format.js';

// isCloudOnly says whether a panel entry is an online-only cloud placeholder (its contents are not downloaded).
export const isCloudOnly = (e) => !!e && !e.up && !!(e.flags & F.CLOUD);

// cloudRowClass is the row class that draws the mark: only when the option is on.
export const cloudRowClass = (e, enabled) => (enabled && isCloudOnly(e) ? 'mc-cloud' : '');

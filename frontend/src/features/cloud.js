// Cloud placeholder awareness (issue #30, F-873; Go: feature_cloud.go, internal/cloudfile). Go marks online-only
// iCloud Drive / OneDrive files with the entry flag F.CLOUD, read from the system's own stat data (macOS SF_DATALESS,
// Windows recall attributes), so knowing it never downloads anything. With Options > Display > "Mark online-only cloud
// files" (Display.CloudMark, off by default) such rows get a small cloud mark after the name. Custom columns can show
// [=mc.cloud] anyway, and Operation.CloudNoDownload keeps searches, thumbnails and media columns from reading them.

import './cloud.css';
import { Panel } from '../core/panel.js';
import { on } from './config/util.js';
import { cloudRowClass } from './cloud-lib.js';

Panel.rowClassHooks.push((p, e) => cloudRowClass(e, on('Display.CloudMark')));

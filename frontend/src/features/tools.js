// The Files-menu tools (FEATURES.md sections 12, 34, 35 and the link items of 12/42), over the Go engines
// internal/attrs, internal/dirsize, internal/fileutil and internal/links (feature_tools*.go, window.go.main.Tools):
//
//   tools/attrs.js      Change attributes (cm_SetAttrib)
//   tools/props.js      Properties (cm_Properties, Alt+Enter, Cmd+I)
//   tools/space.js      Space on a folder, Alt+Shift+Enter (cm_CountDirContent), Ctrl+L (cm_GetFileSpace)
//   tools/checksums.js  cm_CRCCreate, cm_CRCCheck (and Enter on a checksum file), cm_ChecksumToClip (ours)
//   tools/split.js      cm_FileSplit, cm_FileCombine, cm_UUEncode, cm_UUDecode
//   tools/links.js      cm_CreateSymbolicLink, cm_CreateHardLink, Ctrl+Shift+F5 (cm_CreateShortcut)
//   tools/xattr.js      Remove quarantine flag (cm_RemoveQuarantine, ours, macOS)
//
// Their keys and menu items are already in core/keymap.js and core/menus.js; every dialog is a core/dialogs.js dialog,
// so control mode can read and answer it (kinds: attributes, properties, tools-space, crc-create, crc-verify, crc-clip,
// split, combine, encode, decode, create-link, create-links, tools-progress, tools-overwrite, tools-error).

import './tools.css';
import './tools/attrs.js';
import './tools/props.js';
import { init as initSpace } from './tools/space.js';
import { init as initChecksums } from './tools/checksums.js';
import './tools/split.js';
import './tools/links.js';
import './tools/xattr.js';

export function init(app) {
  initSpace(app);
  initChecksums(app);
}

# Ship exact TC keys where every rival slips

mormiCommander has a real opening: no tool today is at once free, open source, cross-platform, polished on macOS and faithful to Total Commander's keys. Double Commander (DC) is the only mature free commander on Windows, macOS and Linux. It is very active (v1.2.11 on 2026-11-08, about 1,200 commits in 12 months), but two people write about 97% of the code, it looks like a Windows app on the Mac, and its Homebrew cask has been **disabled since 2026-09-01 because it fails Gatekeeper** ([cask JSON](https://formulae.brew.sh/api/cask/double-commander.json)). The polished Mac options (ForkLift, Commander One, Path Finder, QSpace, Cmdr) are paid, Mac-only, and lose TC users on specific missing features: sync dirs, compare, multi-rename and keyboard selection. What switchers miss is not one feature but muscle memory. Every clone is "close but not the same", and the details that give it away are typing into the command line, Ins/Space selection, Alt+F7 and Ctrl+Q ([HN 43203348](https://news.ycombinator.com/item?id=43203348), [HN 47546477](https://news.ycombinator.com/item?id=47546477)). So the order of work is: key-level fidelity first, then the four switch-blockers (sync dirs, multi-rename, archives you can edit inside, Lister), signed and notarized builds, and a few cheap extras TC refuses or other apps charge for (undo, free cloud panels via rclone). Two limits on the evidence: Reddit could not be reached in any research pass, so sentiment leans on Hacker News, the ghisler.ch forum and GitHub trackers, and nothing here comes from hands-on testing of competitors' keys.

## What to do first

1. **Make the TC keyboard layer perfect before adding breadth.** That means §8 hotkeys, §10 "typing goes to the command line", §6 Ins/Space selection and §3 quick search and quick filter. Switchers judge the app by whether muscle memory works.
2. **Sign and notarize the macOS builds as universal binaries, and sign the Windows builds.** This alone beats DC on the Mac now that its cask is disabled. Do it for mormiView and mormiDiskFree in the same pipeline.
3. **Ship the four switch-blockers:** sync dirs (§25), multi-rename with preview and undo (§22), archives as folders including edit-and-repack (§29, F-592), and Lister/quick view (§27).
4. **Turn two N/A items into features:** F-489 undo and F-645 cloud/S3/WebDAV. Undo is something TC refuses on principle. rclone is a Go library, so a free cloud panel is cheap for you and costs money in Commander One.
5. **Launch with an honest README** ("inspired by Total Commander, not affiliated", a key table, a "vs Double Commander" table, a no-telemetry statement anyone can check). Then post to Show HN and r/macapps, using the Mormitech tap, winget, Scoop and .deb on day one.

## Only Double Commander spans three OSes, and it just lost Homebrew

The field splits into five groups. On Windows, the paid tier is healthy and Windows-only: TC 11.58.1 (29 Oct 2026, €42), Directory Opus 13 (updates need a subscription, which caused a backlash), XYplorer (about $70–80) and One Commander (free for home use, $30 Pro) ([ghisler.com](https://www.ghisler.com/order.htm), [gpsoft](https://www.gpsoft.com.au/), [onecommander.com](https://onecommander.com/)). Ghisler's answer to a Mac or Linux version has always been "no", and the Linux port was cancelled ([TC wiki](https://www.ghisler.ch/wiki/index.php/Total_Commander_for_Linux)). On macOS, paid native apps are actively maintained: ForkLift 4.7.7 (Oct 2026), Commander One 3.18 (Sep 2026) and Path Finder 26 (May 2026, subscription only). The free options are mostly stale (Marta has had no release since July 2024) or single-maintainer (Nimble Commander, GPLv3 since Dec 2024, about one feature release a year) ([binarynights](https://binarynights.com/), [Eltima](https://mac.eltima.com/commander-one-purchase.html), [TidBITS](https://talk.tidbits.com/t/path-finder-26-0/33442), [marta.sh](https://marta.sh/changelog/), [Nimble releases](https://github.com/mikekazakov/nimble-commander/releases)). A 2026 wave of small Mac newcomers (Cmdr, WhimFiles, EmpiricCommander, mq-dir, Captain's Deck, Ferry) shows the niche is crowded but has no leader. On Linux, Krusader has not released since 2.9.0 (Dec 2024), Sunflower just shipped 0.6-64 after four years, and Tux Commander is dead ([KDE announce](https://mail.kde.org/pipermail/kde-announce-apps/2025-January/005841.html), [Sunflower](https://github.com/MeanEYE/Sunflower/releases/tag/0.6-64)). In the terminal, mc and far2l carry the orthodox layout, while the momentum is with vim-style TUIs: yazi has 43k stars, superfile 24k (written in Go) and nnn 22k ([yazi](https://github.com/sxyazi/yazi), [superfile](https://github.com/yorukot/superfile)).

| App | Platforms | Price | Stack | TC keys | Standout | Main weakness | Activity (Nov 2026) | Sentiment |
|---|---|---|---|---|---|---|---|---|
| **Double Commander** | Win/Mac/Linux | Free, GPL-2 | Pascal/Lazarus (Cocoa, GTK2, Qt5/6) | High; F2–F8 bound | TC plugin API (WCX/WDX/WFX/WLX) | Non-native on Mac; Mac/Wayland bugs; Homebrew cask disabled | v1.2.11, very active, bus factor about 2 | "Best option on macOS" (4.7/5) but "off" ([SourceForge](https://sourceforge.net/projects/doublecmd/reviews/)) |
| File Commander | Win (primary)/Mac/Linux | Free, Apache-2 | C++/Qt 6 | Orthodox | Own plugins, TUI | 459 stars, 1.0 prerelease | Active | Little discussion |
| muCommander | Win/Mac/Linux | Free | Java | Partial | Mature | 1.5.2 (Oct 2024); 1.6 unconfirmed | Slow | Rarely discussed |
| Total Commander | Windows (+Android) | €42 | Pascal (Delphi/Lazarus) | The reference | Plugins, MRT, sync, Lister | No undo; Windows-only | 11.58.1, active | Loved; "the only software I still miss" ([HN 45272281](https://news.ycombinator.com/item?id=45272281)) |
| Directory Opus | Windows | Perpetual + paid updates | Native C++ | No (own) | Scripting, layouts, Everything | Learning curve, subscription | 13.x, active | "King of the dual panes" |
| XYplorer / One Commander / Files | Windows | $70–80 / free+$30 / free+Store | Native / .NET / C# WinUI | No | Scripting / Miller columns / modern UI | Windows-only | Active | Positive |
| Commander One | macOS | Free + PRO $29.99 | Native | Partial (Tab, F5, F6, F8) | Cloud mounting, root mode (PRO) | No sync, compare or multi-rename; paywall | 3.18, active | "Constantly annoyed by missing features" ([ghisler.ch](https://www.ghisler.ch/board/viewtopic.php?p=408106)) |
| ForkLift 4 | macOS | About $20–30 (sources conflict) | Native | Disputed preset; refused Ins/Space selection | SFTP/S3/cloud, sync, Git | Two panes max; licensing churn | 4.7.7, very active | Liked for polish |
| Nimble Commander | macOS | Free, GPL-3 | C++/AppKit | Good F-keys | Speed, viewer, type-to-filter | Single maintainer; won't take feature requests | v1.9.0, yearly | Most praised by TC users on HN ([HN 38360881](https://news.ycombinator.com/item?id=38360881)) |
| Marta | macOS | Free (beta) | Native, Lua API | No | Lua plugins, home-row keys | Stale since Jul 2024 | Dormant | Fond but frustrated |
| Path Finder / QSpace | macOS | $29.95/yr / $12.99+IAP | Native | No | Finder-replacement depth / 12 layouts | Subscription resented / Mac-only | Active | Mixed |
| Cmdr | macOS (Linux soon) | Free personal, $59 work, BSL | Rust | F3–F8 | Brief view, MCP, local AI | Not OSI open source | v0.60, active | Vendor marketing only |
| Krusader | Linux (KDE) | Free, GPL | Qt6/KF6 | Partial | Powerful, KIO | Pulls in KDE; no release in 22 months | Stale | Recommended by Ghisler for Linux |
| Sunflower | Linux (GNOME) | Free, GPL-3 | Python/GTK | F-key bar | Wayland-native | Small | 0.6-64 (Oct 2026) | Little discussion |
| mc / far2l | Linux/Mac/BSD terminal | Free | C / C++ | Norton/Far keys | VFS over SSH, panelize | Terminal only; far2l still beta | Active | mc: 525 HN points ([HN](https://news.ycombinator.com/item?id=45271481)) |
| yazi / superfile / nnn | Terminal, all OSes | Free | Rust / Go / C | No (vim) | Async previews, plugins | Fonts, discoverability | Very active | Large audiences |

Three conclusions follow from the table. First, **the only free competitor on all three platforms is fragile on the Mac**. DC has 97 open macOS-tagged issues, a Sept 2026 setup guide that had to remap Cmd-A and restyle the UI to make it usable, a Mac toolbar that forgets customisation, and a launch crash on macOS 27 ([issue search](https://github.com/doublecmd/doublecmd/issues?q=is%3Aissue+is%3Aopen+macos+OR+cocoa+OR+mac), [Medium](https://medium.com/@roughterr/my-double-commander-setup-on-macos-1bfc172e322e), [#3137](https://github.com/doublecmd/doublecmd/issues/3137), [#3186](https://github.com/doublecmd/doublecmd/issues/3186)). Second, **DC's real moat is the TC plugin API, and you should not try to match it**. Windows TC DLLs only load on Windows, WLX viewer plugins expect a native window handle that a Wails webview does not have, and users repeatedly say they run TC without plugins ([DC wiki](https://github.com/doublecmd/doublecmd/wiki/Plugins), [ghisler.ch](https://www.ghisler.ch/board/viewtopic.php?p=438190)). Third, **mormiCommander would be the only webview-based commander**. Mac power users check for "no Electron" and small binaries ([HN 48814952](https://news.ycombinator.com/item?id=48814952)), and Wails on Linux depends on WebKitGTK, which Sigma's CEF experiment suggests is painful. Measure startup time and binary size before you claim "small".

## Switchers judge by muscle memory, then by four heavy tools

The emotional pull is strong and well documented. Users describe going back to TC as "my arm is complete again", and list TC (with WinRAR) as what they miss most after moving to macOS ([HN 39079200](https://news.ycombinator.com/item?id=39079200), [HN 44086169](https://news.ycombinator.com/item?id=44086169)). The complaints are consistent. Under CrossOver, TC freezes, looks "hideous" and has an in-window menu bar. Every Mac clone "falls short by a big margin" on synchronize and compare ([ghisler.ch 2021](https://www.ghisler.ch/board/viewtopic.php?p=408106)). In DC, typing `cd ..` starts a quick search instead of going to the command line ([HN 37348653](https://news.ycombinator.com/item?id=37348653)). Commander One and ForkLift lose TC users on missing sync, compare and multi-rename, and on ForkLift's explicit refusal to add keyboard selection, not on price ([BinaryNights beta 9](https://blog.binarynights.com/2023/08/22/forklift-4-beta-9-is-available/)).

The ranking below merges HN anecdotes, ghisler.ch threads, and tracker reactions on Marta, DC and Nimble. Daily items decide whether someone keeps the app open. Occasional items decide whether they can uninstall TC under Wine.

| Rank | What they miss | Use | Evidence | FEATURES.md |
|---|---|---|---|---|
| 1 | Exact TC keys (F3–F8, Tab, Alt+F1/F2, Ctrl+arrows), TC preset on first run | Daily | Strong: HN, Marta #697 key presets, ForkLift refusal | §8, §9 |
| 2 | Keyboard selection (Ins/Space, Num+/−) | Daily | Strong | §6 |
| 3 | Typing goes to the command line | Daily for CLI users | Medium–strong: HN 37348653, 43203348, Nimble #381 | §10 |
| 4 | Tabs, locked tabs | Daily | Medium–strong | §2 |
| 5 | Archives as folders, edit a file and save it back into the archive | Daily for many | Strong ([HN 37348285](https://news.ycombinator.com/item?id=37348285)) | §29 |
| 6 | Quick search and type-to-filter | Daily | Medium | §3 |
| 7 | Lister (F3) and quick view (Ctrl+Q) | Daily | Medium | §27 |
| 8 | Native feel: menu bar, dark mode, speed | Daily (ambient) | Strong: Marta #626 (18 reactions), CrossOver complaints | §1, §42 |
| 9 | Sync dirs, including against archives and FTP | Occasional, but it blocks the switch | Strong: ghisler, Marta #48, Nimble #17 | §25 |
| 10 | Multi-rename with masks, counters, regex | Weekly | Strong (100 files in 10 seconds vs 10 minutes, [HN 37348475](https://news.ycombinator.com/item?id=37348475)) | §22 |
| 11 | Compare by content | Occasional | Medium | §26 |
| 12 | SFTP/FTP/SMB in a panel | Occasional (daily for ops users) | Medium | §30 |
| 13 | Background queue with pause | Occasional | Medium | §20 |
| 14 | Hotlist and history | Daily for some, unused by others | Medium | §31 |
| 15 | Undo | Occasional safety net | Medium: Marta #705, #230 | §20 (F-489 is N/A) |
| 16 | Mac specifics: fn keys, notarized builds, Full Disk Access, iCloud/OneDrive placeholders, Finder tags, locale sorting | One-time to daily | Medium: DC #3066, #2675, #3146, #3117 | §9, §38 |
| 17 | Linux specifics: Wayland keys and drag-and-drop, Flatpak, trash restore | Daily if broken | Medium: DC #2955, #2452, #309, #1645 | §36, §21 |
| 18 | TC plugin compatibility | Rare, niche | Weak–medium | — |
| 19 | Aliases, start menu, large button bars | Rare | Weak (2010 survey: "never use them") | §11, §18 |

Two Mac conflicts need a decision, not a guess. MacBook F-keys need fn by default, and Mac users expect Space to open Quick Look while TC users expect Space to select ([fman](https://fman.io/blog/fman-is-out-on-os-x/), [HN 37349437](https://news.ycombinator.com/item?id=37349437)). FEATURES.md already handles the first in F-287 and the selection side in F-797. What it lacks is a first-run choice and a native Quick Look key (both are in the ranked list below).

## Distribute signed builds, lead with fidelity and no telemetry

Distribution has changed since DC built its audience. Since 2026-09-01 the official homebrew/cask repo disables casks that fail Gatekeeper, and `--no-quarantine` is being removed. Unsigned apps can still live in your own tap, but users will hit the Gatekeeper prompt ([Homebrew discussion #6334](https://github.com/orgs/Homebrew/discussions/6334), [Acceptable Casks](https://docs.brew.sh/Acceptable-Casks)). Getting into the official cask also needs **225 stars (or 90 forks or watchers) for a self-submission, or 75 stars if a third party submits it**, and a repo at least 30 days old ([acceptance policy](https://docs.brew.sh/Package-Acceptance-Policy)). Rosetta-only casks are ineligible once macOS 27 is current, so ship universal builds. The Mac App Store is not an option, because its sandbox cannot grant Full Disk Access and Commander One's App Store build is cut down for that reason ([Apple forums](https://developer.apple.com/forums/thread/124895), [Eltima](https://mac.eltima.com/mas-vs-site-version-differences/)). On Windows, Scoop's main bucket excludes GUI apps (so use Extras or your own bucket). winget accepts unsigned installers, but SmartScreen and Smart App Control punish them, and SignPath offers free signing for open-source projects ([Scoop criteria](https://github.com/ScoopInstaller/Scoop/wiki/Criteria-for-including-apps-in-the-main-bucket), [SmartScreen](https://learn.microsoft.com/windows/apps/package-and-deploy/smartscreen-reputation)). On Linux, Wails v2 needs `-tags webkit2_41` on Ubuntu 24.04 and newer, which F-761 already covers ([Wails docs](https://wails.io/docs/gettingstarted/building)). Flathub says file managers are "generally not well-suited" and accepts them mainly when upstream submits them. DC's top-reacted open issue is a Flatpak request, and no orthodox commander is on Flathub, so it is a real but later opportunity ([Flathub requirements](https://docs.flathub.org/docs/for-app-authors/requirements), [DC #309](https://github.com/doublecmd/doublecmd/issues/309)).

On positioning, four messages hold up against the evidence. **"Your TC muscle memory on macOS and Linux"** is a gap no competitor fills cross-platform. **"Free, open source, no telemetry, no account, no subscription"** lands because Path Finder's 2022 subscription switch and ForkLift's licensing changes drew backlash, QSpace was criticised on MacPowerUsers for phoning home, and superfile was criticised when its theme download contacted Microsoft servers ([Michael Tsai](https://mjtsai.com/blog/2022/02/23/path-finder-subscriptions), [MacPowerUsers](https://talk.macpowerusers.com/t/forklift-4-by-binary-nights-finder-alternative/34693?page=3), [HN 40318714](https://news.ycombinator.com/item?id=40318714)). Make the no-telemetry claim testable (see F-NEW-3). **"Same app on all three OSes"** is the wedge for people with a Windows work PC and a Mac at home. **The Mormitech family** (mormiView, mormiDiskFree, mormiCommander from the same tap, bucket and apt repo) is worth stating, because it shows the developer ships and maintains apps. Acknowledge DC up front. HN will ask "why not Double Commander", and the honest answer is Mac integration, signed builds and key fidelity, not plugins. Do not claim plugin compatibility.

On naming, "mormiCommander" is safe because "Commander" is generic in this category (Norton, Midnight, Double, One). "Total Commander" is a registered trademark, and Ghisler already had to rename Windows Commander after Microsoft objected in 2002 ([ghisler wiki](https://www.ghisler.ch/wiki/index.php/What_happened_to_Windows_Commander%3F)). He reportedly tolerates functional clones like DC, but his forum objects to name variants ([ghisler.ch](https://ghisler.ch/board/viewtopic.php?p=297375)). Use "a free dual-pane file manager inspired by Total Commander. Not affiliated with Ghisler Software GmbH." Keep "TC" out of the name, icon, domain and colour scheme, and don't reuse TC assets. "Total Commander clone" is fine as a description in body text, not as a tagline. One more caution: repos imitating DC have shipped malware on macOS, and fake "Total Commander unlocked" repos exist ([DC #2858](https://github.com/doublecmd/doublecmd/issues/2858)). Publish one canonical download page with checksums.

### Distribution checklist

1. **P0. Sign and notarize macOS universal builds** with a Developer ID (about $99/yr; check the current price). Apply it to all three Mormitech apps. Use hardened runtime and notarytool in CI.
2. **P0. Sign Windows binaries** with SignPath (free for OSS) or Azure Trusted Signing. Don't UPX-pack Go binaries, because packing invites Defender false positives.
3. **P0. Build Linux with `-tags webkit2_41`** and declare `libwebkit2gtk-4.1-0` in the .deb `Depends:`.
4. **P0. Write the README and homepage:** not-affiliated line, key table, "vs Double Commander / Commander One / ForkLift / Nimble" table, no-telemetry statement with a pointer to the test, GIF of the keys, canonical download URL and checksums.
5. **P1. Homebrew:** the Mormitech tap now. homebrew/cask once the repo is 30+ days old and has 225 stars, or let a user submit it at 75.
6. **P1. winget** with `Microsoft.EdgeWebView2Runtime` as a dependency. **Scoop:** the Mormitech bucket now, then a PR to Extras.
7. **P1. Signed self-hosted apt repo** (gives users updates through apt), plus AppImage, .rpm and AUR `mormicommander-bin`.
8. **P2. Show HN** on a weekday US morning, with you answering in the comments. Lead with keys and the DC comparison.
9. **P2. r/macapps first** (the free-TC-for-Mac gap is sharpest there), then r/linux, r/opensource and r/commandline. Hungarian launch on hup.hu and prohardver.
10. **P2. ghisler.ch forum and r/totalcommander,** only in "other programs" areas and only if the rules allow it.
11. **P3. AlternativeTo listing** as an alternative to TC, DC, Commander One, ForkLift and Marta (this is where "TC for Mac" searches land). Awesome-lists (awesome-go, awesome-wails, open-source-mac-os-apps). LinuxLinks, MacUpdate, Softpedia.
12. **P3, later. Flathub,** submitted by you, with a written justification for filesystem access. Skip the Mac App Store and strict Snap. Product Hunt and Chocolatey are optional.

## Go and the webview make several gaps cheap to close

The beyond-parity ideas that matter use what the stack is good at. **rclone's Go packages can be imported directly** (`fs.NewFs("remote:path")`). That gives you dozens of backends, reuses the user's existing `rclone.conf`, and includes server-side copy and checksums. Meanwhile Commander One puts cloud access behind payment, ForkLift has a fixed provider list, and DC and TC rely on stale or 32-bit WFX plugins ([rclone fs](https://pkg.go.dev/github.com/rclone/rclone/fs@v1.54.1), [cloud_storage plugin](https://awesome.ecosyste.ms/projects/github.com%2Fivanenko%2Fcloud_storage), [S3 Browser thread](https://www.ghisler.ch/board/viewtopic.php?p=447603)). yazi shipped remote file support in Dec 2025, and its users immediately asked for an rclone backend ([yazi #611](https://github.com/sxyazi/yazi/issues/611)). Import a curated subset of backends so the binary stays small. **Multi-level undo** is a long-standing request on Marta (#705, #230), and TC's author has refused it as giving "a false feeling of security" ([HN 37348567](https://news.ycombinator.com/item?id=37348567)). An append-only journal of inverse operations, with deletes going to trash, makes undo a safety feature and a selling point. The webview makes **rich previews** cheap: PDF, Markdown, code, video. HEIC, AVIF and JPEG XL can be decoded in Go without cgo through WASM builds of the codecs ([gen2brain/avif](https://github.com/gen2brain/avif), [bradfitz/heic](https://pkg.go.dev/github.com/bradfitz/heic)). **Table previews** (CSV, Parquet, SQLite, JSONL) are something no commander offers. A **Git status column** is standard in TUI plugins and IDEs but missing from TC, DC, Marta and ForkLift as far as the research found ([yazi-rs/plugins](https://github.com/yazi-rs/plugins)). A **folder-size treemap** fits Go's concurrency, and you already ship mormiDiskFree, so reusing its engine or handing off to it costs little.

Other projects' trackers suggest what users want next. In Files (files-community), a tree view has 169 reactions, search filters 90, a custom copy engine 39, SFTP 21, a duplicate finder 14 and an embedded terminal 13 ([Files issues](https://github.com/files-community/Files/issues?q=is%3Aissue+is%3Aopen+sort%3Areactions-%2B1-desc)). FEATURES.md already covers tree, search, SFTP and duplicates as parity items. The terminal and a command palette are new. AI needs care. Windows 11's AI actions in File Explorer drew a largely negative reaction from technical users, and Microsoft reportedly scaled back Copilot hooks in 2026 ([LaptopMag](https://www.laptopmag.com/ai/ai-actions-file-explorer), [HotHardware](https://hothardware.com/news/windows-11-pulls-back-copilot-features)). The safe version is opt-in and local-only (Ollama or llama.cpp on localhost). It should produce a rename *plan* (a regex or template) shown in the normal MRT preview and covered by undo, never per-file names from the cloud. Snapshot browsing (ZFS/Btrfs, APFS, VSS) and an SSH helper agent are unique, but they are large and belong after 1.0.

## Conclusion

Price is not what's open in this market; DC and Nimble are already free. What's open is trust and fidelity. DC's loss of its Homebrew cask, its two-person bus factor and its Mac rough edges mean a TC user on macOS has no free tool they can recommend without caveats. Faithful keys, signed builds and a working sync-dirs tool would make mormiCommander that tool sooner than any new feature would. Distribution shapes the product too. Signing, notarization and a verifiable no-network test are part of the pitch, so they belong in the backlog alongside features, not after it.

A risk follows from the same logic. Small mistakes in the key layer (a letter that triggers quick search instead of the command line, Space that doesn't select) cost more than missing features, because they are exactly what made users call DC "off". The control-mode test infrastructure in §40 is the right tool here: script the TC key reference end to end and treat a failure as a release blocker.

## Demand outside Reddit: where the numbers point

Reddit could not be reached; it is blocked for the research tools. So this section uses Hacker News, Lemmy and the wider fediverse, Stack Exchange, GitHub issue trackers, AlternativeTo, Homebrew analytics, Debian popcon, Chocolatey, Setapp and SourceForge. All numbers were collected on 2026-11-13. Thread counts are hand-classified search hits, so treat them as rough. Stack Exchange views are lifetime totals, so they favour questions asked in 2010–2012.

### Audience size

| App | Homebrew 365d | Debian popcon | Chocolatey | AlternativeTo likes |
|---|---|---|---|---|
| Marta (Mac) | 7,285 | – | – | 15 (listed as discontinued) |
| ForkLift (Mac) | 4,452 | – | – | 106 |
| Commander One (Mac) | 3,717 | – | – | 73 |
| Double Commander | 2,415 | 1,565 (doublecmd-common) | 722,644 | 626 |
| Nimble Commander (Mac) | 1,971 | – | – | – |
| muCommander | 1,287 | – | – | 128 |
| QSpace Pro (Mac) | 1,235 | – | – | – |
| Path Finder (Mac) | 1,034 | – | – | 56 |
| Krusader (Linux) | – | 1,713 | – | 90 |
| Total Commander (Win) | – | – | 929,265 | reference |
| Midnight Commander (TUI) | 21,270 | 51,632 | – | 248 |
| yazi (TUI) | 100,405 | 637 | – | – |

Sources: [Homebrew cask 365d](https://formulae.brew.sh/api/analytics/cask-install/365d.json), [Homebrew formula 365d](https://formulae.brew.sh/api/analytics/install/365d.json), [Debian popcon](https://popcon.debian.org/by_inst.gz), [Chocolatey TC](https://community.chocolatey.org/packages/totalcommander), [Chocolatey DC](https://community.chocolatey.org/packages/doublecmd), [AlternativeTo Mac list](https://alternativeto.net/software/total-commander/?platform=mac), [AlternativeTo Linux list](https://alternativeto.net/software/total-commander/?platform=linux), [AlternativeTo Marta](https://alternativeto.net/software/marta/about/).

The key fact: **Marta leads Mac installs while dormant.** It has 7,285 cask installs a year, about 31% of the roughly 23,400 for the top eight Mac GUI commanders, and still 494 in the last 30 days ([30d JSON](https://formulae.brew.sh/api/analytics/cask-install/30d.json)). It has had no release since 2024 and has 527 open issues ([tracker](https://github.com/marta-file-manager/marta-issues)). Those users chose a free, native, keyboard-first Mac commander and now have no maintained one. Nimble Commander is the one to watch: 271 installs in 30 days annualises above its 365-day total of 1,971, so it is growing.

Two cautions. The whole Mac GUI segment is about a quarter of yazi alone, so keyboard power users on Mac are drifting to TUIs. DC's 2,415 is mostly from before its cask was disabled on 2026-09-01. On Linux, GUI commanders are small next to mc: Krusader and DC together have about 3,300 popcon installs against 51,632 for mc. Neither is on [Flathub](https://flathub.org/api/v2/appstream).

### The Finder gaps get the most views

Single-feature Finder questions dwarf the "Total Commander for Mac" questions (about 62,000 views in total). A commander already solves each of them.

| Need | Total SE views | Top question | Extra signal | FEATURES.md |
|---|---|---|---|---|
| Cut/move with the keyboard | ~225,000 | [201,622 views](https://apple.stackexchange.com/questions/12391/why-is-it-not-possible-to-use-the-cut-command-to-manipulate-a-file-in-the-find) | [Marta #321](https://github.com/marta-file-manager/marta-issues/issues/321), 11 reactions | F-237, F-256, F-285 |
| Show hidden files | ~178,000 (8 questions) | [86,984 views](https://apple.stackexchange.com/questions/5870/how-to-show-hidden-files-and-folders-in-finder) | [45,784 views](https://apple.stackexchange.com/questions/82218/show-hidden-files-when-searching-in-finder) on hidden files in search | F-090, F-242 |
| Bulk rename | ~149,000 (Linux ~105k, Mac ~44k) | [56,989 views](https://askubuntu.com/questions/511862/an-ubuntu-linux-alternative-of-bulk-rename-utility-for-windows) | [Marta #395](https://github.com/marta-file-manager/marta-issues/issues/395), 8 reactions | §22 |
| Copy path | ~142,000 | [102,316 views](https://apple.stackexchange.com/questions/47216/copying-the-current-directorys-path-to-the-clipboard) | Nimble [#41](https://github.com/mikekazakov/nimble-commander/issues/41), [#571](https://github.com/mikekazakov/nimble-commander/issues/571) (quoted paths) | F-139, F-141, F-142, F-796 |
| Keyboard selection on Mac keyboards | ~106,000 | [93,311 views](https://apple.stackexchange.com/questions/11310/how-can-i-select-files-in-midnight-commander-on-a-macbook-pro) (no Insert key) | Marta [#697](https://github.com/marta-file-manager/marta-issues/issues/697), 15 reactions | F-118, F-119, F-287 |
| Compare/sync folders | ~44,000 | [13,173 views](https://apple.stackexchange.com/questions/194144/osx-how-to-compare-and-merge-two-folders-and-their-subfolders-and-differing-fil) | [DC #2024](https://github.com/doublecmd/doublecmd/issues/2024), 31 comments | §25, §26 |
| Root/admin operations | ~14,000 | [10,134 views](https://apple.stackexchange.com/questions/340414/running-mc-midnight-commander-under-su-on-mojave-catalina-big-sur-monterey-ven) | [Marta #177](https://github.com/marta-file-manager/marta-issues/issues/177), 8 reactions | F-720 (Windows only) |

### Merged demand tally

"Threads" counts distinct 2023–2026 HN and Lemmy threads (about 45 with relevant content) where the feature is named as needed, praised or missing. Q&A and tracker signals come from the table above and the linked issues.

| Feature | HN/Lemmy threads | Q&A and tracker signal | FEATURES.md |
|---|---|---|---|
| Keyboard-first (F-keys, selection, filtering) | 9 | ~106k views; Marta #697 (15), [#305](https://github.com/marta-file-manager/marta-issues/issues/305) (12) | §6, §8, §9 |
| Remote file systems (SFTP/FTP/SMB) | 8 | [DC #1604](https://github.com/doublecmd/doublecmd/issues/1604) 52 comments; [DC #2131](https://github.com/doublecmd/doublecmd/issues/2131) SMB corruption, 31 | §30 |
| Multi-rename | 8 | ~149k views | §22 |
| Compare/sync | 5 | ~44k views; [Marta #48](https://github.com/marta-file-manager/marta-issues/issues/48) (9) | §25, §26 |
| Archives as folders | 4 | [DC #1669](https://github.com/doublecmd/doublecmd/issues/1669) 45 comments | §29 |
| Shell/terminal integration | 4 | [mc Ctrl+O and %macros](https://news.ycombinator.com/item?id=39083459) | §10, F-NEW-15 |
| Network-share hangs | 3 | [DC #2727](https://github.com/doublecmd/doublecmd/issues/2727) NFS refresh, 24 comments | §30, §38 |
| Fast F3 viewer | 3 | [100 MB+ files](https://forums.macrumors.com/threads/file-managers-for-mac.1921711/) | §27 |
| Copy queue, pause, resume | 3 | [HN 49592284](https://news.ycombinator.com/item?id=49592284) | §20 |
| Tabs | 3 | [5,450 views](https://superuser.com/questions/160074/free-dual-pane-file-manager-for-mac-os-x-with-tabs) | §2 |
| Cut/move | 1 | ~225k views | §8, §9 |
| Copy path | 1 | ~142k views | §6 |
| Hidden files | 0 | ~178k views | §4, §8 |
| Undo | 1 | Marta [#705](https://github.com/marta-file-manager/marta-issues/issues/705) (13), [#230](https://github.com/marta-file-manager/marta-issues/issues/230) (7) | F-NEW-2 |
| Folder merge on copy | 1 | [HN 49589960](https://news.ycombinator.com/item?id=49589960) | F-460 |
| Follow dark mode | 0 | Marta [#626](https://github.com/marta-file-manager/marta-issues/issues/626) (18) | §42 |

The two sources see different things. HN and Lemmy users are TC switchers and ask for the heavy tools: rename, sync, remote panels. Stack Exchange shows the much bigger pool of Finder users who hit cut, hidden files and copy path and never search for "commander".

### Most-recommended apps

| App | Threads (HN + fediverse) | Main caveat raised |
|---|---|---|
| Double Commander | ~36 | "Fine, but still a little inferior" ([lemy.nl](https://lemy.nl/comment/6994142)); non-native on Mac ([HN 39079174](https://news.ycombinator.com/item?id=39079174)) |
| Krusader | ~18 (almost all fediverse) | Pulls in KDE ([lemmy.ml](https://lemmy.ml/comment/4140123)) |
| Midnight Commander | ~11 | Dated, key customisation "a total headache" ([HN 47735239](https://news.ycombinator.com/item?id=47735239)) |
| Path Finder | 5 | Subscription-only ([HN 43100460](https://news.ycombinator.com/item?id=43100460)) |
| ForkLift | 4 | Subscription-like licence ([HN 37407943](https://news.ycombinator.com/item?id=37407943)) |
| Commander One | 4 | "Kinda barebones" ([HN 41205287](https://news.ycombinator.com/item?id=41205287)) |
| Marta | 3 | Closed source, data-loss bugs ([HN 39077317](https://news.ycombinator.com/item?id=39077317)) |
| Nimble Commander | 2 | Little known; "almost 1:1" TC UX ([HN 39079174](https://news.ycombinator.com/item?id=39079174)) |

On Linux the answer is settled (DC, then Krusader). On Mac every answer comes with a caveat, and threads still open with "nothing like this for macOS" ([HN 39076987](https://news.ycombinator.com/item?id=39076987), [HN 49590571](https://news.ycombinator.com/item?id=49590571)).

### Recurring complaints per incumbent

- **Double Commander:** Mac crashes ([F3 crash, 45 comments](https://github.com/doublecmd/doublecmd/issues/2815); [Tahoe icons, 43](https://github.com/doublecmd/doublecmd/issues/2519)); ugly default theme ([SourceForge](https://sourceforge.net/projects/doublecmd/reviews/)); freezes for about a minute on an unreachable share ([sh.itjust.works](https://sh.itjust.works/comment/20236888)); installer blocked by SmartScreen and browsers.
- **Krusader:** KDE dependencies on GTK desktops ([Ask Ubuntu](https://askubuntu.com/questions/19459/which-norton-commander-style-filemanager-do-you-recommend)); "needs considerable settings tweaks" ([AlternativeTo](https://alternativeto.net/software/total-commander/?platform=linux)).
- **ForkLift:** Google Drive re-auth on every launch, OneDrive errors, freezes, and a request for per-file error logs with skip-and-continue ([Setapp](https://setapp.com/apps/forklift/customer-reviews)); SMB freezing the system for minutes ([AlternativeTo](https://alternativeto.net/software/forklift/about/)); copies stopping after five files in 4.7.x ([BinaryNights](https://blog.binarynights.com/2026/08/18/forklift-4-7-4-is-available-fixes-for-file-transfers-and-tags/)); panes not refreshing ([HN 43105235](https://news.ycombinator.com/item?id=43105235)).
- **Path Finder:** 8 of the 15 latest Setapp reviews are negative or mixed, mostly crashes and freezes ([Setapp](https://setapp.com/apps/path-finder/customer-reviews)).
- **Commander One:** drag-and-drop, FTP and cloud behind the $29.99 PRO pack; "clunky", non-native UI ([App Store](https://apps.apple.com/us/app/commander-one-file-manager/id1035236694)).
- **Marta:** no release since 2024; open "can lead to data loss" issue ([#989](https://github.com/marta-file-manager/marta-issues/issues/989), 10 reactions); config-file-only settings ([AlternativeTo](https://alternativeto.net/software/marta/about/)).

Reliability beats breadth. The top complaint theme in review samples is crashes and freezes (about 9 mentions), then cloud connector failures (about 6), then ugly or non-native UI (about 5).

### Pricing and privacy

Pricing signals are consistent. ForkLift 4's move to a quasi-subscription disappointed users ([HN 37407943](https://news.ycombinator.com/item?id=37407943)), and one reviewer left Path Finder for DC over its subscription ([SourceForge](https://sourceforge.net/projects/doublecmd/reviews/)). HN users put a fair one-off price at "$10-20" ([HN 43097749](https://news.ycombinator.com/item?id=43097749)) or "25-35 tops" ([HN 43100076](https://news.ycombinator.com/item?id=43100076)). Paid Mac App Store commanders sit between $9.99 and $29.99 ([iTunes API](https://itunes.apple.com/search?term=dual+pane+file+manager&entity=macSoftware&country=us)). TC's perpetual licence earns goodwill ([HN 46080651](https://news.ycombinator.com/item?id=46080651)). Free with no paywall on basics is a real differentiator.

Privacy signals depend on where you look. The review-site pass found **no telemetry complaints** on AlternativeTo, Setapp, SourceForge or the App Store. On HN and Lemmy, trust in closed-source file managers with full disk access came up unprompted. One user asked how to know such an app "won't upload my data in the background" ([HN 41205591](https://news.ycombinator.com/item?id=41205591)), and another struggles to trust closed source with that much access ([HN 41205925](https://news.ycombinator.com/item?id=41205925)). A Lemmy warning that Nautilus makes remote requests when previewing got 212 upvotes ([toast.ooo](https://toast.ooo/post/6873514)). Read honestly: privacy is not why typical reviewers pick or drop an app, but the technical audience that launches open-source tools on HN does check it. That supports F-NEW-3 as a launch message, not as the headline.

### What this changes in the ranking

Changes to the ranked list below:

1. **Add a "Finder-gap trio" at rank 2, just below exact keys.** All three are parity items, all are cheap, and together they carry about 545,000 views: cut/paste with a visible cut state (F-237, F-256, F-285), hidden-files toggle (F-090, F-242), and copy path (F-139, F-141, F-142, F-796). Copy path is already covered. Give F-139 and F-141 default keys rather than leaving them assignable, and add a quoted-path variant to F-142. For cut, extend F-237's acceptance: cut files show dimmed until pasted, and Esc cancels the cut.
2. **Move multi-rename (§22) up from rank 6 to rank 4.** It ties for most-cited feature on HN/Lemmy (8 threads) and has about 149,000 views.
3. **Move SFTP (§30) up from rank 12 to rank 7.** Remote file systems are named in 8 threads, and DC's most-commented issue is its SFTP plugin.
4. **Move cloud via rclone (F-NEW-4) down, after SFTP and the new F-NEW-20.** Cloud connectors are the second-biggest complaint theme for ForkLift and Path Finder. Ship solid SFTP and SMB first.
5. **Keep F-NEW-1 high.** The 93,311-view mc selection question backs its fn check and preset picker. Add Shift+arrows (F-119) to the Mac-native preset so selection works without Insert.
6. **Amend F-460** so copying a folder onto a same-named folder merges, never replaces ([HN 49589960](https://news.ycombinator.com/item?id=49589960)).
7. **Leave root operations at F-720 for now.** About 14,000 views is real, but a macOS privileged helper is L effort. Revisit after 1.0.
8. **Treemap, table preview, Git column, AI rename and snapshots (F-NEW-7, -8, -10, -17, -18)** had no demand signal in this pass. They keep their place as differentiators, but nothing here argues for moving them up.

New items (checked by grep against FEATURES.md; none is covered by an existing F-ID):

- [ ] F-NEW-19 Transfer error list with skip-and-continue: a background copy/move/delete never stops on a per-file error; failures are collected and the rest of the job runs on — — — with a locked file and an unreadable file in a 100-file copy, 98 files copy without a prompt (when "Skip errors in background" is on); the progress window shows an error count; at the end a list shows each failed path with the plain-English reason (F-718) and buttons Retry failed, Copy list, Save log
- [ ] F-NEW-20 Network and unreachable-volume hang resistance: all directory listing, size calculation and refresh on network/removable volumes runs off the UI thread with a timeout — — — with an SMB/NFS mount whose server is unplugged, opening it shows "Not responding, retrying…" within 3 s; the other panel, tabs and menus stay usable; Esc or Cancel returns to the previous folder; the drive list does not block on stale mounts
- [x] F-NEW-21 Hidden-files state always visible and honoured by search — — — when hidden files are shown, the status bar (or path bar) shows a "Hidden: shown" marker; Find files (Alt+F7) has an "Include hidden files" checkbox that defaults to the panel's current state; the toggle F-090/F-242 is listed in the Show menu with its key
- [ ] F-NEW-22 Marta-refugee onboarding: import Marta favorites and offer a Marta-like keymap preset in first-run setup (F-NEW-1) — — — with a Marta config folder present, first-run setup offers "Import from Marta"; its favorites appear in the directory hotlist (§31); a docs page maps Marta's common actions to mormiCommander keys; nothing is changed in Marta's files

Rationale for F-NEW-22: Marta has the largest Mac commander install base and no release since 2024. The README and AlternativeTo listing (distribution checklist, step 11) should name Marta as an alternative target alongside TC and DC.

## Ranked features to add to docs/FEATURES.md

> **Read this list together with "What this changes in the ranking" above.** The demand pass came later and adds a Finder-gap trio at rank 2, moves multi-rename to 4 and SFTP to 7, moves F-NEW-4 below SFTP, and adds F-NEW-19 to F-NEW-22. Where the two disagree, the demand section wins.


Ranked by user value, then differentiation, then effort. Items marked **Parity** already exist in FEATURES.md and should be done first. Items marked **New** are not in the file (checked by grep against all 853 items). They are written as ready-to-paste lines; give them the next free F-number in the suggested section when you add them.

1. **Parity: exact keys and the command line (§8, §10, §3).** Do F-275, F-300, F-066, F-068 and F-290 first. Rationale: the most consistent complaint about every clone, and DC fails the `cd ..` test ([HN 37348653](https://news.ycombinator.com/item?id=37348653)). Effort: M.
2. **Parity: keyboard selection (§6).** Do F-118, F-120 and F-797. Rationale: ForkLift lost TC users by refusing it, and Space selection is "what made it popular with TC fans" ([ForkLift 4.2 post](https://blog.binarynights.com/2024/11/26/forklift-4-2-is-available-display-up-to-three-color-tags-in-list-view/)). Effort: S.
3. **New: first-run setup (§9).**
   `- [ ] F-NEW-1 First-run setup: choose keymap preset (TC-exact default, Mac-native, vim), F-key/fn check and macOS Full Disk Access prompt — — — first launch shows one page: preset picker, "press F5" test that tells the user whether fn is needed, and a button that opens System Settings > Privacy > Full Disk Access; choice saved; reopenable from Configuration`
   Rationale: key presets and choosing bindings on first launch are among Marta's most-reacted requests (#697, 15; 12 reactions). FDA prompts and fn are first-run friction on every Mac. Effort: S.
4. **Parity: archives as folders with edit-and-repack (§29).** Do F-596, F-600, F-606 and F-592. Rationale: named as daily use, including "edit a file and save it back into the ZIP" ([HN 37348285](https://news.ycombinator.com/item?id=37348285)). Effort: M–L.
5. **Parity: sync dirs and compare (§25, §26).** Do F-546 to F-555 and the §26 compare tool. Rationale: the most consistent switch-blocker. It is missing in Commander One and requested in Marta (#48) and Nimble (#17). Effort: L.
6. **Parity: multi-rename (§22).** Do F-506, F-508, F-514 and F-515. Rationale: Commander One markets itself as TC-approved but lacks it, and users cite it in HN anecdotes. Effort: M.
7. **Parity: Lister and quick view (§27).** Do F-082, F-589, F-571 and F-575. Rationale: daily use, and a webview makes it cheap to beat TC's Lister. Effort: M.
8. **Parity: tabs (§2).** Do F-044 and the locked tab items. Rationale: "Now with tabs, it is magical" ([HN 37348569](https://news.ycombinator.com/item?id=37348569)), and locked tabs serve as a hotlist alternative. Effort: S.
9. **New: undo journal (§20; replaces N/A item F-489).**
   `- [ ] F-NEW-2 Undo journal for file operations (rename, move, copy, make folder, delete to trash) — Cmd+Z on macOS; — elsewhere (Ctrl+Z is TC's edit comment) — after each listed operation in a temp folder, undo restores the previous state; multi-level; journal survives restart; operations that can't be undone (permanent delete, overwrite) are marked as such in the confirm dialog`
   Rationale: open in Marta (#705, #230), refused by TC, and a safety net for people whose F8 muscle memory comes from another app. Effort: M.
10. **New: verifiable zero-network guarantee (§39).**
   `- [ ] F-NEW-3 Zero-network test: app makes no outbound connection unless the user opens a remote location or clicks Check for updates — — — CI test launches the app in control mode with network monitoring, runs a scripted session (browse, view, copy, search, thumbnails, Markdown preview with remote images) and fails on any outbound connection; docs/privacy.md links the test`
   Rationale: "phoning home" scrutiny on MacPowerUsers, and superfile's theme-download incident. This turns the main positioning claim into something users can check. Effort: S.
11. **Parity: background queue with pause (§20).** Do F-468, F-470 and F-471. Rationale: "copying/syncing with editable queues" is part of TC's "heavy work" ([HN 37348871](https://news.ycombinator.com/item?id=37348871)). Requested in Nimble (#66). Effort: M.
12. **Parity: SFTP (§30).** Do F-632, F-634 and F-636. Rationale: daily for ops users, and Files #2407 (21 reactions). Effort: M.
13. **New: cloud and object storage via rclone (§30; replaces N/A item F-645).**
   `- [ ] F-NEW-4 Cloud/object-storage panels via rclone library (S3, R2, B2, Azure, GCS, WebDAV, Drive, Dropbox, OneDrive, SMB) with remotes read from the user's rclone.conf — — — a remote defined in rclone.conf appears in the drive list and opens as a panel; copy, move, delete, rename work between local and remote and between two remotes (server-side copy when both are on the same provider); no network traffic until a remote is opened`
   Rationale: Commander One charges for cloud, DC and TC plugins are stale or 32-bit, yazi users asked for rclone, and rclone is Go. Effort: M (browse/copy), L (OAuth wizard).
14. **New: native Quick Look on macOS (§27).**
   `- [ ] F-NEW-5 Native macOS Quick Look for file under cursor — Cmd+Y (macOS only) — Quick Look window opens for the file under cursor and follows the cursor while open; Space keeps its TC meaning unless the user picks the Mac-native preset`
   Rationale: Mac users expect Quick Look, which collides with TC's Space ([HN 37349437](https://news.ycombinator.com/item?id=37349437)). A separate key resolves the conflict. Effort: S.
15. **Parity: native feel and correctness (§1, §5, §42, §36).** Do F-021, F-742, F-757, F-754, F-116 and Wayland drag-and-drop in §36. Rationale: dark mode is Marta's top request (#626, 18 reactions). DC fails on locale collation (#3117), Wayland keys (#2955) and drag-and-drop (#2452). Effort: M.
16. **New: command palette (§14).**
   `- [ ] F-NEW-6 Command palette: fuzzy search over all cm_* commands, hotlist, directory history, open tabs and settings pages — Ctrl+Shift+P (Cmd+Shift+P on macOS) — typing "sync" lists Synchronize dirs with its hotkey; Enter runs it; results ranked by recent use; works with the keyboard only`
   Rationale: the expected pattern for TUI and VS Code users, and a discoverable way into a huge command set. Builds on F-373. Effort: S.
17. **New: Git awareness (§32).**
   `- [ ] F-NEW-7 Git status column and branch indicator — — — in a git work tree a column shows M/A/D/?/ignored per file, ignored files can be dimmed or hidden, the path bar shows branch and dirty/ahead/behind; uses git CLI --porcelain=v2 when installed; no column outside repos`
   Rationale: standard in yazi plugins, absent in TC, DC, Marta and ForkLift, and developers are a core audience. Effort: S–M.
18. **New: folder size treemap (§38).**
   `- [ ] F-NEW-8 Folder size treemap of current folder (shared engine with mormiDiskFree) — — — command shows a treemap of the panel folder with sizes calculated in background; clicking a block navigates there; Esc returns to the list; sizes cached and reused by F-183`
   Rationale: replaces a separate WizTree or ncdu, Go concurrency makes it fast, and it ties the Mormitech apps together. Effort: M.
19. **New: modern image formats everywhere (§27/§33).**
   `- [ ] F-NEW-9 HEIC, AVIF and JPEG XL view and thumbnails on Windows and Linux too (pure Go/WASM decoders, no cgo) — — — sample .heic/.avif/.jxl files show in Lister, quick view and thumbnails on all three OSes`
   Rationale: F-585 and F-676 only promise HEIC on macOS, and webview codec support is inconsistent across WebKitGTK, WebView2 and WKWebView. Effort: M.
20. **New: data table preview (§27).**
   `- [ ] F-NEW-10 Table preview for CSV, TSV, JSONL, Parquet and SQLite in Lister and quick view — — — file opens as a scrollable grid (first 10,000 rows, virtual scrolling) with column names and types; SQLite shows table list; file opened read-only`
   Rationale: no commander offers it, and it is cheap with modernc.org/sqlite and a Parquet library. Effort: M.
21. **New: frecency jump (§31).**
   `- [ ] F-NEW-11 Frecency jump: fuzzy jump to frequently and recently used folders, importing zoxide's database when installed — — — typing part of a folder name lists matches ranked by frequency and recency; Enter navigates the active panel; data stored locally only`
   Rationale: TUI users expect zoxide, and it complements a hotlist that many users never fill. Effort: S.
22. **New: tags (§34).**
   `- [ ] F-NEW-12 File tags: show and edit macOS Finder tags and Linux user.xdg.tags as a column and in the properties dialog — — — tag set in Finder appears in the column; tag set in the app appears in Finder; tags kept on copy/move; Windows: N/A unless a sidecar option is enabled`
   Rationale: Marta #443 (10 reactions) and #1045, DC #3146 (2026), and ForkLift added tag display in 4.2. Effort: M.
23. **New: extended attributes and quarantine (§34).**
   `- [ ] F-NEW-13 Extended attributes column and actions, including remove macOS quarantine flag — — — xattr names listed in properties; "Remove quarantine" clears com.apple.quarantine on selected files; copying to a filesystem that drops xattrs (FAT, exFAT, cloud) warns once`
   Rationale: a common Mac power-user chore. It extends F-475, which only preserves xattrs. Effort: S.
24. **New: cloud placeholder awareness (§38).**
   `- [ ] F-NEW-14 iCloud Drive and OneDrive placeholder awareness — — — a column/icon shows online-only vs downloaded; opening or copying an online-only file triggers the download with progress instead of failing; search and size calculation don't force downloads`
   Rationale: open DC bugs #3066 (OneDrive) and #2675 (iCloud search), and Marta crashes with iCloud (#1181). Effort: M.
25. **New: embedded terminal (§10).**
   `- [ ] F-NEW-15 Embedded terminal pane that follows the active panel folder — — (assignable) — pane opens below the panels running the user's shell in the current folder; changing folder in the panel sends cd; selected names can be inserted at the prompt`
   Rationale: Files #6235 (13 reactions), and mc's Ctrl+O is a reason users stay on mc ([HN](https://news.ycombinator.com/item?id=45271481)). Effort: M (ConPTY on Windows is extra work).
26. **New: duplicate results actions (§24).**
   `- [ ] F-NEW-16 Actions on duplicate search results: keep newest/oldest/first path, send the rest to trash, or replace with hardlinks — — — after an F-534 duplicate search, one command marks all but one per group by the chosen rule; preview shows space saved; delete goes to trash`
   Rationale: Files #11385 (14 reactions). F-534 finds duplicates but doesn't help act on them. Use BLAKE3 for the content pass. Effort: S–M.
27. **New: opt-in local AI rename plan (§22).**
   `- [ ] F-NEW-17 Smart rename plan from a plain-English instruction using a local model endpoint (Ollama/llama.cpp on localhost); off by default — — — with no endpoint configured the feature is hidden; with one configured, the instruction produces a regex/template filled into MRT fields, shown in the normal preview and covered by MRT undo; nothing leaves localhost`
   Rationale: users have asked for it on the Opus forum and no commander ships it, but technical users react badly to forced AI. Keep it hidden by default. Effort: M.
28. **New: snapshot browser (§38).**
   `- [ ] F-NEW-18 Versions of this file/folder from filesystem snapshots (ZFS/Btrfs first, then APFS local snapshots and Windows VSS) — — — command lists available snapshot versions with dates; choosing one opens it in the other panel for compare or copy back`
   Rationale: no commander does this cross-platform. ZFS and Btrfs snapshots are plain folders. Effort: L (M for ZFS/Btrfs only).

Not ranked on purpose: TC binary plugin compatibility (L effort; WLX does not fit a webview; DLLs work on Windows only), a general Lua/JS plugin API (strategic but L, so after 1.0), an SSH helper agent and container panels (niche and L), and semantic AI search (privacy-sensitive and L).

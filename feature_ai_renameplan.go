package main

// The Multi-Rename tool's AI rename plan (issue #33, F-861): from a plain-English instruction, a model on this
// computer proposes the tool's settings, which the page fills into the tool; the user sees the new names in the
// normal preview and renames with Start! (covered by the tool's Undo). This side only asks the model: it never renames.
//
// Off by default: with no local model address in Options > AI the page shows no button, and RenamePlan refuses. The
// address must be on this computer, and the client is a LocalOnly one, which connects to loopback addresses only, so
// the file names sent as examples never leave localhost. The network (loopback) is used only when the user asks for a
// plan; privacy_test.go holds the code to that.

import (
	"context"
	"strings"
	"time"

	"mormicommander/internal/ai"
)

// AIRenamePlan is the model's proposal (internal/ai.RenamePlan).
type AIRenamePlan = ai.RenamePlan

// RenamePlanReady says whether a local model address is set (the page shows the AI button only then). It does not
// contact the model.
func (x *AI) RenamePlanReady() bool {
	return strings.TrimSpace(x.options().LocalURL) != ""
}

// RenamePlan asks the local model for a rename plan for these names (the page sends the old names of the tool's
// list; the first ai.MaxPlanNames go to the model as examples, with the count of the rest). Nothing is renamed.
func (x *AI) RenamePlan(instruction string, names []string) (AIRenamePlan, error) {
	opt := x.options()
	url := strings.TrimSpace(opt.LocalURL)
	if err := ai.CheckLocalURL(url); err != nil {
		return AIRenamePlan{}, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	return (&ai.Client{BaseURL: url, LocalOnly: true}).RenamePlan(ctx, opt.LocalModel, instruction, names)
}

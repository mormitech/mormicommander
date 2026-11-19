package ai

// The AI rename plan (issue #33, F-861): the user writes in plain words how the files should be renamed ("put the
// date first and replace spaces with underscores"), a model on this computer (Ollama, LM Studio, llama.cpp's server:
// any OpenAI-compatible chat completions API on localhost) answers with the Multi-Rename tool's own settings (masks,
// search & replace, case), and the page fills them into the tool. The model only proposes: the user sees the new names
// in the tool's normal preview and renames with Start!, which the tool's Undo covers. Nothing here renames anything.
//
// The address must be on this computer (CheckLocalURL), and a Client with LocalOnly set refuses to connect anywhere
// else, even when a name resolves elsewhere or the server redirects (chat_http.go), so no file name leaves localhost.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"regexp"
	"strings"
)

// MaxPlanNames is how many of the file names go to the model as examples: enough to see the pattern, not a whole
// folder of thousands.
const MaxPlanNames = 40

// MaxPlanInstruction caps the instruction's length (a sentence or two is the use; a pasted book is a mistake).
const MaxPlanInstruction = 2000

// RenamePlan is the model's proposal: the fields of the Multi-Rename tool it wants set (internal/multirename.Settings
// by their JSON names), and a sentence for the user saying what it does. Fields the model leaves out keep what the
// tool has; Set says which were given.
type RenamePlan struct {
	NameMask      string `json:"nameMask"`
	ExtMask       string `json:"extMask"`
	Search        string `json:"search"`
	Replace       string `json:"replace"`
	RegEx         bool   `json:"regex"`
	CaseSensitive bool   `json:"caseSensitive"`
	OnlyFirst     bool   `json:"onlyFirst"`
	IncludeExt    bool   `json:"includeExt"`
	Case          string `json:"case"`
	Explanation   string `json:"explanation"`
	// Set names the fields the model gave (JSON names), so the page changes only those.
	Set []string `json:"set"`
}

// IsLoopbackHost says whether a host name is this computer: "localhost" (or a name under .localhost) or a loopback
// address such as 127.0.0.1 or ::1.
func IsLoopbackHost(host string) bool {
	h := strings.ToLower(strings.TrimSuffix(strings.Trim(host, "[]"), "."))
	if h == "localhost" || strings.HasSuffix(h, ".localhost") {
		return true
	}
	ip := net.ParseIP(h)
	return ip != nil && ip.IsLoopback()
}

// CheckLocalURL says what is wrong with the address of the local model, or nil: it must be an http(s) address of this
// computer, without a user name or password.
func CheckLocalURL(s string) error {
	s = strings.TrimSpace(s)
	if s == "" {
		return errors.New("No local model address is set (Configuration > Options > AI: Local model address for rename plans).")
	}
	u, err := url.Parse(s)
	if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") {
		return errors.New("The local model address must be a web address like http://localhost:11434/v1.")
	}
	if u.User != nil {
		return errors.New("The local model address must not hold a user name or password.")
	}
	if !IsLoopbackHost(u.Hostname()) {
		return errors.New("The local model address must be on this computer (localhost or 127.0.0.1): rename plans never send file names to another machine.")
	}
	return nil
}

// renamePlanSystem tells the model what the tool can do and how to answer.
const renamePlanSystem = `You turn a plain-English rename instruction into settings of a file manager's Multi-Rename tool (like Total Commander's). Answer with ONE JSON object and nothing else. Its keys (leave out what you don't need to change):
- "nameMask": template of the new name without extension. [N] = old name, [N2-5] = characters 2 to 5, [N3-] = from character 3, [N-3-] = last 3 characters, [C] = counter, [Y] [M] [D] = file date year/month/day, [h] [m] [s] = file time, [P] = parent folder name, [U] / [L] / [F] = uppercase / lowercase / Title Case from here. Other text is copied literally. Default "[N]".
- "extMask": template of the new extension. [E] = old extension. Default "[E]".
- "search", "replace": text to find in the new name and what to put instead (applied after the masks). "replace" may be empty to delete.
- "regex": true when "search" is a regular expression (RE2 syntax, $1 $2 for groups in "replace").
- "caseSensitive", "onlyFirst" (replace only the first match), "includeExt" (search the extension too): true or false.
- "case": "" (unchanged), "lower", "upper", "first" (first letter uppercase) or "words" (each word capitalised).
- "explanation": one short sentence for the user saying what the settings do.
Never invent other keys. Do not rename by listing new names; only give these settings.`

// renamePlanPrompt is the user message: the instruction and some of the names.
func renamePlanPrompt(instruction string, names []string) string {
	var b strings.Builder
	b.WriteString("Instruction: ")
	b.WriteString(instruction)
	b.WriteString("\n\nSome of the file names:\n")
	for i, n := range names {
		if i >= MaxPlanNames {
			fmt.Fprintf(&b, "(and %d more)\n", len(names)-MaxPlanNames)
			break
		}
		b.WriteString(n)
		b.WriteByte('\n')
	}
	return b.String()
}

// RenamePlan asks the model for a rename plan. The client must be a LocalOnly one at a local address.
func (c *Client) RenamePlan(ctx context.Context, model, instruction string, names []string) (RenamePlan, error) {
	instruction = strings.TrimSpace(instruction)
	if instruction == "" {
		return RenamePlan{}, errors.New("Write how the files should be renamed.")
	}
	if len([]rune(instruction)) > MaxPlanInstruction {
		return RenamePlan{}, fmt.Errorf("The instruction is too long (at most %d characters).", MaxPlanInstruction)
	}
	if !c.LocalOnly {
		return RenamePlan{}, errors.New("Rename plans use a model on this computer only.")
	}
	if err := CheckLocalURL(c.BaseURL); err != nil {
		return RenamePlan{}, err
	}
	if strings.TrimSpace(model) == "" {
		return RenamePlan{}, errors.New("No local model is chosen (Configuration > Options > AI: Local model for rename plans, like llama3.2).")
	}
	msgs := []ChatMessage{{Role: "system", Content: renamePlanSystem}, {Role: "user", Content: renamePlanPrompt(instruction, names)}}
	reply, err := c.Complete(ctx, strings.TrimSpace(model), msgs, nil)
	if err != nil {
		return RenamePlan{}, err
	}
	return ParseRenamePlan(reply.Text)
}

var fenceRE = regexp.MustCompile("(?s)```(?:json)?\\s*(.*?)```")

// jsonObject finds the JSON object in a model's answer: the whole text, a ```json fence, or the span from the first
// "{" to the last "}" (small models like to talk around it).
func jsonObject(text string) string {
	t := strings.TrimSpace(text)
	if m := fenceRE.FindStringSubmatch(t); m != nil {
		t = strings.TrimSpace(m[1])
	}
	if i, j := strings.Index(t, "{"), strings.LastIndex(t, "}"); i >= 0 && j > i {
		return t[i : j+1]
	}
	return t
}

// planFields are the keys a plan may set, with their kind ("s" text, "b" true/false).
var planFields = map[string]string{
	"nameMask": "s", "extMask": "s", "search": "s", "replace": "s", "case": "s",
	"regex": "b", "caseSensitive": "b", "onlyFirst": "b", "includeExt": "b",
}

// planOrder is the order of Set (stable for the page and tests).
var planOrder = []string{"nameMask", "extMask", "search", "replace", "regex", "caseSensitive", "onlyFirst", "includeExt", "case"}

// maxPlanField caps a text field: a mask or a pattern longer than this is not a rename plan.
const maxPlanField = 500

// ParseRenamePlan reads the model's answer into a plan and checks it: only the known keys, texts of sane length
// without line breaks, a case the tool knows, and a regular expression that compiles. Unknown keys are ignored.
func ParseRenamePlan(text string) (RenamePlan, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal([]byte(jsonObject(text)), &raw); err != nil || raw == nil {
		return RenamePlan{}, errors.New("The local model did not answer with a rename plan (no JSON object in its answer). Try again, or a more capable model.")
	}
	var p RenamePlan
	str := map[string]*string{"nameMask": &p.NameMask, "extMask": &p.ExtMask, "search": &p.Search, "replace": &p.Replace, "case": &p.Case}
	boo := map[string]*bool{"regex": &p.RegEx, "caseSensitive": &p.CaseSensitive, "onlyFirst": &p.OnlyFirst, "includeExt": &p.IncludeExt}
	for _, k := range planOrder {
		v, ok := raw[k]
		if !ok || string(v) == "null" {
			continue
		}
		switch planFields[k] {
		case "s":
			var s string
			if json.Unmarshal(v, &s) != nil {
				return RenamePlan{}, fmt.Errorf("The local model's plan has a bad %q (not a text).", k)
			}
			if len(s) > maxPlanField || strings.ContainsAny(s, "\r\n\x00") {
				return RenamePlan{}, fmt.Errorf("The local model's plan has a bad %q (too long or with line breaks).", k)
			}
			*str[k] = s
		case "b":
			var b bool
			if json.Unmarshal(v, &b) != nil {
				var s string
				if json.Unmarshal(v, &s) != nil || (s != "true" && s != "false") {
					return RenamePlan{}, fmt.Errorf("The local model's plan has a bad %q (not true or false).", k)
				}
				b = s == "true"
			}
			*boo[k] = b
		}
		p.Set = append(p.Set, k)
	}
	if v, ok := raw["explanation"]; ok {
		var s string
		if json.Unmarshal(v, &s) == nil {
			s = strings.Join(strings.Fields(s), " ")
			if r := []rune(s); len(r) > 300 {
				s = string(r[:300]) + "…"
			}
			p.Explanation = s
		}
	}
	switch p.Case {
	case "", "lower", "upper", "first", "words":
	case "unchanged", "none":
		p.Case = ""
	default:
		return RenamePlan{}, fmt.Errorf("The local model's plan has an unknown case %q.", p.Case)
	}
	if p.RegEx && p.Search != "" {
		if _, err := regexp.Compile(p.Search); err != nil {
			return RenamePlan{}, fmt.Errorf("The local model's plan has a regular expression that does not work: %v", err)
		}
	}
	if len(p.Set) == 0 {
		return RenamePlan{}, errors.New("The local model's answer changes nothing. Try to say more precisely how the names should change.")
	}
	return p, nil
}

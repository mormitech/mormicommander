package main

// OpenRouter, or any other OpenAI-compatible chat completions API, as the AI layer's second provider (issue #146,
// step 2). The app talks to the API itself (internal/ai Client and Loop): each tool call of the model goes through the
// same page relay as the Claude CLI's MCP calls (callTool: an "ai-tool" event, answered with ToolReply, 120 s at most),
// and the steps reach the page as the same "ai-progress" events.
//
// The API key is kept in the system keychain only, under the name "ai.openrouter" (internal/keychain), never in a
// settings file; the page can save or remove it (SetAPIKey) and ask whether there is one (HasAPIKey), but never read
// it back. The network is used only when the user starts a run with this provider, or opens the model list of the AI
// settings (Models); privacy_test.go holds the code to that.

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"mormicommander/internal/ai"
	"mormicommander/internal/keychain"
)

// AIModel is one model of the provider's list (internal/ai.Model): {id, name, contextLength, pricing: {prompt,
// completion} per token in US dollars when the API gives prices, noTools when the API says the model can't call
// tools}.
type AIModel = ai.Model

// aiSecretName is the keychain name of a provider's API key.
func aiSecretName(provider string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "openrouter":
		return "ai.openrouter", nil
	}
	return "", fmt.Errorf("The AI provider %q has no API key.", provider)
}

// keychain opens the system keychain once per run of the app; the error says why there is none.
func (x *AI) keychain() (keychain.Keychain, error) {
	x.kcOnce.Do(func() { x.kc, x.kcErr = x.openKeychain() })
	return x.kc, x.kcErr
}

// apiKey returns a provider's saved API key ("" when none is saved). It is read from the keychain once and then kept
// in memory for the session, so opening the Find dialog does not ask the keychain every time.
func (x *AI) apiKey(provider string) (string, error) {
	name, err := aiSecretName(provider)
	if err != nil {
		return "", err
	}
	x.mu.Lock()
	key, ok := x.keys[name]
	x.mu.Unlock()
	if ok {
		return key, nil
	}
	kc, err := x.keychain()
	if err != nil {
		return "", err
	}
	key, err = kc.Get(name)
	if errors.Is(err, keychain.ErrNotFound) {
		key, err = "", nil
	}
	if err != nil {
		return "", err
	}
	x.mu.Lock()
	x.keys[name] = key
	x.mu.Unlock()
	return key, nil
}

// SetAPIKey saves a provider's API key in the system keychain, replacing the one there; an empty key removes it.
// When the system has no keychain (Linux without secret-tool), the error says why, and nothing is saved anywhere else.
func (x *AI) SetAPIKey(provider string, key string) error {
	name, err := aiSecretName(provider)
	if err != nil {
		return err
	}
	key = strings.TrimSpace(key)
	if strings.ContainsAny(key, " \t\r\n") {
		return errors.New("An API key has no spaces or line breaks in it; check what was pasted.")
	}
	kc, err := x.keychain()
	if err != nil {
		return errors.New("The API key can't be saved, because the system keychain can't be used: " + kcMsg(err))
	}
	if key == "" {
		err = kc.Delete(name)
	} else {
		err = kc.Set(name, key)
	}
	if err != nil {
		return errors.New("The system keychain refused the API key: " + kcMsg(err))
	}
	x.mu.Lock()
	x.keys[name] = key
	x.models = map[string][]AIModel{} // (a list fetched with the old key may differ)
	x.mu.Unlock()
	return nil
}

// HasAPIKey says whether a provider's API key is saved (false also when the keychain can't be read).
func (x *AI) HasAPIKey(provider string) bool {
	key, err := x.apiKey(provider)
	return err == nil && key != ""
}

// openRouterURL is the API address of the options, OpenRouter's own when the option is empty.
func openRouterURL(opt aiOptions) string {
	if u := strings.TrimSpace(opt.ORURL); u != "" {
		return u
	}
	return ai.DefaultBaseURL
}

// Models lists the models of a provider's API (GET {url}/models), at the address of the options. The list is kept
// for the session.
func (x *AI) Models(provider string) ([]AIModel, error) {
	return x.ModelsFrom(provider, "")
}

// ModelsFrom is Models at another API address: the one typed in the AI settings before it is saved ("" = the
// options' address).
func (x *AI) ModelsFrom(provider string, url string) ([]AIModel, error) {
	if strings.ToLower(strings.TrimSpace(provider)) != "openrouter" {
		return nil, fmt.Errorf("The AI provider %q has no model list.", provider)
	}
	if strings.TrimSpace(url) == "" {
		url = openRouterURL(x.options())
	}
	url = strings.TrimSpace(url)
	if err := ai.CheckBaseURL(url); err != nil {
		return nil, err
	}
	x.mu.Lock()
	list, ok := x.models[url]
	x.mu.Unlock()
	if ok {
		return list, nil
	}
	// The list needs no key at OpenRouter, but other servers want one; a keychain that can't be read is no reason to
	// fail here.
	key, _ := x.apiKey("openrouter")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	list, err := (&ai.Client{BaseURL: url, Key: key}).Models(ctx)
	if err != nil {
		return nil, err
	}
	x.mu.Lock()
	x.models[url] = list
	x.mu.Unlock()
	return list, nil
}

// openRouterProvider says whether OpenRouter can be used: a key saved and a model chosen. The keychain is asked only
// when OpenRouter is the chosen provider (or was asked already), so a user of Claude Code never gets a keychain
// prompt from opening the Find dialog.
func (x *AI) openRouterProvider(opt aiOptions, selected bool) AIProvider {
	p := AIProvider{Model: strings.TrimSpace(opt.ORModel), URL: openRouterURL(opt)}
	if err := ai.CheckBaseURL(p.URL); err != nil {
		p.Error = err.Error() + " Change it in Configuration > AI settings."
		return p
	}
	if !selected {
		name, _ := aiSecretName("openrouter")
		x.mu.Lock()
		_, known := x.keys[name]
		x.mu.Unlock()
		if !known {
			p.Error = "OpenRouter is not the chosen AI provider (Configuration > AI settings)."
			return p
		}
	}
	key, err := x.apiKey("openrouter")
	switch {
	case err != nil:
		p.Error = "The API key can't be read, because the system keychain can't be used: " + kcMsg(err)
	case key == "":
		p.Error = "No OpenRouter API key is saved. Add one in Configuration > AI settings."
	case p.Model == "":
		p.Error = "No model is chosen. Choose one in Configuration > AI settings."
	default:
		p.Available = true
	}
	return p
}

// startOpenRouter starts a run with the chat completions loop in the background.
func (x *AI) startOpenRouter(opt aiOptions, req AIRequest) (int, error) {
	key, err := x.apiKey("openrouter")
	if err != nil {
		return 0, errors.New("The API key can't be read, because the system keychain can't be used: " + kcMsg(err))
	}
	if key == "" {
		return 0, errors.New("No OpenRouter API key is saved. Add one in Configuration > AI settings.")
	}
	model := strings.TrimSpace(req.Model)
	if model == "" {
		model = strings.TrimSpace(opt.ORModel)
	}
	if model == "" {
		return 0, errors.New("No OpenRouter model is chosen. Choose one in Configuration > AI settings.")
	}
	url := openRouterURL(opt)
	if err := ai.CheckBaseURL(url); err != nil {
		return 0, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	x.mu.Lock()
	x.runSeq++
	id := x.runSeq
	r := &aiRun{id: id, cancel: cancel, ended: make(chan struct{}), exited: make(chan struct{})}
	x.runs[id] = r
	x.mu.Unlock()
	loop := ai.Loop{
		Client: &ai.Client{BaseURL: url, Key: key},
		Model:  model,
		Tools:  x.toolList(),
		Call: func(ctx context.Context, name string, args map[string]any) (string, bool) {
			return x.callTool(ctx, id, name, args)
		},
		// Steps after the run ended (after Stop) are dropped, so the page never sees anything after its last event.
		Emit: func(p ai.Progress) {
			select {
			case <-r.ended:
			default:
				x.emit("ai-progress", id, p)
			}
		},
	}
	go func() {
		last := loop.Run(ctx, ai.SystemPrompt(req.Context), req.Prompt)
		cancel()
		close(r.exited)
		x.finish(r, last)
		x.mu.Lock()
		delete(x.runs, id)
		x.mu.Unlock()
	}()
	return id, nil
}

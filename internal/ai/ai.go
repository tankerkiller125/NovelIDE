// Package ai is NovelIDE's optional AI layer. Provider connections and per-mode
// model selection are configured here (see config.go); the agent itself is built
// on the Microsoft Agent Framework (see maf.go), which handles the provider wire
// formats, streaming, and the tool-calling loop.
package ai

// ProviderKind selects which Agent Framework provider backs a connection.
type ProviderKind string

const (
	KindOpenAI    ProviderKind = "openai"
	KindAnthropic ProviderKind = "anthropic"
	KindGemini    ProviderKind = "gemini"
	// KindA2A connects to a remote Agent-to-Agent (A2A) agent by its card URL
	// (stored in BaseURL); it delegates the turn to that remote agent, so the
	// per-mode Model and the local tool set do not apply.
	KindA2A ProviderKind = "a2a"
	// KindACP runs a locally-installed coding-agent CLI over the Agent Client
	// Protocol; BaseURL holds the chosen agent id (see internal/acp). It reads the
	// manuscript directly and its edits are surfaced as proposals.
	KindACP ProviderKind = "acp"
)

// ValidKind reports whether k is a supported provider kind.
func ValidKind(k ProviderKind) bool {
	switch k {
	case KindOpenAI, KindAnthropic, KindGemini, KindA2A, KindACP:
		return true
	}
	return false
}

// Provider is a fully-specified connection to a model.
type Provider struct {
	Kind    ProviderKind `json:"kind"`
	BaseURL string       `json:"baseUrl"` // OpenAI-compatible base URL, Anthropic host, or an A2A agent-card URL
	APIKey  string       `json:"apiKey"`
	Model   string       `json:"model"`
}

// Role is a message author, used for the conversation history passed from the
// frontend.
type Role string

const (
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
)

// ToolCall names a tool invocation with its JSON arguments; used to bridge the
// tool handlers to the existing dispatch logic.
type ToolCall struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"` // JSON object as a string
}

// Message is one prior turn of the conversation (user or assistant text).
type Message struct {
	Role    Role   `json:"role"`
	Content string `json:"content,omitempty"`
}

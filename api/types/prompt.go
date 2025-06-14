package types

type PromptKind string

const (
	PromptKindPassword    PromptKind = "password"
	PromptKindMultiSelect PromptKind = "multi-select"
	PromptKindBusyChannel PromptKind = "busy-channel"
)

type Prompt struct {
	ID      string         `json:"id"`
	Kind    PromptKind     `json:"kind"`
	Message string         `json:"message"`
	Options []PromptOption `json:"options,omitempty"` // For multi-select
}

type PromptOption struct {
	Value   string `json:"value"`
	Desc    string `json:"desc,omitempty"`
	Checked bool   `json:"checked"`
}

type PromptResponse struct {
	ID    string `json:"id"`
	Value string `json:"value"`
	Err   error  `json:"error"`
}

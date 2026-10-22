package usage

// Quota represents a single usage quota period.
type Quota struct {
	Utilization float64 `json:"utilization"`
	ResetsAt    string  `json:"resetsAt"`
}

// Usage represents account-level rate limit data.
// Populated from Claude Code statusline hook via hookDataStore.
type Usage struct {
	FiveHour *Quota `json:"fiveHour,omitempty"`
	SevenDay *Quota `json:"sevenDay,omitempty"`
}

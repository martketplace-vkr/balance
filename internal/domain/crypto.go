package domain

type RegisterWatchAddressEvent struct {
	UserID   int64  `json:"user_id"`
	Address  string `json:"address"`
	Network  string `json:"network"`
	Asset    string `json:"asset"`
	Source   string `json:"source,omitempty"`
	Provider string `json:"provider,omitempty"`
}

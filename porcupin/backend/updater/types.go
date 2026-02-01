package updater

import "time"

// UpdateInfo contains information about a new release
type UpdateInfo struct {
	Available    bool      `json:"available"`
	Version      string    `json:"version"`
	CurrentVer   string    `json:"current_version"`
	ReleaseNotes string    `json:"release_notes"`
	PubDate      time.Time `json:"pub_date"`
	AssetURL     string    `json:"asset_url"`
	HumanSize    string    `json:"human_size"` // e.g. "12.5 MB"
}

// UpdateProgress represents the progress of an active update
type UpdateProgress struct {
	Phase      string  `json:"phase"` // "downloading", "verifying", "installing", "complete", "error"
	Percent    float64 `json:"percent"`
	Downloaded int64   `json:"downloaded"`
	Total      int64   `json:"total"`
	Message    string  `json:"message"`
	Error      string  `json:"error"`
}

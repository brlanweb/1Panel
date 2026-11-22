package dto

// Cluster alert types. They share one per-target state machine
// (model.AlertTargetState) instead of the daily send counter used by the
// classic resource alerts.
const (
	ClusterAlertResource = "clusterResource"
	ClusterAlertSite     = "siteProbe"
	ClusterAlertModel    = "modelApi"
)

func IsClusterAlertType(alertType string) bool {
	switch alertType {
	case ClusterAlertResource, ClusterAlertSite, ClusterAlertModel:
		return true
	}
	return false
}

// ClusterAlertParams is the AdvancedParams JSON of the cluster alert types.
// Alert.Cycle is the check interval in minutes (site / model) or the CPU and
// memory averaging window (cluster resource); Alert.Count is the number of
// consecutive failed checks before a target counts as failed; Alert.SendCount
// is the number of notifications per incident before the cooldown starts.
type ClusterAlertParams struct {
	// clusterResource
	NodeIDs     []uint   `json:"nodeIds,omitempty"`
	CPU         float64  `json:"cpu,omitempty"`
	Memory      float64  `json:"memory,omitempty"`
	Disk        float64  `json:"disk,omitempty"`
	Offline     bool     `json:"offline,omitempty"`
	DiskExclude []string `json:"diskExclude,omitempty"`

	// siteProbe
	URL            string            `json:"url,omitempty"`
	Method         string            `json:"method,omitempty"`
	ExpectStatus   string            `json:"expectStatus,omitempty"`
	Keyword        string            `json:"keyword,omitempty"`
	FollowRedirect bool              `json:"followRedirect,omitempty"`
	VerifyTLS      bool              `json:"verifyTLS,omitempty"`
	Headers        map[string]string `json:"headers,omitempty"`

	// modelApi
	Provider       string `json:"provider,omitempty"`
	BaseURL        string `json:"baseUrl,omitempty"`
	Model          string `json:"model,omitempty"`
	APIKey         string `json:"apiKey,omitempty"`
	APIKeySet      bool   `json:"apiKeySet,omitempty"`
	MaxTokens      int    `json:"maxTokens,omitempty"`
	StrictContent  bool   `json:"strictContent,omitempty"`
	Treat429AsFail bool   `json:"treat429AsFail,omitempty"`

	// common
	TimeoutSec    int  `json:"timeoutSec,omitempty"`
	UseProxy      bool `json:"useProxy,omitempty"`
	CooldownHours int  `json:"cooldownHours,omitempty"`
}

// ClusterAlertTargetResult is one evaluated target of a check.
type ClusterAlertTargetResult struct {
	Key    string `json:"key"`
	Label  string `json:"label"`
	Failed bool   `json:"failed"`
	Value  string `json:"value"`
	Error  string `json:"error"`
}

type ClusterAlertCheck struct {
	AlertID uint                       `json:"alertId"`
	Targets []ClusterAlertTargetResult `json:"targets"`
}

// ClusterAlertReport is posted by the master core (cluster resource
// sampler) to its own agent over the unix socket.
type ClusterAlertReport struct {
	Checks []ClusterAlertCheck `json:"checks"`
}

// ClusterAlertRule is a rule as handed to the core sampler.
type ClusterAlertRule struct {
	ID        uint               `json:"id"`
	Type      string             `json:"type"`
	Title     string             `json:"title"`
	Cycle     uint               `json:"cycle"`
	Count     uint               `json:"count"`
	SendCount uint               `json:"sendCount"`
	Params    ClusterAlertParams `json:"params"`
}

// ClusterAlertTargetStateDTO is the per-target status shown in the UI.
type ClusterAlertTargetStateDTO struct {
	TargetKey       string `json:"targetKey"`
	TargetLabel     string `json:"targetLabel"`
	Status          string `json:"status"`
	ConsecutiveFail uint   `json:"consecutiveFail"`
	SentCount       uint   `json:"sentCount"`
	CooldownUntil   int64  `json:"cooldownUntil"`
	LastCheckAt     int64  `json:"lastCheckAt"`
	LastChangeAt    int64  `json:"lastChangeAt"`
	LastValue       string `json:"lastValue"`
	LastError       string `json:"lastError"`
}

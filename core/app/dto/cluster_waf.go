package dto

// ClusterWafOverview is the centralized WAF overview. It deliberately
// contains NO traffic or interception numbers: those come from the paid
// xpack WAF log/statistics backend, which does not exist in the open-source
// tree. Inventing them would be fake data. Only counts that can be verified
// against a remote node (website totals) are reported here.
type ClusterWafOverview struct {
	TotalNodes    int                    `json:"totalNodes"`
	TotalWebsites int                    `json:"totalWebsites"`
	Nodes         []ClusterWafNodeStatus `json:"nodes"`
}

type ClusterWafNodeStatus struct {
	NodeID       uint   `json:"nodeId"`
	Name         string `json:"name"`
	WebsiteCount int    `json:"websiteCount"`
	FetchError   string `json:"fetchError"`
}

// ClusterWafWebsite is one website on one remote node, as shown on the
// centralized website-settings page. The WAF fields are PENDING SCHEMA:
// the protection mode / detection strength / rate-limit field names do not
// exist anywhere in the open-source tree, so until they are confirmed by
// reading a real node's {openresty}/1pwaf/data/ configuration, they are
// reported as unknown and the UI must render them read-only with a jump
// link to the remote panel's own WAF page instead of offering edits.
type ClusterWafWebsite struct {
	NodeID        uint   `json:"nodeId"`
	NodeName      string `json:"nodeName"`
	Domain        string `json:"domain"`
	Type          string `json:"type"`
	Status        string `json:"status"`
	Enabled       *bool  `json:"enabled"`
	Mode          string `json:"mode"`
	Strength      string `json:"strength"`
	RateLimit     *bool  `json:"rateLimit"`
	SchemaPending bool   `json:"schemaPending"`
	// JumpURL opens the remote panel's own WAF website-settings page.
	JumpURL string `json:"jumpUrl"`
}

type ClusterWafWebsiteSearch struct {
	SearchWithPage
	NodeID uint `json:"nodeId"`
}

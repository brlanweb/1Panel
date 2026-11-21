package dto

// ProxyNodeSave saves (and downloads) a Clash-format subscription.
type ProxyNodeSave struct {
	SubscriptionURL string `json:"subscriptionUrl" validate:"required"`
	Image           string `json:"image"`
	Port            int    `json:"port"`
	Selected        string `json:"selected"`
}

type ProxyNodeSelect struct {
	Name string `json:"name" validate:"required"`
}

type ProxyNodeOperate struct {
	Operate string `json:"operate" validate:"required,oneof=start stop"`
}

// ProxyNodeItem is one node of the subscription with its last test result.
// Delay is in milliseconds and Speed in bytes per second; -1 means the test
// failed, 0 means not tested yet.
type ProxyNodeItem struct {
	Name   string  `json:"name"`
	Type   string  `json:"type"`
	Delay  int     `json:"delay"`
	Speed  float64 `json:"speed"`
	Tested bool    `json:"tested"`
}

type ProxyNodeStatus struct {
	SubscriptionURL string          `json:"subscriptionUrl"`
	Image           string          `json:"image"`
	Port            int             `json:"port"`
	Selected        string          `json:"selected"`
	ContainerName   string          `json:"containerName"`
	Running         bool            `json:"running"`
	ProxyURL        string          `json:"proxyUrl"`
	Nodes           []ProxyNodeItem `json:"nodes"`
	Testing         bool            `json:"testing"`
	TestDone        int             `json:"testDone"`
	TestTotal       int             `json:"testTotal"`
	TestedAt        string          `json:"testedAt"`
	UpdatedAt       string          `json:"updatedAt"`
	Message         string          `json:"message"`
}

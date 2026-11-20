package service

import (
	"net/http"
	"strings"
	"time"

	"github.com/1Panel-dev/1Panel/core/app/dto"
	"github.com/1Panel-dev/1Panel/core/app/model"
)

// ClusterWafService implements the centralized WAF overview and the
// per-site settings listing. It is READ-ONLY by design: the protection
// mode / detection strength / rate-limit field names do not exist anywhere
// in the open-source tree, so until they are confirmed by reading a real
// node's {openresty}/1pwaf/data/ configuration, no write API is offered.
// The frontend renders the WAF columns as unknown with a jump link to the
// remote panel's own WAF page. Traffic / interception / map / trend numbers
// are NOT implemented (paid xpack WAF log backend) and are never fabricated.
type ClusterWafService struct {
	nodes *ClusterNodeService
}

type IClusterWafService interface {
	Overview() (dto.ClusterWafOverview, error)
	Websites(req dto.ClusterWafWebsiteSearch) (int64, interface{}, error)
}

func NewIClusterWafService() IClusterWafService {
	return &ClusterWafService{nodes: NewIClusterNodeService().(*ClusterNodeService)}
}

func (s *ClusterWafService) Overview() (dto.ClusterWafOverview, error) {
	nodes, err := clusterNodeRepo.GetList()
	if err != nil {
		return dto.ClusterWafOverview{}, err
	}
	var overview dto.ClusterWafOverview
	overview.TotalNodes = len(nodes)
	for _, node := range nodes {
		status := dto.ClusterWafNodeStatus{NodeID: node.ID, Name: node.Name}
		if node.Status == model.ClusterNodeStatusOffline {
			status.FetchError = node.LastError
			if status.FetchError == "" {
				status.FetchError = "node is offline (last check failed)"
			}
			overview.Nodes = append(overview.Nodes, status)
			continue
		}
		total, err := s.nodes.remotePageTotal(node, "/api/v2/websites/search", map[string]interface{}{"page": 1, "pageSize": 1, "orderBy": "created_at", "order": "descending"})
		if err != nil {
			status.FetchError = truncateErr(err.Error())
		} else {
			status.WebsiteCount = total
			overview.TotalWebsites += total
		}
		overview.Nodes = append(overview.Nodes, status)
	}
	if overview.Nodes == nil {
		overview.Nodes = []dto.ClusterWafNodeStatus{}
	}
	return overview, nil
}

type remoteWebsiteItem struct {
	PrimaryDomain string `json:"primaryDomain"`
	Type          string `json:"type"`
	Status        string `json:"status"`
}

func (s *ClusterWafService) Websites(req dto.ClusterWafWebsiteSearch) (int64, interface{}, error) {
	nodes, err := clusterNodeRepo.GetList()
	if err != nil {
		return 0, nil, err
	}
	page := req.Page
	if page <= 0 {
		page = 1
	}
	pageSize := req.PageSize
	if pageSize <= 0 {
		pageSize = 20
	}
	var all []dto.ClusterWafWebsite
	for _, node := range nodes {
		if req.NodeID != 0 && node.ID != req.NodeID {
			continue
		}
		if node.Status == model.ClusterNodeStatusOffline {
			continue
		}
		var pageResult struct {
			Total int64               `json:"total"`
			Items []remoteWebsiteItem `json:"items"`
		}
		body := map[string]interface{}{"page": 1, "pageSize": 100, "orderBy": "created_at", "order": "descending"}
		if err := s.nodes.callRemoteJSON(node, http.MethodPost, "/api/v2/websites/search", body, &pageResult, 10*time.Second); err != nil {
			// A single unreachable node must not fail the whole listing.
			continue
		}
		for _, item := range pageResult.Items {
			all = append(all, dto.ClusterWafWebsite{
				NodeID:        node.ID,
				NodeName:      node.Name,
				Domain:        item.PrimaryDomain,
				Type:          item.Type,
				Status:        item.Status,
				SchemaPending: true,
				JumpURL:       strings.TrimRight(node.EntryURL, "/") + "/xpack/waf/websites",
			})
		}
	}
	total := int64(len(all))
	if info := strings.TrimSpace(req.Info); info != "" {
		needle := strings.ToLower(info)
		var filtered []dto.ClusterWafWebsite
		for _, w := range all {
			if strings.Contains(strings.ToLower(w.Domain), needle) || strings.Contains(strings.ToLower(w.NodeName), needle) {
				filtered = append(filtered, w)
			}
		}
		all = filtered
		total = int64(len(all))
	}
	start := (page - 1) * pageSize
	if start >= len(all) {
		return total, []dto.ClusterWafWebsite{}, nil
	}
	end := start + pageSize
	if end > len(all) {
		end = len(all)
	}
	return total, all[start:end], nil
}
